package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
)

// stubPaymentUserRepo implements domain.UserRepository with balance control for payment tests.
type stubPaymentUserRepo struct {
	balance      float64
	deductErr    error
	creditErr    error
	deductCalled bool
	creditCalled bool
}

func (s *stubPaymentUserRepo) Create(_ domain.User) (domain.User, error) { return domain.User{}, nil }

func (s *stubPaymentUserRepo) GetByEmail(_ string) (domain.User, error) { return domain.User{}, nil }

func (s *stubPaymentUserRepo) GetByID(_ string) (domain.User, error) { return domain.User{}, nil }

func (s *stubPaymentUserRepo) UpdateEmail(_ context.Context, _, _ string) (domain.User, error) {
	return domain.User{}, nil
}

func (s *stubPaymentUserRepo) UpdatePasswordHash(_ context.Context, _, _ string) error { return nil }

func (s *stubPaymentUserRepo) UpdateFullName(_ context.Context, _, _ string) (domain.User, error) {
	return domain.User{}, nil
}

func (s *stubPaymentUserRepo) UpdateAvatarURL(_ context.Context, _, _ string) (domain.User, error) {
	return domain.User{}, nil
}

func (s *stubPaymentUserRepo) DeductBalance(_ context.Context, _ string, _ float64) (float64, error) {
	s.deductCalled = true
	if s.deductErr != nil {
		return 0, s.deductErr
	}
	return s.balance, nil
}

func (s *stubPaymentUserRepo) CreditBalance(_ context.Context, _ string, _ float64) (float64, error) {
	s.creditCalled = true
	if s.creditErr != nil {
		return 0, s.creditErr
	}
	return s.balance, nil
}

// stubEntitlementRepo implements domain.EntitlementRepository for payment tests.
type stubEntitlementRepo struct {
	payment        domain.Payment
	grant          domain.AccessGrant
	createPayErr   error
	createGrantErr error
	getPayErr      error
	getGrantErr    error
	updateErr      error
	revokeErr      error
	hasActiveGrant bool
	hasActiveErr   error
	history        []domain.PaymentHistoryEntry
	historyErr     error
}

func (s *stubEntitlementRepo) CreatePayment(_ context.Context, p domain.Payment) error {
	s.payment = p
	return s.createPayErr
}

func (s *stubEntitlementRepo) GetPayment(_ context.Context, _ string) (domain.Payment, error) {
	return s.payment, s.getPayErr
}

func (s *stubEntitlementRepo) UpdatePaymentStatus(_ context.Context, _, _ string) error {
	return s.updateErr
}

func (s *stubEntitlementRepo) CreateGrant(_ context.Context, g domain.AccessGrant) error {
	s.grant = g
	return s.createGrantErr
}

func (s *stubEntitlementRepo) RevokeGrant(_ context.Context, _, _ string) error { return s.revokeErr }

func (s *stubEntitlementRepo) HasActiveGrant(_ context.Context, _, _ string) (bool, error) {
	return s.hasActiveGrant, s.hasActiveErr
}

func (s *stubEntitlementRepo) ListPayments(_ context.Context, _ string) ([]domain.PaymentHistoryEntry, error) {
	return s.history, s.historyErr
}

func (s *stubEntitlementRepo) GetActiveGrant(_ context.Context, _, _ string) (domain.AccessGrant, error) {
	return s.grant, s.getGrantErr
}

func (s *stubEntitlementRepo) LogAccessCheck(_ context.Context, _ domain.AccessCheckLog) error {
	return nil
}

func (s *stubEntitlementRepo) GetEnrolledCourses(_ context.Context, _ string) ([]domain.Course, error) {
	return nil, nil
}

func newPaymentUC(ent *stubEntitlementRepo, cat *stubCatalogRepository, usr *stubPaymentUserRepo) *PaymentUseCase {
	return NewPaymentUseCase(ent, cat, usr)
}

// --- Checkout ---

func TestCheckout_Success(t *testing.T) {
	cat := &stubCatalogRepository{course: domain.Course{ID: "c1", Price: 49.99, Currency: "USD"}}
	usr := &stubPaymentUserRepo{balance: 50.01}
	ent := &stubEntitlementRepo{}
	uc := newPaymentUC(ent, cat, usr)

	res, err := uc.Checkout(context.Background(), "user-1", "c1")
	if err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if res.Payment.Status != "succeeded" {
		t.Errorf("status = %q, want succeeded", res.Payment.Status)
	}
	if res.Payment.CourseID != "c1" {
		t.Errorf("course_id = %q, want c1", res.Payment.CourseID)
	}
	if !res.Payment.Sandbox {
		t.Errorf("expected sandbox=true")
	}
	if res.PaymentSessionID == "" {
		t.Errorf("expected non-empty payment_session_id")
	}
	if res.RedirectURL == "" {
		t.Errorf("expected non-empty redirect_url")
	}
}

