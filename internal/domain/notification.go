package domain

import (
	"context"
	"time"
)

// NotificationType classifies what produced a notification. Kept as a small
// closed set of string constants so new producers stay self-documenting; the
// column itself is a free TEXT, so adding a type needs no migration.
type NotificationType string

const (
	// NotificationTypeMessage is emitted to the *other* participant of a course
	// chat thread when a message is posted (see usecase.ChatUseCase).
	NotificationTypeMessage NotificationType = "message"
)

// Notification is one item in a user's in-app "bell" feed, addressed to a single
// recipient (UserID). ReadAt is nil while the notification is unread — the badge
// on the bell counts those. CourseID is an optional deep-link target the client
// opens when the notification is clicked.
type Notification struct {
	ID        string           `json:"id"`
	UserID    string           `json:"user_id"`
	Type      NotificationType `json:"type"`
	Title     string           `json:"title"`
	Body      string           `json:"body"`
	CourseID  *string          `json:"course_id,omitempty"`
	ReadAt    *time.Time       `json:"read_at,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
}

// NotificationRepository persists and retrieves per-user notifications.
type NotificationRepository interface {
	// Create inserts a notification and returns it with its generated id and
	// created_at timestamp.
	Create(ctx context.Context, n Notification) (Notification, error)
	// ListForUser returns a user's notifications, newest first, capped at limit.
	ListForUser(ctx context.Context, userID string, limit int) ([]Notification, error)
	// CountUnread returns how many of the user's notifications are still unread.
	CountUnread(ctx context.Context, userID string) (int, error)
	// MarkRead stamps a single notification as read. It is a no-op (nil error)
	// if the id does not belong to the user, so one user can never touch
	// another's feed.
	MarkRead(ctx context.Context, userID, id string) error
	// MarkAllRead stamps every one of the user's unread notifications as read.
	MarkAllRead(ctx context.Context, userID string) error
}
