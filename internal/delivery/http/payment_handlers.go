package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/logging"
	"github.com/student-learning-portal/backend/internal/usecase"
)

type PurchaseHandler struct {
	paymentUseCase *usecase.PaymentUseCase
	analytics      *usecase.AnalyticsRecorder
}

func NewPurchaseHandler(uc *usecase.PaymentUseCase, analytics *usecase.AnalyticsRecorder) *PurchaseHandler {
	return &PurchaseHandler{paymentUseCase: uc, analytics: analytics}
}

type checkoutRequest struct {
	CourseID string `json:"course_id"`
}

type checkoutResponse struct {
	TransactionID    string  `json:"transaction_id"`
	Status           string  `json:"status"`
	Amount           float64 `json:"amount"`
	Currency         string  `json:"currency"`
	Balance          float64 `json:"balance"`
	PaymentSessionID string  `json:"payment_session_id"`
	RedirectURL      string  `json:"redirect_url"`
}

// Checkout handles POST /api/v1/purchase/checkout
func (h *PurchaseHandler) Checkout(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	if claims.Role == domain.RoleTeacher {
		writeError(w, http.StatusForbidden, "teachers cannot purchase courses")
		return
	}

	var req checkoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CourseID == "" {
		writeError(w, http.StatusBadRequest, "course_id is required")
		return
	}

	h.analytics.Record(r.Context(), domain.EventAccessCheckoutStart, domain.PIINone, map[string]any{
		keyCourseID: req.CourseID,
	})

	result, err := h.paymentUseCase.Checkout(r.Context(), claims.UserID, req.CourseID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrCourseNotFound):
			writeError(w, http.StatusNotFound, "course not found")
		case errors.Is(err, domain.ErrInsufficientFunds):
			writeError(w, http.StatusPaymentRequired, "insufficient wallet balance")
		case errors.Is(err, domain.ErrUserNotFound):
			writeError(w, http.StatusUnauthorized, "user not found")
		case errors.Is(err, domain.ErrAlreadyPurchased):
			writeError(w, http.StatusConflict, "course already purchased")
		default:
			writeError(w, http.StatusInternalServerError, "checkout failed")
		}
		return
	}
	payment := result.Payment

	// Sandbox checkout settles immediately and opens access (see PaymentUseCase),
	// so the payment-cleared and access-opened events fire together here.
	h.analytics.Record(r.Context(), domain.EventAccessPaymentSucceeded, domain.PIINone, map[string]any{
		keyCourseID: payment.CourseID,
		"cart_id":   payment.CartID,
		keyTxnID:    payment.TxnID,
		"amount":    payment.Amount,
		"currency":  payment.Currency,
		"sandbox":   payment.Sandbox,
	})
	h.analytics.Record(r.Context(), domain.EventAccessGranted, domain.PIINone, map[string]any{
		keyCourseID: payment.CourseID,
		keyTxnID:    payment.TxnID,
		"reason":    "purchase",
	})

	writeJSON(w, http.StatusOK, checkoutResponse{
		TransactionID:    payment.TxnID,
		Status:           payment.Status,
		Amount:           payment.Amount,
		Currency:         payment.Currency,
		Balance:          result.Balance,
		PaymentSessionID: result.PaymentSessionID,
		RedirectURL:      result.RedirectURL,
	})
}

type refundRequest struct {
	CourseID string `json:"course_id"`
}

type refundResponse struct {
	TransactionID string  `json:"transaction_id"`
	Status        string  `json:"status"`
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
	Balance       float64 `json:"balance"`
}

// Refund handles POST /api/v1/purchase/refund
func (h *PurchaseHandler) Refund(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	if claims.Role == domain.RoleTeacher {
		writeError(w, http.StatusForbidden, "teachers cannot refund courses")
		return
	}

	var req refundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CourseID == "" {
		writeError(w, http.StatusBadRequest, "course_id is required")
		return
	}

	result, err := h.paymentUseCase.Refund(r.Context(), claims.UserID, req.CourseID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrGrantNotFound):
			writeError(w, http.StatusNotFound, "no active purchase found for this course")
		case errors.Is(err, domain.ErrPaymentNotFound):
			writeError(w, http.StatusNotFound, "payment not found")
		default:
			writeError(w, http.StatusInternalServerError, "refund failed")
		}
		return
	}
	payment := result.Payment

	h.analytics.Record(r.Context(), domain.EventAccessRefundCompleted, domain.PIINone, map[string]any{
		keyCourseID: payment.CourseID,
		keyTxnID:    payment.TxnID,
		"amount":    payment.Amount,
		"currency":  payment.Currency,
	})
	h.analytics.Record(r.Context(), domain.EventAccessRevoked, domain.PIINone, map[string]any{
		keyCourseID: payment.CourseID,
		keyTxnID:    payment.TxnID,
		"reason":    "refund",
	})

	writeJSON(w, http.StatusOK, refundResponse{
		TransactionID: payment.TxnID,
		Status:        payment.Status,
		Amount:        payment.Amount,
		Currency:      payment.Currency,
		Balance:       result.Balance,
	})
}

