package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

// paymentStubEntRepo implements domain.EntitlementRepository for payment handler tests.
type paymentStubEntRepo struct {
	payment        domain.Payment
	grant          domain.AccessGrant
	createPayErr   error
	createGrantErr error
	getPayErr      error
	getGrantErr    error
	hasActiveGrant bool
	history        []domain.PaymentHistoryEntry
	historyErr     error
}

func (s *paymentStubEntRepo) CreatePayment(_ context.Context, p domain.Payment) error {
	s.payment = p
	return s.createPayErr
}

func (s *paymentStubEntRepo) GetPayment(_ context.Context, _ string) (domain.Payment, error) {
	return s.payment, s.getPayErr
}

func (s *paymentStubEntRepo) UpdatePaymentStatus(_ context.Context, _, _ string) error { return nil }

func (s *paymentStubEntRepo) CreateGrant(_ context.Context, g domain.AccessGrant) error {
	s.grant = g
	return s.createGrantErr
}

func (s *paymentStubEntRepo) RevokeGrant(_ context.Context, _, _ string) error { return nil }

func (s *paymentStubEntRepo) HasActiveGrant(_ context.Context, _, _ string) (bool, error) {
	return s.hasActiveGrant, nil
}

func (s *paymentStubEntRepo) GetActiveGrant(_ context.Context, _, _ string) (domain.AccessGrant, error) {
	return s.grant, s.getGrantErr
}

func (s *paymentStubEntRepo) LogAccessCheck(_ context.Context, _ domain.AccessCheckLog) error {
	return nil
}

func (s *paymentStubEntRepo) GetEnrolledCourses(_ context.Context, _ string) ([]domain.Course, error) {
	return nil, nil
}

func (s *paymentStubEntRepo) ListPayments(_ context.Context, _ string) ([]domain.PaymentHistoryEntry, error) {
	return s.history, s.historyErr
}

// paymentStubCatRepo implements domain.CatalogRepository for payment handler tests.
type paymentStubCatRepo struct {
	course    domain.Course
	courseErr error
}

func (s *paymentStubCatRepo) GetCourses(_ domain.CourseListParams) ([]domain.Course, int, error) {
	return nil, 0, nil
}

func (s *paymentStubCatRepo) GetByID(_ context.Context, _ string) (domain.Course, error) {
	return s.course, s.courseErr
}

func (s *paymentStubCatRepo) GetByTeacherID(_ context.Context, _ string) ([]domain.Course, error) {
	return nil, nil
}

// paymentStubUserRepo implements domain.UserRepository for payment handler tests.
type paymentStubUserRepo struct {
	balance   float64
	deductErr error
}

func (s *paymentStubUserRepo) Create(_ domain.User) (domain.User, error) { return domain.User{}, nil }

func (s *paymentStubUserRepo) GetByEmail(_ string) (domain.User, error) { return domain.User{}, nil }

func (s *paymentStubUserRepo) GetByID(_ string) (domain.User, error) { return domain.User{}, nil }

func (s *paymentStubUserRepo) DeductBalance(_ context.Context, _ string, _ float64) (float64, error) {
	return s.balance, s.deductErr
}

func (s *paymentStubUserRepo) CreditBalance(_ context.Context, _ string, _ float64) (float64, error) {
	return s.balance, nil
}

func (s *paymentStubUserRepo) UpdateEmail(_ context.Context, _, _ string) (domain.User, error) {
	return domain.User{}, nil
}

func (s *paymentStubUserRepo) UpdatePasswordHash(_ context.Context, _, _ string) error { return nil }

func (s *paymentStubUserRepo) UpdateFullName(_ context.Context, _, _ string) (domain.User, error) {
	return domain.User{}, nil
}

func (s *paymentStubUserRepo) UpdateAvatarURL(_ context.Context, _, _ string) (domain.User, error) {
	return domain.User{}, nil
}

func newPurchaseHandler(ent *paymentStubEntRepo, cat *paymentStubCatRepo, usr *paymentStubUserRepo) *PurchaseHandler {
	uc := usecase.NewPaymentUseCase(ent, cat, usr)
	return NewPurchaseHandler(uc, noopRecorder())
}

