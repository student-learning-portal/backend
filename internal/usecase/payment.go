package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/student-learning-portal/backend/internal/domain"
)

type PaymentUseCase struct {
	entitlements domain.EntitlementRepository
}

func NewPaymentUseCase(entitlements domain.EntitlementRepository) *PaymentUseCase {
	return &PaymentUseCase{entitlements: entitlements}
}

// Checkout creates a sandbox payment (immediately succeeded) and grants course access.
func (uc *PaymentUseCase) Checkout(ctx context.Context, actorID, courseID string) (domain.Payment, error) {
	now := time.Now()
	payment := domain.Payment{
		TxnID:     uuid.NewString(),
		CartID:    uuid.NewString(),
		ActorID:   actorID,
		CourseID:  courseID,
		Amount:    0,
		Currency:  "USD",
		Status:    "succeeded",
		Sandbox:   true,
		CreatedAt: now,
	}
	if err := uc.entitlements.CreatePayment(ctx, payment); err != nil {
		return domain.Payment{}, fmt.Errorf("checkout: %w", err)
	}
	grant := domain.AccessGrant{
		GrantID:   uuid.NewString(),
		ActorID:   actorID,
		CourseID:  courseID,
		TxnID:     payment.TxnID,
		GrantedAt: now,
	}
	if err := uc.entitlements.CreateGrant(ctx, grant); err != nil {
		return domain.Payment{}, fmt.Errorf("checkout: grant: %w", err)
	}
	return payment, nil
}

// ProcessWebhook handles idempotent payment gateway callbacks (SUCCESS or REFUNDED).
func (uc *PaymentUseCase) ProcessWebhook(ctx context.Context, txnID, status, actorID, courseID string) error {
	switch status {
	case "SUCCESS":
		now := time.Now()
		p := domain.Payment{
			TxnID:     txnID,
			CartID:    uuid.NewString(),
			ActorID:   actorID,
			CourseID:  courseID,
			Amount:    0,
			Currency:  "USD",
			Status:    "succeeded",
			Sandbox:   true,
			CreatedAt: now,
		}
		// Ignore duplicate errors for idempotency.
		_ = uc.entitlements.CreatePayment(ctx, p)
		_ = uc.entitlements.CreateGrant(ctx, domain.AccessGrant{
			GrantID:   uuid.NewString(),
			ActorID:   actorID,
			CourseID:  courseID,
			TxnID:     txnID,
			GrantedAt: now,
		})
		return nil

	case "REFUNDED":
		if err := uc.entitlements.UpdatePaymentStatus(ctx, txnID, "refunded"); err != nil {
			return fmt.Errorf("webhook refund: %w", err)
		}
		if err := uc.entitlements.RevokeGrant(ctx, txnID, "refund"); err != nil {
			return fmt.Errorf("webhook revoke: %w", err)
		}
		return nil

	default:
		return fmt.Errorf("unknown webhook status: %s", status)
	}
}