type historyEntryResponse struct {
	TransactionID string    `json:"transaction_id"`
	CourseID      string    `json:"course_id"`
	CourseTitle   string    `json:"course_title"`
	Amount        float64   `json:"amount"`
	Currency      string    `json:"currency"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

type historyResponse struct {
	Transactions []historyEntryResponse `json:"transactions"`
}

// History handles GET /api/v1/purchase/history
func (h *PurchaseHandler) History(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	entries, err := h.paymentUseCase.History(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load payment history")
		return
	}

	transactions := make([]historyEntryResponse, 0, len(entries))
	for _, e := range entries {
		transactions = append(transactions, historyEntryResponse{
			TransactionID: e.TxnID,
			CourseID:      e.CourseID,
			CourseTitle:   e.CourseTitle,
			Amount:        e.Amount,
			Currency:      e.Currency,
			Status:        e.Status,
			CreatedAt:     e.CreatedAt,
		})
	}

	writeJSON(w, http.StatusOK, historyResponse{Transactions: transactions})
}

type webhookRequest struct {
	TransactionID string `json:"transaction_id"`
	Status        string `json:"status"`
	UserID        string `json:"user_id"`
	CourseID      string `json:"course_id"`
}

// Webhook handles POST /api/v1/purchase/webhook
func (h *PurchaseHandler) Webhook(w http.ResponseWriter, r *http.Request) {
	var req webhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.paymentUseCase.ProcessWebhook(r.Context(), req.TransactionID, req.Status, req.UserID, req.CourseID); err != nil {
		if errors.Is(err, domain.ErrUnknownWebhookStatus) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		// An internal/DB failure, not a bad request: return 5xx (not the raw
		// error text) so the gateway's retry logic kicks in instead of
		// treating this delivery as permanently accepted.
		logging.FromContext(r.Context()).Error("webhook processing failed",
			slog.String("txn_id", req.TransactionID),
			slog.String("status", req.Status),
			slog.Any("error", err),
		)
		writeError(w, http.StatusInternalServerError, "failed to process payment webhook")
		return
	}

	h.recordWebhook(r, req)

	w.WriteHeader(http.StatusOK)
}

// recordWebhook mirrors the processed gateway callback onto the analytics event
// stream. The webhook actor is the gateway itself, so the events are attributed
// to the user_id carried in the callback rather than a request session.
func (h *PurchaseHandler) recordWebhook(r *http.Request, req webhookRequest) {
	ctx := domain.ContextWithActor(r.Context(), domain.Actor{
		ActorID:   req.UserID,
		Role:      domain.RoleSystem,
		AuthState: domain.AuthStateAnonymous,
	})

	switch req.Status {
	case "SUCCESS":
		h.analytics.Record(ctx, domain.EventAccessPaymentSucceeded, domain.PIINone, map[string]any{
			keyCourseID: req.CourseID,
			keyTxnID:    req.TransactionID,
		})
		h.analytics.Record(ctx, domain.EventAccessGranted, domain.PIINone, map[string]any{
			keyCourseID: req.CourseID,
			keyTxnID:    req.TransactionID,
			"reason":    "purchase",
		})
	case "REFUNDED":
		h.analytics.Record(ctx, domain.EventAccessRefundCompleted, domain.PIINone, map[string]any{
			keyCourseID: req.CourseID,
			keyTxnID:    req.TransactionID,
		})
		h.analytics.Record(ctx, domain.EventAccessRevoked, domain.PIINone, map[string]any{
			keyCourseID: req.CourseID,
			keyTxnID:    req.TransactionID,
			"reason":    "refund",
		})
	}
}
