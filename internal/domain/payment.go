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

// Payment is a single processed transaction against the sandbox payment
// gateway, keyed by the gateway's txn_id (the idempotency key for webhook
// redelivery — see ErrPaymentAlreadyRecorded).
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

// AccessGrant records a buyer's entitlement to a course. RevokedAt/RevokeReason
// are nil while active; RequireEntitlement only honors grants where
// RevokedAt is nil.
type AccessGrant struct {
	GrantID      string
	ActorID      string
	CourseID     string
	TxnID        string
	GrantedAt    time.Time
	RevokedAt    *time.Time
	RevokeReason *string
}

// AccessCheckLog is an audit row written by the RequireEntitlement middleware
// for every player request, allow or deny, so entitlement decisions are
// independently reviewable after the fact.
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

// EntitlementRepository is the transactional source of truth for who owns
// access to what: it backs the purchase/refund flow and the
// RequireEntitlement middleware guarding player endpoints.
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
