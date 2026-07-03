package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrPaymentNotFound  = errors.New("payment not found")
	ErrGrantNotFound    = errors.New("no active purchase found for this course")
	ErrAlreadyPurchased = errors.New("course already purchased")

	// ErrPaymentAlreadyRecorded signals a duplicate txn_id on payment insert
	// (the payment table's primary key): a gateway webhook redelivering the
	// same event. Callers treat this as an idempotent no-op, not a failure.
	ErrPaymentAlreadyRecorded = errors.New("payment already recorded")

	// ErrUnknownWebhookStatus is a caller (gateway) error — an unrecognized
	// status value in the webhook payload — as opposed to an internal
	// failure, so handlers can map it to 400 instead of 500.
	ErrUnknownWebhookStatus = errors.New("unknown webhook status")
)

type Payment struct {
	TxnID     string
	CartID    string
	ActorID   string
	CourseID  string
	Amount    float64
	Currency  string
	Status    string
	Sandbox   bool
	CreatedAt time.Time
}

type AccessGrant struct {
	GrantID      string
	ActorID      string
	CourseID     string
	TxnID        string
	GrantedAt    time.Time
	RevokedAt    *time.Time
	RevokeReason *string
}

type AccessCheckLog struct {
	EventID    string
	ActorID    string
	CourseID   string
	LessonID   string
	Decision   string
	DenyReason string
	CheckedAt  time.Time
}

// PaymentHistoryEntry is a payment enriched with the course title for
// display in a buyer-facing transaction history.
type PaymentHistoryEntry struct {
	Payment
	CourseTitle string
}

type EntitlementRepository interface {
	// CreatePaymentAndGrant inserts the payment and access grant as a single
	// atomic unit: either both rows are created or neither is, so a failure
	// partway through (e.g. a concurrent checkout winning the active-grant
	// race) can never leave a "succeeded" payment with no matching grant.
	// Returns ErrPaymentAlreadyRecorded when txn_id was already processed
	// (idempotent webhook redelivery) and ErrAlreadyPurchased when the grant
	// insert loses the active-grant race.
	CreatePaymentAndGrant(ctx context.Context, p Payment, g AccessGrant) error
	// SettleRefund atomically marks the payment refunded, revokes its access
	// grant, and credits the buyer's wallet, so a mid-refund failure can
	// never revoke access or mark a payment refunded without the buyer
	// actually getting their money back. Idempotent: settling an
	// already-refunded payment returns its current state without crediting
	// the wallet twice.
	SettleRefund(ctx context.Context, txnID string) (Payment, float64, error)
	HasActiveGrant(ctx context.Context, actorID, courseID string) (bool, error)
	GetActiveGrant(ctx context.Context, actorID, courseID string) (AccessGrant, error)
	LogAccessCheck(ctx context.Context, l AccessCheckLog) error
	GetEnrolledCourses(ctx context.Context, actorID string) ([]Course, error)
	ListPayments(ctx context.Context, actorID string) ([]PaymentHistoryEntry, error)
}
