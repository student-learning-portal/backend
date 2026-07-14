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
	// check can't catch (see CreatePaymentAndGrant below).
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
		Status:    "succeeded", //nolint:goconst // sandbox always succeeds
		Sandbox:   true,
		CreatedAt: now,
	}
	grant := domain.AccessGrant{
		GrantID:   uuid.NewString(),
		ActorID:   actorID,
		CourseID:  courseID,
		TxnID:     payment.TxnID,
		GrantedAt: now,
	}
	// Payment and grant are created atomically (CreatePaymentAndGrant): a
	// failure here rolls back cleanly with no partial row persisted, so the
	// wallet refund below is the only compensation needed.
	if err := uc.entitlements.CreatePaymentAndGrant(ctx, payment, grant); err != nil {
		uc.refund(ctx, actorID, course.Price)
		if errors.Is(err, domain.ErrAlreadyPurchased) {
			// Lost the race: a concurrent checkout for this course won the
			// DB constraint first. The buyer was already refunded above.
			return CheckoutResult{}, domain.ErrAlreadyPurchased
		}
		return CheckoutResult{}, fmt.Errorf("checkout: %w", err)
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
	// SettleRefund atomically marks the payment refunded, revokes the grant,
	// and credits the wallet, so a mid-refund failure can never revoke
	// access without the buyer actually getting their money back.
	payment, balance, err := uc.entitlements.SettleRefund(ctx, grant.TxnID)
	if err != nil {
		return RefundResult{}, fmt.Errorf("refund: %w", err)
	}
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
			Currency:  currencyUSD,
			Status:    "succeeded",
			Sandbox:   true,
			CreatedAt: now,
		}
		g := domain.AccessGrant{
			GrantID:   uuid.NewString(),
			ActorID:   actorID,
			CourseID:  courseID,
			TxnID:     txnID,
			GrantedAt: now,
		}
		err := uc.entitlements.CreatePaymentAndGrant(ctx, p, g)
		if err == nil ||
			errors.Is(err, domain.ErrPaymentAlreadyRecorded) ||
			errors.Is(err, domain.ErrAlreadyPurchased) {
			// Idempotent: this event (or the grant it implies) was already
			// applied by an earlier delivery of the same webhook.
			return nil
		}
		return fmt.Errorf("webhook success: %w", err)

	case "REFUNDED":
		if _, _, err := uc.entitlements.SettleRefund(ctx, txnID); err != nil {
			return fmt.Errorf("webhook refund: %w", err)
		}
		return nil

	default:
		return fmt.Errorf("%w: %s", domain.ErrUnknownWebhookStatus, status)
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
