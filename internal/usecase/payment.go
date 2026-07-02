package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/student-learning-portal/backend/internal/domain"
)

type PaymentUseCase struct {
	entitlements domain.EntitlementRepository
	courses      domain.CatalogRepository
	users        domain.UserRepository
}

func NewPaymentUseCase(
	entitlements domain.EntitlementRepository,
	courses domain.CatalogRepository,
	users domain.UserRepository,
) *PaymentUseCase {
	return &PaymentUseCase{entitlements: entitlements, courses: courses, users: users}
}

// CheckoutResult bundles the settled payment with the mock payment session
// details and the buyer's updated wallet balance.
type CheckoutResult struct {
	Payment          domain.Payment
	Balance          float64
	PaymentSessionID string
	RedirectURL      string
}

// Checkout creates a sandbox order for a course: it charges the course price
// against the buyer's virtual wallet, immediately settles the payment, grants
// course access, and returns a mock payment session/redirect (no real money
// or external gateway is involved).
func (uc *PaymentUseCase) Checkout(ctx context.Context, actorID, courseID string) (CheckoutResult, error) {
	course, err := uc.courses.GetByID(ctx, courseID)
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("checkout: %w", err)
	}

	// Fast path: avoid charging at all when the buyer already owns the
	// course (e.g. a double-click). The DB-level unique index on
	// access_grant is the authoritative guard for the concurrent case this
	// check can't catch (see CreateGrant below).
	has, err := uc.entitlements.HasActiveGrant(ctx, actorID, courseID)
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("checkout: %w", err)
	}
	if has {
		return CheckoutResult{}, domain.ErrAlreadyPurchased
	}

	balance, err := uc.users.DeductBalance(ctx, actorID, course.Price)
	if err != nil {
		return CheckoutResult{}, err
	}

	now := time.Now()
	payment := domain.Payment{
		TxnID:     uuid.NewString(),
		CartID:    uuid.NewString(),
		ActorID:   actorID,
		CourseID:  courseID,
		Amount:    course.Price,
		Currency:  course.Currency,
		Status:    "succeeded",
		Sandbox:   true,
		CreatedAt: now,
	}
	if err := uc.entitlements.CreatePayment(ctx, payment); err != nil {
		uc.refund(ctx, actorID, course.Price)
		return CheckoutResult{}, fmt.Errorf("checkout: %w", err)
	}
	grant := domain.AccessGrant{
		GrantID:   uuid.NewString(),
		ActorID:   actorID,
		CourseID:  courseID,
		TxnID:     payment.TxnID,
		GrantedAt: now,
	}
	if err := uc.entitlements.CreateGrant(ctx, grant); err != nil {
		uc.refund(ctx, actorID, course.Price)
		if errors.Is(err, domain.ErrAlreadyPurchased) {
			// Lost the race: a concurrent checkout for this course won the
			// DB constraint first. The buyer was already refunded above.
			return CheckoutResult{}, domain.ErrAlreadyPurchased
		}
		return CheckoutResult{}, fmt.Errorf("checkout: grant: %w", err)
	}

	return CheckoutResult{
		Payment:          payment,
		Balance:          balance,
		PaymentSessionID: "sandbox_" + payment.TxnID,
		RedirectURL:      "/sandbox/payments/" + payment.TxnID,
	}, nil
}

// refund best-effort credits the buyer back when settling the order fails
// after their wallet was already charged.
func (uc *PaymentUseCase) refund(ctx context.Context, actorID string, amount float64) {
	_, _ = uc.users.CreditBalance(ctx, actorID, amount)
}

// RefundResult bundles the reversed payment with the buyer's updated wallet balance.
type RefundResult struct {
	Payment domain.Payment
	Balance float64
}

// Refund returns a purchased course: it revokes the buyer's access grant,
// marks the sandbox payment as refunded, and credits the full amount back to
// their virtual wallet.
func (uc *PaymentUseCase) Refund(ctx context.Context, actorID, courseID string) (RefundResult, error) {
	grant, err := uc.entitlements.GetActiveGrant(ctx, actorID, courseID)
	if err != nil {
		return RefundResult{}, fmt.Errorf("refund: %w", err)
	}
	payment, err := uc.entitlements.GetPayment(ctx, grant.TxnID)
	if err != nil {
		return RefundResult{}, fmt.Errorf("refund: %w", err)
	}
	if err = uc.entitlements.UpdatePaymentStatus(ctx, grant.TxnID, "refunded"); err != nil {
		return RefundResult{}, fmt.Errorf("refund: %w", err)
	}
	if err = uc.entitlements.RevokeGrant(ctx, grant.TxnID, "refund"); err != nil {
		return RefundResult{}, fmt.Errorf("refund: %w", err)
	}
	balance, err := uc.users.CreditBalance(ctx, actorID, payment.Amount)
	if err != nil {
		return RefundResult{}, fmt.Errorf("refund: credit: %w", err)
	}
	payment.Status = "refunded"
	return RefundResult{Payment: payment, Balance: balance}, nil
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
		payment, err := uc.entitlements.GetPayment(ctx, txnID)
		if err != nil {
			return fmt.Errorf("webhook refund: %w", err)
		}
		if err := uc.entitlements.UpdatePaymentStatus(ctx, txnID, "refunded"); err != nil {
			return fmt.Errorf("webhook refund: %w", err)
		}
		if err := uc.entitlements.RevokeGrant(ctx, txnID, "refund"); err != nil {
			return fmt.Errorf("webhook revoke: %w", err)
		}
		if _, err := uc.users.CreditBalance(ctx, payment.ActorID, payment.Amount); err != nil {
			return fmt.Errorf("webhook credit: %w", err)
		}
		return nil

	default:
		return fmt.Errorf("unknown webhook status: %s", status)
	}
}

// History returns the buyer's full payment history, newest first.
func (uc *PaymentUseCase) History(ctx context.Context, actorID string) ([]domain.PaymentHistoryEntry, error) {
	entries, err := uc.entitlements.ListPayments(ctx, actorID)
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	return entries, nil
}
