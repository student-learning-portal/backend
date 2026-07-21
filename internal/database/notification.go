package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/student-learning-portal/backend/internal/domain"
)

// defaultNotificationLimit caps ListForUser when the caller passes a
// non-positive limit, so the bell feed never tries to load an unbounded history.
const defaultNotificationLimit = 50

type PostgresNotificationRepository struct {
	db *sql.DB
}

func NewPostgresNotificationRepository(db *sql.DB) domain.NotificationRepository {
	return &PostgresNotificationRepository{db: db}
}

func (r *PostgresNotificationRepository) Create(ctx context.Context, n domain.Notification) (domain.Notification, error) {
	courseID := sql.NullString{}
	if n.CourseID != nil && *n.CourseID != "" {
		courseID = sql.NullString{String: *n.CourseID, Valid: true}
	}
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO notifications (user_id, type, title, body, course_id)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at`,
		n.UserID, n.Type, n.Title, n.Body, courseID,
	).Scan(&n.ID, &n.CreatedAt)
	if err != nil {
		return domain.Notification{}, fmt.Errorf("create notification: %w", err)
	}
	return n, nil
}

func (r *PostgresNotificationRepository) ListForUser(ctx context.Context, userID string, limit int) ([]domain.Notification, error) {
	if limit <= 0 {
		limit = defaultNotificationLimit
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, type, title, body, course_id, read_at, created_at
		 FROM notifications
		 WHERE user_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2`,
		userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	notifications := []domain.Notification{}
	for rows.Next() {
		var (
			n        domain.Notification
			courseID sql.NullString
			readAt   sql.NullTime
		)
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body,
			&courseID, &readAt, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		if courseID.Valid {
			n.CourseID = &courseID.String
		}
		if readAt.Valid {
			n.ReadAt = &readAt.Time
		}
		notifications = append(notifications, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notifications: %w", err)
	}
	return notifications, nil
}

func (r *PostgresNotificationRepository) CountUnread(ctx context.Context, userID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`,
		userID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count unread notifications: %w", err)
	}
	return count, nil
}

func (r *PostgresNotificationRepository) MarkRead(ctx context.Context, userID, id string) error {
	// The user_id predicate scopes the update to the caller's own feed: a
	// mismatched (id, user) pair updates zero rows and returns nil.
	_, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET read_at = now()
		 WHERE id = $1 AND user_id = $2 AND read_at IS NULL`,
		id, userID,
	)
	if err != nil {
		return fmt.Errorf("mark notification read: %w", err)
	}
	return nil
}

func (r *PostgresNotificationRepository) MarkAllRead(ctx context.Context, userID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET read_at = now()
		 WHERE user_id = $1 AND read_at IS NULL`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("mark all notifications read: %w", err)
	}
	return nil
}
