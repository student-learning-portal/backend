package http

import (
	"encoding/json"
	"net/http"

	"github.com/student-learning-portal/backend/internal/domain"
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
	TransactionID string `json:"transaction_id"`
	Status        string `json:"status"`
}

// Checkout handles POST /api/v1/purchase/checkout
func (h *PurchaseHandler) Checkout(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
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
		"course_id": req.CourseID,
	})

	payment, err := h.paymentUseCase.Checkout(r.Context(), claims.UserID, req.CourseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "checkout failed")
		return
	}

	// Sandbox checkout settles immediately and opens access (see PaymentUseCase),
	// so the payment-cleared and access-opened events fire together here.
	h.analytics.Record(r.Context(), domain.EventAccessPaymentSucceeded, domain.PIINone, map[string]any{
		"course_id": payment.CourseID,
		"cart_id":   payment.CartID,
		"txn_id":    payment.TxnID,
		"amount":    payment.Amount,
		"currency":  payment.Currency,
		"sandbox":   payment.Sandbox,
	})
	h.analytics.Record(r.Context(), domain.EventAccessGranted, domain.PIINone, map[string]any{
		"course_id": payment.CourseID,
		"txn_id":    payment.TxnID,
		"reason":    "purchase",
	})

	writeJSON(w, http.StatusOK, checkoutResponse{
		TransactionID: payment.TxnID,
		Status:        payment.Status,
	})
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
		writeError(w, http.StatusBadRequest, err.Error())
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
			"course_id": req.CourseID,
			"txn_id":    req.TransactionID,
		})
		h.analytics.Record(ctx, domain.EventAccessGranted, domain.PIINone, map[string]any{
			"course_id": req.CourseID,
			"txn_id":    req.TransactionID,
			"reason":    "purchase",
		})
	case "REFUNDED":
		h.analytics.Record(ctx, domain.EventAccessRefundCompleted, domain.PIINone, map[string]any{
			"course_id": req.CourseID,
			"txn_id":    req.TransactionID,
		})
		h.analytics.Record(ctx, domain.EventAccessRevoked, domain.PIINone, map[string]any{
			"course_id": req.CourseID,
			"txn_id":    req.TransactionID,
			"reason":    "refund",
		})
	}
}
