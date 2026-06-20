package http

import (
	"encoding/json"
	"net/http"

	"github.com/student-learning-portal/backend/internal/usecase"
)

type PurchaseHandler struct {
	paymentUseCase *usecase.PaymentUseCase
}

func NewPurchaseHandler(uc *usecase.PaymentUseCase) *PurchaseHandler {
	return &PurchaseHandler{paymentUseCase: uc}
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

	payment, err := h.paymentUseCase.Checkout(r.Context(), claims.UserID, req.CourseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "checkout failed")
		return
	}

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

	w.WriteHeader(http.StatusOK)
}
