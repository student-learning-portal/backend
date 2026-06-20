package database

import (
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
		 RETURNING id, email, password_hash, full_name, role, COALESCE(anonymous_id::text, ''), created_at, updated_at`,
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
		`SELECT id, email, password_hash, full_name, role, COALESCE(anonymous_id::text, ''), created_at, updated_at
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
		`SELECT id, email, password_hash, full_name, role, COALESCE(anonymous_id::text, ''), created_at, updated_at
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
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &u.Role, &u.AnonymousID, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}
