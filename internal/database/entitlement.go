package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/student-learning-portal/backend/internal/domain"
)

type PostgresEntitlementRepository struct {
	db *sql.DB
}

func NewPostgresEntitlementRepository(db *sql.DB) domain.EntitlementRepository {
	return &PostgresEntitlementRepository{db: db}
}

// CreatePaymentAndGrant inserts the payment and access grant inside a single
// transaction (domain.EntitlementRepository doc). The partial unique index
// ux_access_grant_one_active (actor_id, course_id WHERE revoked_at IS NULL)
// guarantees only one active grant per course exists, so a concurrent
// checkout for the same course surfaces as a unique violation on the grant
// insert, reported as domain.ErrAlreadyPurchased; the payment's txn_id
// primary key gives the same signal for a duplicate webhook delivery,
// reported as domain.ErrPaymentAlreadyRecorded. Either way the transaction
// rolls back, so no partial row is ever left behind.
func (r *PostgresEntitlementRepository) CreatePaymentAndGrant(ctx context.Context, p domain.Payment, g domain.AccessGrant) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("create payment and grant: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO payment (txn_id, cart_id, actor_id, course_id, amount, currency, status, sandbox)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		p.TxnID, p.CartID, p.ActorID, p.CourseID, p.Amount, p.Currency, p.Status, p.Sandbox,
	); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return domain.ErrPaymentAlreadyRecorded
		}
		return fmt.Errorf("create payment and grant: create payment: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO access_grant (grant_id, actor_id, course_id, txn_id, granted_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		g.GrantID, g.ActorID, g.CourseID, g.TxnID, g.GrantedAt,
	); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return domain.ErrAlreadyPurchased
		}
		return fmt.Errorf("create payment and grant: create grant: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("create payment and grant: commit: %w", err)
	}
	return nil
}

// SettleRefund atomically settles a refund (domain.EntitlementRepository
// doc): it locks the payment row, marks it refunded, revokes the associated
// access grant, and credits the buyer's wallet, all in one transaction. If
// the payment is already refunded (idempotent retry, e.g. a redelivered
// webhook) it returns the current state and balance without crediting the
// wallet again.
func (r *PostgresEntitlementRepository) SettleRefund(ctx context.Context, txnID string) (domain.Payment, float64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Payment{}, 0, fmt.Errorf("settle refund: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	var p domain.Payment
	err = tx.QueryRowContext(ctx,
		`SELECT txn_id, cart_id, actor_id, course_id, amount, currency, status, sandbox, created_at
		 FROM payment WHERE txn_id = $1 FOR UPDATE`,
		txnID,
	).Scan(&p.TxnID, &p.CartID, &p.ActorID, &p.CourseID, &p.Amount, &p.Currency, &p.Status, &p.Sandbox, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Payment{}, 0, domain.ErrPaymentNotFound
	}
	if err != nil {
		return domain.Payment{}, 0, fmt.Errorf("settle refund: get payment: %w", err)
	}

	if p.Status == "refunded" {
		var balance float64
		if err = tx.QueryRowContext(ctx,
			`SELECT wallet_balance FROM users WHERE id = $1`, p.ActorID,
		).Scan(&balance); err != nil {
			return domain.Payment{}, 0, fmt.Errorf("settle refund: read balance: %w", err)
		}
		if err = tx.Commit(); err != nil {
			return domain.Payment{}, 0, fmt.Errorf("settle refund: commit: %w", err)
		}
		return p, balance, nil
	}

	if _, err = tx.ExecContext(ctx,
		`UPDATE payment SET status = 'refunded' WHERE txn_id = $1`, txnID,
	); err != nil {
		return domain.Payment{}, 0, fmt.Errorf("settle refund: update payment: %w", err)
	}

	if _, err = tx.ExecContext(ctx,
		`UPDATE access_grant SET revoked_at = $1, revoke_reason = 'refund'
		 WHERE txn_id = $2 AND revoked_at IS NULL`,
		time.Now(), txnID,
	); err != nil {
		return domain.Payment{}, 0, fmt.Errorf("settle refund: revoke grant: %w", err)
	}

	var balance float64
	err = tx.QueryRowContext(ctx,
		`UPDATE users SET wallet_balance = wallet_balance + $1 WHERE id = $2 RETURNING wallet_balance`,
		p.Amount, p.ActorID,
	).Scan(&balance)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Payment{}, 0, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.Payment{}, 0, fmt.Errorf("settle refund: credit balance: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return domain.Payment{}, 0, fmt.Errorf("settle refund: commit: %w", err)
	}
	p.Status = "refunded"
	return p, balance, nil
}

func (r *PostgresEntitlementRepository) HasActiveGrant(ctx context.Context, actorID, courseID string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM access_grant
			WHERE actor_id = $1 AND course_id = $2 AND revoked_at IS NULL
		)`,
		actorID, courseID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check active grant: %w", err)
	}
	return exists, nil
}

func (r *PostgresEntitlementRepository) GetActiveGrant(ctx context.Context, actorID, courseID string) (domain.AccessGrant, error) {
	var g domain.AccessGrant
	err := r.db.QueryRowContext(ctx,
		`SELECT grant_id, actor_id, course_id, txn_id, granted_at
		 FROM access_grant
		 WHERE actor_id = $1 AND course_id = $2 AND revoked_at IS NULL
		 ORDER BY granted_at DESC
		 LIMIT 1`,
		actorID, courseID,
	).Scan(&g.GrantID, &g.ActorID, &g.CourseID, &g.TxnID, &g.GrantedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AccessGrant{}, domain.ErrGrantNotFound
	}
	if err != nil {
		return domain.AccessGrant{}, fmt.Errorf("get active grant: %w", err)
	}
	return g, nil
}

func (r *PostgresEntitlementRepository) GetEnrolledCourses(ctx context.Context, actorID string) ([]domain.Course, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT c.id, c.teacher_id, c.title, c.description, c.subject, c.price, c.currency, c.status, c.created_at, c.updated_at
		 FROM courses c
		 INNER JOIN access_grant ag ON ag.course_id = c.id::text
		 WHERE ag.actor_id = $1 AND ag.revoked_at IS NULL
		 ORDER BY c.title`,
		actorID,
	)
	if err != nil {
		return nil, fmt.Errorf("get enrolled courses: %w", err)
	}
	defer rows.Close()

	return scanCourseRows(rows)
}

