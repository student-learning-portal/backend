package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/student-learning-portal/backend/internal/domain"
)

const pgUniqueViolation = "23505"

type PostgresUserRepository struct {
	db *sql.DB
}

func NewPostgresUserRepository(db *sql.DB) domain.UserRepository {
	return &PostgresUserRepository{db: db}
}

func (r *PostgresUserRepository) Create(user domain.User) (domain.User, error) {
	anonymousID := sql.NullString{String: user.AnonymousID, Valid: user.AnonymousID != ""}

	row := r.db.QueryRow(
		`INSERT INTO users (email, password_hash, full_name, role, anonymous_id)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, email, password_hash, full_name, role, COALESCE(anonymous_id::text, ''), created_at, updated_at, wallet_balance`,
		user.Email, user.PasswordHash, user.FullName, user.Role, anonymousID,
	)

	created, err := scanUser(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return domain.User{}, domain.ErrEmailTaken
		}
		return domain.User{}, fmt.Errorf("create user: %w", err)
	}
	return created, nil
}

func (r *PostgresUserRepository) GetByEmail(email string) (domain.User, error) {
	row := r.db.QueryRow(
		`SELECT id, email, password_hash, full_name, role, COALESCE(anonymous_id::text, ''), created_at, updated_at, wallet_balance
		 FROM users WHERE email = $1`,
		email,
	)
	user, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("get user by email: %w", err)
	}
	return user, nil
}

func (r *PostgresUserRepository) GetByID(id string) (domain.User, error) {
	row := r.db.QueryRow(
		`SELECT id, email, password_hash, full_name, role, COALESCE(anonymous_id::text, ''), created_at, updated_at, wallet_balance
		 FROM users WHERE id = $1`,
		id,
	)
	user, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("get user by id: %w", err)
	}
	return user, nil
}

func scanUser(row *sql.Row) (domain.User, error) {
	var u domain.User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &u.Role, &u.AnonymousID, &u.CreatedAt, &u.UpdatedAt, &u.Balance)
	return u, err
}

// DeductBalance atomically subtracts amount from the user's wallet, failing
// with ErrInsufficientFunds if the balance would go negative.
func (r *PostgresUserRepository) DeductBalance(ctx context.Context, userID string, amount float64) (float64, error) {
	var balance float64
	err := r.db.QueryRowContext(ctx,
		`UPDATE users SET wallet_balance = wallet_balance - $1
		 WHERE id = $2 AND wallet_balance >= $1
		 RETURNING wallet_balance`,
		amount, userID,
	).Scan(&balance)
	if errors.Is(err, sql.ErrNoRows) {
		if _, getErr := r.GetByID(userID); errors.Is(getErr, domain.ErrUserNotFound) {
			return 0, domain.ErrUserNotFound
		}
		return 0, domain.ErrInsufficientFunds
	}
	if err != nil {
		return 0, fmt.Errorf("deduct balance: %w", err)
	}
	return balance, nil
}

// CreditBalance atomically adds amount to the user's wallet (e.g. on refund).
func (r *PostgresUserRepository) CreditBalance(ctx context.Context, userID string, amount float64) (float64, error) {
	var balance float64
	err := r.db.QueryRowContext(ctx,
		`UPDATE users SET wallet_balance = wallet_balance + $1
		 WHERE id = $2
		 RETURNING wallet_balance`,
		amount, userID,
	).Scan(&balance)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, domain.ErrUserNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("credit balance: %w", err)
	}
	return balance, nil
}