func purchasePostRequest(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "http://x/", strings.NewReader(body))
	return r.WithContext(context.WithValue(r.Context(), claimsContextKey, domain.Claims{UserID: "user-1"}))
}

// --- Checkout ---

func TestCheckoutHandler_Success(t *testing.T) {
	cat := &paymentStubCatRepo{course: domain.Course{ID: "c1", Price: 49.99, Currency: "USD"}}
	usr := &paymentStubUserRepo{balance: 100}
	ent := &paymentStubEntRepo{}
	h := newPurchaseHandler(ent, cat, usr)

	w := httptest.NewRecorder()
	h.Checkout(w, purchasePostRequest(`{"course_id":"c1"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp checkoutResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "succeeded" {
		t.Errorf("status = %q, want succeeded", resp.Status)
	}
	if resp.TransactionID == "" {
		t.Errorf("expected non-empty transaction_id")
	}
}

func TestCheckoutHandler_MissingAuth(t *testing.T) {
	h := newPurchaseHandler(&paymentStubEntRepo{}, &paymentStubCatRepo{}, &paymentStubUserRepo{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "http://x/", strings.NewReader(`{"course_id":"c1"}`))
	h.Checkout(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestCheckoutHandler_InvalidBody(t *testing.T) {
	h := newPurchaseHandler(&paymentStubEntRepo{}, &paymentStubCatRepo{}, &paymentStubUserRepo{})
	w := httptest.NewRecorder()
	h.Checkout(w, purchasePostRequest("{bad json"))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestCheckoutHandler_MissingCourseID(t *testing.T) {
	h := newPurchaseHandler(&paymentStubEntRepo{}, &paymentStubCatRepo{}, &paymentStubUserRepo{})
	w := httptest.NewRecorder()
	h.Checkout(w, purchasePostRequest(`{}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestCheckoutHandler_CourseNotFound(t *testing.T) {
	cat := &paymentStubCatRepo{courseErr: domain.ErrCourseNotFound}
	h := newPurchaseHandler(&paymentStubEntRepo{}, cat, &paymentStubUserRepo{})
	w := httptest.NewRecorder()
	h.Checkout(w, purchasePostRequest(`{"course_id":"missing"}`))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestCheckoutHandler_InsufficientFunds(t *testing.T) {
	cat := &paymentStubCatRepo{course: domain.Course{ID: "c1", Price: 999}}
	usr := &paymentStubUserRepo{deductErr: domain.ErrInsufficientFunds}
	h := newPurchaseHandler(&paymentStubEntRepo{}, cat, usr)
	w := httptest.NewRecorder()
	h.Checkout(w, purchasePostRequest(`{"course_id":"c1"}`))
	if w.Code != http.StatusPaymentRequired {
		t.Errorf("status = %d, want 402", w.Code)
	}
}

func TestCheckoutHandler_AlreadyPurchased(t *testing.T) {
	cat := &paymentStubCatRepo{course: domain.Course{ID: "c1", Price: 49.99}}
	ent := &paymentStubEntRepo{hasActiveGrant: true}
	h := newPurchaseHandler(ent, cat, &paymentStubUserRepo{balance: 100})
	w := httptest.NewRecorder()
	h.Checkout(w, purchasePostRequest(`{"course_id":"c1"}`))
	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", w.Code)
	}
}

// --- Refund ---

func TestRefundHandler_Success(t *testing.T) {
	payment := domain.Payment{TxnID: "txn-1", Amount: 49.99, Currency: "USD", CourseID: "c1", Status: "succeeded"}
	grant := domain.AccessGrant{TxnID: "txn-1", CourseID: "c1"}
	ent := &paymentStubEntRepo{payment: payment, grant: grant}
	usr := &paymentStubUserRepo{balance: 149.99}
	h := newPurchaseHandler(ent, &paymentStubCatRepo{}, usr)

	w := httptest.NewRecorder()
	h.Refund(w, purchasePostRequest(`{"course_id":"c1"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp refundResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "refunded" {
		t.Errorf("status = %q, want refunded", resp.Status)
	}
}

func TestRefundHandler_MissingAuth(t *testing.T) {
	h := newPurchaseHandler(&paymentStubEntRepo{}, &paymentStubCatRepo{}, &paymentStubUserRepo{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "http://x/", strings.NewReader(`{"course_id":"c1"}`))
	h.Refund(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestRefundHandler_MissingCourseID(t *testing.T) {
	h := newPurchaseHandler(&paymentStubEntRepo{}, &paymentStubCatRepo{}, &paymentStubUserRepo{})
	w := httptest.NewRecorder()
	h.Refund(w, purchasePostRequest(`{}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestRefundHandler_GrantNotFound(t *testing.T) {
	ent := &paymentStubEntRepo{getGrantErr: domain.ErrGrantNotFound}
	h := newPurchaseHandler(ent, &paymentStubCatRepo{}, &paymentStubUserRepo{})
	w := httptest.NewRecorder()
	h.Refund(w, purchasePostRequest(`{"course_id":"c1"}`))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// --- Webhook ---

func TestWebhookHandler_Success(t *testing.T) {
	ent := &paymentStubEntRepo{}
	h := newPurchaseHandler(ent, &paymentStubCatRepo{}, &paymentStubUserRepo{})
	w := httptest.NewRecorder()
	body := `{"transaction_id":"txn-1","status":"SUCCESS","user_id":"user-1","course_id":"c1"}`
	r := httptest.NewRequest(http.MethodPost, "http://x/", strings.NewReader(body))
	h.Webhook(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestWebhookHandler_Refunded(t *testing.T) {
	payment := domain.Payment{TxnID: "txn-1", Amount: 50, ActorID: "user-1"}
	ent := &paymentStubEntRepo{payment: payment}
	h := newPurchaseHandler(ent, &paymentStubCatRepo{}, &paymentStubUserRepo{balance: 100})
	w := httptest.NewRecorder()
	body := `{"transaction_id":"txn-1","status":"REFUNDED","user_id":"user-1","course_id":"c1"}`
	r := httptest.NewRequest(http.MethodPost, "http://x/", strings.NewReader(body))
	h.Webhook(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestWebhookHandler_UnknownStatus(t *testing.T) {
	h := newPurchaseHandler(&paymentStubEntRepo{}, &paymentStubCatRepo{}, &paymentStubUserRepo{})
	w := httptest.NewRecorder()
	body := `{"transaction_id":"txn-1","status":"PENDING","user_id":"user-1","course_id":"c1"}`
	r := httptest.NewRequest(http.MethodPost, "http://x/", strings.NewReader(body))
	h.Webhook(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestWebhookHandler_InvalidBody(t *testing.T) {
	h := newPurchaseHandler(&paymentStubEntRepo{}, &paymentStubCatRepo{}, &paymentStubUserRepo{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "http://x/", strings.NewReader("{bad json"))
	h.Webhook(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// --- History ---

func TestHistoryHandler_Success(t *testing.T) {
	entries := []domain.PaymentHistoryEntry{
		{Payment: domain.Payment{TxnID: "txn-1", CourseID: "c1", Amount: 49.99, Currency: "USD", Status: "succeeded"}, CourseTitle: "Go Mastery"},
	}
	ent := &paymentStubEntRepo{history: entries}
	h := newPurchaseHandler(ent, &paymentStubCatRepo{}, &paymentStubUserRepo{})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	r = r.WithContext(context.WithValue(r.Context(), claimsContextKey, domain.Claims{UserID: "user-1"}))
	h.History(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp historyResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Transactions) != 1 || resp.Transactions[0].CourseTitle != "Go Mastery" {
		t.Errorf("transactions = %+v, want [Go Mastery entry]", resp.Transactions)
	}
}

func TestHistoryHandler_MissingAuth(t *testing.T) {
	h := newPurchaseHandler(&paymentStubEntRepo{}, &paymentStubCatRepo{}, &paymentStubUserRepo{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	h.History(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}