// ListPayments returns the buyer's full payment history (purchases and
// refunds), newest first, enriched with the course title for display.
func (r *PostgresEntitlementRepository) ListPayments(ctx context.Context, actorID string) ([]domain.PaymentHistoryEntry, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT p.txn_id, p.cart_id, p.actor_id, p.course_id, p.amount, p.currency, p.status, p.sandbox, p.created_at,
		        COALESCE(c.title, '')
		 FROM payment p
		 LEFT JOIN courses c ON c.id::text = p.course_id
		 WHERE p.actor_id = $1
		 ORDER BY p.created_at DESC`,
		actorID,
	)
	if err != nil {
		return nil, fmt.Errorf("list payments: %w", err)
	}
	defer rows.Close()

	var entries []domain.PaymentHistoryEntry
	for rows.Next() {
		var e domain.PaymentHistoryEntry
		if err := rows.Scan(
			&e.TxnID, &e.CartID, &e.ActorID, &e.CourseID, &e.Amount, &e.Currency, &e.Status, &e.Sandbox, &e.CreatedAt,
			&e.CourseTitle,
		); err != nil {
			return nil, fmt.Errorf("scan payment history: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list payments: %w", err)
	}
	return entries, nil
}

func (r *PostgresEntitlementRepository) LogAccessCheck(ctx context.Context, l domain.AccessCheckLog) error {
	lessonID := sql.NullString{String: l.LessonID, Valid: l.LessonID != ""}
	denyReason := sql.NullString{String: l.DenyReason, Valid: l.DenyReason != ""}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO access_check_log (event_id, actor_id, course_id, lesson_id, decision, deny_reason, checked_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		l.EventID, l.ActorID, l.CourseID, lessonID, l.Decision, denyReason, l.CheckedAt,
	)
	if err != nil {
		return fmt.Errorf("log access check: %w", err)
	}
	return nil
}