func TestCheckout_CourseNotFound(t *testing.T) {
	cat := &stubCatalogRepository{courseErr: domain.ErrCourseNotFound}
	uc := newPaymentUC(&stubEntitlementRepo{}, cat, &stubPaymentUserRepo{})
	_, err := uc.Checkout(context.Background(), "user-1", "missing")
	if !errors.Is(err, domain.ErrCourseNotFound) {
		t.Errorf("err = %v, want ErrCourseNotFound", err)
	}
}

func TestCheckout_InsufficientFunds(t *testing.T) {
	cat := &stubCatalogRepository{course: domain.Course{ID: "c1", Price: 100}}
	usr := &stubPaymentUserRepo{deductErr: domain.ErrInsufficientFunds}
	uc := newPaymentUC(&stubEntitlementRepo{}, cat, usr)
	_, err := uc.Checkout(context.Background(), "user-1", "c1")
	if !errors.Is(err, domain.ErrInsufficientFunds) {
		t.Errorf("err = %v, want ErrInsufficientFunds", err)
	}
}

func TestCheckout_PaymentCreateFails(t *testing.T) {
	cat := &stubCatalogRepository{course: domain.Course{ID: "c1", Price: 10}}
	ent := &stubEntitlementRepo{createPayErr: errors.New("db error")}
	uc := newPaymentUC(ent, cat, &stubPaymentUserRepo{balance: 90})
	_, err := uc.Checkout(context.Background(), "user-1", "c1")
	if err == nil {
		t.Fatal("expected error when payment creation fails")
	}
}

func TestCheckout_GrantCreateFails(t *testing.T) {
	cat := &stubCatalogRepository{course: domain.Course{ID: "c1", Price: 10}}
	ent := &stubEntitlementRepo{createGrantErr: errors.New("grant error")}
	uc := newPaymentUC(ent, cat, &stubPaymentUserRepo{balance: 90})
	_, err := uc.Checkout(context.Background(), "user-1", "c1")
	if err == nil {
		t.Fatal("expected error when grant creation fails")
	}
}

func TestCheckout_AlreadyOwned_FastPathSkipsCharge(t *testing.T) {
	cat := &stubCatalogRepository{course: domain.Course{ID: "c1", Price: 49.99}}
	usr := &stubPaymentUserRepo{balance: 100}
	ent := &stubEntitlementRepo{hasActiveGrant: true}
	uc := newPaymentUC(ent, cat, usr)

	_, err := uc.Checkout(context.Background(), "user-1", "c1")
	if !errors.Is(err, domain.ErrAlreadyPurchased) {
		t.Errorf("err = %v, want ErrAlreadyPurchased", err)
	}
	if usr.deductCalled {
		t.Errorf("expected DeductBalance not to be called on the fast path")
	}
}

func TestCheckout_GrantRaceLost_RefundsAndReturnsAlreadyPurchased(t *testing.T) {
	cat := &stubCatalogRepository{course: domain.Course{ID: "c1", Price: 49.99}}
	usr := &stubPaymentUserRepo{balance: 50.01}
	ent := &stubEntitlementRepo{createGrantErr: domain.ErrAlreadyPurchased}
	uc := newPaymentUC(ent, cat, usr)

	_, err := uc.Checkout(context.Background(), "user-1", "c1")
	if !errors.Is(err, domain.ErrAlreadyPurchased) {
		t.Errorf("err = %v, want ErrAlreadyPurchased", err)
	}
	if !usr.creditCalled {
		t.Errorf("expected the buyer to be refunded after losing the grant race")
	}
}

// --- Refund ---

func TestRefund_Success(t *testing.T) {
	payment := domain.Payment{TxnID: "txn-1", Amount: 49.99, Currency: "USD", CourseID: "c1", Status: "succeeded"}
	grant := domain.AccessGrant{TxnID: "txn-1", ActorID: "user-1", CourseID: "c1"}
	ent := &stubEntitlementRepo{payment: payment, grant: grant}
	usr := &stubPaymentUserRepo{balance: 149.99}
	uc := newPaymentUC(ent, &stubCatalogRepository{}, usr)

	res, err := uc.Refund(context.Background(), "user-1", "c1")
	if err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if res.Payment.Status != "refunded" {
		t.Errorf("status = %q, want refunded", res.Payment.Status)
	}
	if res.Balance != 149.99 {
		t.Errorf("balance = %v, want 149.99", res.Balance)
	}
}

