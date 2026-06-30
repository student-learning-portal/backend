package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/student-learning-portal/backend/internal/domain"
)

type PostgresEntitlementRepository struct {
	db *sql.DB
}

func NewPostgresEntitlementRepository(db *sql.DB) domain.EntitlementRepository {
	return &PostgresEntitlementRepository{db: db}
}

func (r *PostgresEntitlementRepository) CreatePayment(ctx context.Context, p domain.Payment) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO payment (txn_id, cart_id, actor_id, course_id, amount, currency, status, sandbox)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		p.TxnID, p.CartID, p.ActorID, p.CourseID, p.Amount, p.Currency, p.Status, p.Sandbox,
	)
	if err != nil {
		return fmt.Errorf("create payment: %w", err)
	}
	return nil
}

func (r *PostgresEntitlementRepository) GetPayment(ctx context.Context, txnID string) (domain.Payment, error) {
	var p domain.Payment
	err := r.db.QueryRowContext(ctx,
		`SELECT txn_id, cart_id, actor_id, course_id, amount, currency, status, sandbox, created_at
		 FROM payment WHERE txn_id = $1`,
		txnID,
	).Scan(&p.TxnID, &p.CartID, &p.ActorID, &p.CourseID, &p.Amount, &p.Currency, &p.Status, &p.Sandbox, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Payment{}, domain.ErrPaymentNotFound
	}
	if err != nil {
		return domain.Payment{}, fmt.Errorf("get payment: %w", err)
	}
	return p, nil
}

func (r *PostgresEntitlementRepository) UpdatePaymentStatus(ctx context.Context, txnID, status string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE payment SET status = $1 WHERE txn_id = $2`,
		status, txnID,
	)
	if err != nil {
		return fmt.Errorf("update payment status: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrPaymentNotFound
	}
	return nil
}

func (r *PostgresEntitlementRepository) CreateGrant(ctx context.Context, g domain.AccessGrant) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO access_grant (grant_id, actor_id, course_id, txn_id, granted_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (actor_id, course_id, txn_id) DO NOTHING`,
		g.GrantID, g.ActorID, g.CourseID, g.TxnID, g.GrantedAt,
	)
	if err != nil {
		return fmt.Errorf("create grant: %w", err)
	}
	return nil
}

func (r *PostgresEntitlementRepository) RevokeGrant(ctx context.Context, txnID, reason string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE access_grant SET revoked_at = $1, revoke_reason = $2
		 WHERE txn_id = $3 AND revoked_at IS NULL`,
		time.Now(), reason, txnID,
	)
	if err != nil {
		return fmt.Errorf("revoke grant: %w", err)
	}
	return nil
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
