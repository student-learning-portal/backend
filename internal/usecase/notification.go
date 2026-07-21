package usecase

import (
	"context"

	"github.com/student-learning-portal/backend/internal/domain"
)

// NotificationUseCase serves a user's in-app "bell" feed: listing, the unread
// count behind the badge, and marking items read. Authorization is implicit —
// every method is scoped to the caller's own UserID, so one user can never read
// or mutate another's feed.
type NotificationUseCase struct {
	notifications domain.NotificationRepository
}

func NewNotificationUseCase(notifications domain.NotificationRepository) *NotificationUseCase {
	return &NotificationUseCase{notifications: notifications}
}

// List returns the user's notifications, newest first, capped at limit (a
// non-positive limit falls back to the repository default).
func (uc *NotificationUseCase) List(ctx context.Context, userID string, limit int) ([]domain.Notification, error) {
	return uc.notifications.ListForUser(ctx, userID, limit)
}

// UnreadCount returns how many of the user's notifications are unread.
func (uc *NotificationUseCase) UnreadCount(ctx context.Context, userID string) (int, error) {
	return uc.notifications.CountUnread(ctx, userID)
}

// MarkRead stamps a single notification read; a foreign id is silently ignored.
func (uc *NotificationUseCase) MarkRead(ctx context.Context, userID, id string) error {
	return uc.notifications.MarkRead(ctx, userID, id)
}

// MarkAllRead stamps every unread notification of the user read.
func (uc *NotificationUseCase) MarkAllRead(ctx context.Context, userID string) error {
	return uc.notifications.MarkAllRead(ctx, userID)
}