func TestRefund_GrantNotFound(t *testing.T) {
	ent := &stubEntitlementRepo{getGrantErr: domain.ErrGrantNotFound}
	uc := newPaymentUC(ent, &stubCatalogRepository{}, &stubPaymentUserRepo{})
	_, err := uc.Refund(context.Background(), "user-1", "c1")
	if !errors.Is(err, domain.ErrGrantNotFound) {
		t.Errorf("err = %v, want ErrGrantNotFound", err)
	}
}

func TestRefund_PaymentNotFound(t *testing.T) {
	grant := domain.AccessGrant{TxnID: "txn-1"}
	ent := &stubEntitlementRepo{grant: grant, getPayErr: domain.ErrPaymentNotFound}
	uc := newPaymentUC(ent, &stubCatalogRepository{}, &stubPaymentUserRepo{})
	_, err := uc.Refund(context.Background(), "user-1", "c1")
	if !errors.Is(err, domain.ErrPaymentNotFound) {
		t.Errorf("err = %v, want ErrPaymentNotFound", err)
	}
}

// --- ProcessWebhook ---

func TestProcessWebhook_SuccessStatus(t *testing.T) {
	ent := &stubEntitlementRepo{}
	uc := newPaymentUC(ent, &stubCatalogRepository{}, &stubPaymentUserRepo{})
	err := uc.ProcessWebhook(context.Background(), "txn-1", "SUCCESS", "user-1", "c1")
	if err != nil {
		t.Fatalf("ProcessWebhook SUCCESS: %v", err)
	}
}

func TestProcessWebhook_RefundedStatus(t *testing.T) {
	payment := domain.Payment{TxnID: "txn-1", Amount: 50, ActorID: "user-1"}
	ent := &stubEntitlementRepo{payment: payment}
	usr := &stubPaymentUserRepo{balance: 100}
	uc := newPaymentUC(ent, &stubCatalogRepository{}, usr)
	err := uc.ProcessWebhook(context.Background(), "txn-1", "REFUNDED", "user-1", "c1")
	if err != nil {
		t.Fatalf("ProcessWebhook REFUNDED: %v", err)
	}
}

func TestProcessWebhook_UnknownStatus(t *testing.T) {
	uc := newPaymentUC(&stubEntitlementRepo{}, &stubCatalogRepository{}, &stubPaymentUserRepo{})
	err := uc.ProcessWebhook(context.Background(), "txn-1", "PENDING", "user-1", "c1")
	if err == nil {
		t.Fatal("expected error for unknown webhook status")
	}
}

func TestProcessWebhook_RefundedPaymentNotFound(t *testing.T) {
	ent := &stubEntitlementRepo{getPayErr: domain.ErrPaymentNotFound}
	uc := newPaymentUC(ent, &stubCatalogRepository{}, &stubPaymentUserRepo{})
	err := uc.ProcessWebhook(context.Background(), "txn-1", "REFUNDED", "user-1", "c1")
	if err == nil {
		t.Fatal("expected error when payment not found during refund webhook")
	}
}

// --- History ---

func TestHistory_Success(t *testing.T) {
	entries := []domain.PaymentHistoryEntry{
		{Payment: domain.Payment{TxnID: "txn-1", CourseID: "c1", Amount: 49.99, Status: "succeeded"}, CourseTitle: "Go Mastery"},
	}
	ent := &stubEntitlementRepo{history: entries}
	uc := newPaymentUC(ent, &stubCatalogRepository{}, &stubPaymentUserRepo{})

	got, err := uc.History(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(got) != 1 || got[0].CourseTitle != "Go Mastery" {
		t.Errorf("History = %+v, want [Go Mastery entry]", got)
	}
}

func TestHistory_RepoError(t *testing.T) {
	ent := &stubEntitlementRepo{historyErr: errors.New("db error")}
	uc := newPaymentUC(ent, &stubCatalogRepository{}, &stubPaymentUserRepo{})
	if _, err := uc.History(context.Background(), "user-1"); err == nil {
		t.Fatal("expected error when the repository fails")
	}
}
