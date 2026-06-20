package domain

import (
	"context"
	"errors"
	"time"
)

var ErrPaymentNotFound = errors.New("payment not found")

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

type EntitlementRepository interface {
	CreatePayment(ctx context.Context, p Payment) error
	GetPayment(ctx context.Context, txnID string) (Payment, error)
	UpdatePaymentStatus(ctx context.Context, txnID, status string) error
	CreateGrant(ctx context.Context, g AccessGrant) error
	RevokeGrant(ctx context.Context, txnID, reason string) error
	HasActiveGrant(ctx context.Context, actorID, courseID string) (bool, error)
	LogAccessCheck(ctx context.Context, l AccessCheckLog) error
}
