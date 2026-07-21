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

// userCols is the canonical column list returned by every user SELECT/RETURNING.
const userCols = `id, email, password_hash, full_name, role,
	COALESCE(anonymous_id::text, ''), created_at, updated_at, wallet_balance,
	COALESCE(avatar_url, ''), COALESCE(teacher_status, ''),
	teacher_status_updated_at`

type PostgresUserRepository struct {
	db *sql.DB
}

// NewPostgresUserRepository returns the concrete repository: it satisfies both
// domain.UserRepository (the request path) and domain.TeacherApprovalRepository
// (the admin moderation path), and callers narrow it to whichever they need.
func NewPostgresUserRepository(db *sql.DB) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

func (r *PostgresUserRepository) Create(user domain.User) (domain.User, error) {
	anonymousID := sql.NullString{String: user.AnonymousID, Valid: user.AnonymousID != ""}
	teacherStatus := sql.NullString{String: string(user.TeacherStatus), Valid: user.TeacherStatus != ""}

	row := r.db.QueryRow(
		`INSERT INTO users (email, password_hash, full_name, role, anonymous_id, teacher_status,
		                    teacher_status_updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, CASE WHEN $6::text IS NULL THEN NULL ELSE now() END)
		 RETURNING `+userCols,
		user.Email, user.PasswordHash, user.FullName, user.Role, anonymousID, teacherStatus,
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
	row := r.db.QueryRow(`SELECT `+userCols+` FROM users WHERE email = $1`, email)
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
	row := r.db.QueryRow(`SELECT `+userCols+` FROM users WHERE id = $1`, id)
	user, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("get user by id: %w", err)
	}
	return user, nil
}

func (r *PostgresUserRepository) UpdateEmail(ctx context.Context, userID, newEmail string) (domain.User, error) {
	row := r.db.QueryRowContext(ctx,
		`UPDATE users SET email = $1, updated_at = now() WHERE id = $2 RETURNING `+userCols,
		newEmail, userID,
	)
	user, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return domain.User{}, domain.ErrEmailTaken
		}
		return domain.User{}, fmt.Errorf("update email: %w", err)
	}
	return user, nil
}

func (r *PostgresUserRepository) UpdatePasswordHash(ctx context.Context, userID, newHash string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2`,
		newHash, userID,
	)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}

func (r *PostgresUserRepository) UpdateFullName(ctx context.Context, userID, fullName string) (domain.User, error) {
	row := r.db.QueryRowContext(ctx,
		`UPDATE users SET full_name = $1, updated_at = now() WHERE id = $2 RETURNING `+userCols,
		fullName, userID,
	)
	user, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("update full name: %w", err)
	}
	return user, nil
}

func (r *PostgresUserRepository) UpdateAvatarURL(ctx context.Context, userID, avatarURL string) (domain.User, error) {
	row := r.db.QueryRowContext(ctx,
		`UPDATE users SET avatar_url = $1, updated_at = now() WHERE id = $2 RETURNING `+userCols,
		avatarURL, userID,
	)
	user, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("update avatar url: %w", err)
	}
	return user, nil
}

// rowScanner is the shared shape of *sql.Row and *sql.Rows, so single-user
// lookups and the admin queue listing can share one column mapping.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (domain.User, error) {
	var u domain.User
	// Left as a nullable column rather than COALESCEd to an epoch default: a
	// row that was never reviewed must come back as the zero time, which the
	// admin DTO renders as "no decision yet" instead of 1 January 1970.
	var statusUpdatedAt sql.NullTime
	err := row.Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &u.Role,
		&u.AnonymousID, &u.CreatedAt, &u.UpdatedAt, &u.Balance, &u.AvatarURL,
		&u.TeacherStatus, &statusUpdatedAt,
	)
	u.TeacherStatusUpdatedAt = statusUpdatedAt.Time
	return u, err
}

// ListTeachersByStatus returns the teacher accounts in one queue state (or all
// of them when status is empty), newest registration first so the admin's
// review list opens on the applications that just came in.
func (r *PostgresUserRepository) ListTeachersByStatus(
	ctx context.Context, status domain.TeacherStatus,
) ([]domain.User, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+userCols+`
		   FROM users
		  WHERE role = 'teacher' AND ($1::text = '' OR teacher_status = $1::text)
		  ORDER BY created_at DESC`,
		string(status),
	)
	if err != nil {
		return nil, fmt.Errorf("list teachers by status: %w", err)
	}
	defer rows.Close()

	users := make([]domain.User, 0)
	for rows.Next() {
		u, scanErr := scanUser(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan teacher: %w", scanErr)
		}
		users = append(users, u)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate teachers: %w", err)
	}
	return users, nil
}

// SetTeacherStatus records an administrator's decision. The role = 'teacher'
// guard in the WHERE clause keeps the queue from being used to flip a student
// or a fellow admin into an approved teacher; a miss is disambiguated with a
// follow-up read so the caller gets ErrNotTeacher instead of a blank 404.
func (r *PostgresUserRepository) SetTeacherStatus(
	ctx context.Context, userID string, status domain.TeacherStatus, reviewerID string,
) (domain.User, error) {
	row := r.db.QueryRowContext(ctx,
		`UPDATE users
		    SET teacher_status = $1,
		        teacher_status_updated_at = now(),
		        teacher_reviewed_by = $2,
		        updated_at = now()
		  WHERE id = $3 AND role = 'teacher'
		 RETURNING `+userCols,
		string(status), sql.NullString{String: reviewerID, Valid: reviewerID != ""}, userID,
	)
	user, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		if existing, getErr := r.GetByID(userID); getErr == nil && existing.Role != domain.RoleTeacher {
			return domain.User{}, domain.ErrNotTeacher
		}
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("set teacher status: %w", err)
	}
	return user, nil
}

// EnsureAdmin is the startup bootstrap: it inserts the administrator account
// when it is missing and otherwise leaves the existing row exactly as it is, so
// restarting the server never resets a password an admin deliberately changed.
func (r *PostgresUserRepository) EnsureAdmin(
	ctx context.Context, login, passwordHash, fullName string,
) (domain.User, bool, error) {
	row := r.db.QueryRowContext(ctx,
		`INSERT INTO users (email, password_hash, full_name, role)
		 VALUES ($1, $2, $3, 'admin')
		 ON CONFLICT (email) DO NOTHING
		 RETURNING `+userCols,
		login, passwordHash, fullName,
	)
	created, err := scanUser(row)
	if err == nil {
		return created, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, false, fmt.Errorf("create admin: %w", err)
	}

	existing, err := r.GetByEmail(login)
	if err != nil {
		return domain.User{}, false, err
	}
	if existing.Role != domain.RoleAdmin {
		return domain.User{}, false, domain.ErrLoginTaken
	}
	return existing, false, nil
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
