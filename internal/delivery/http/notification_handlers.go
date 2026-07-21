package http

import (
	"net/http"
	"strconv"

	"github.com/student-learning-portal/backend/internal/usecase"
)

// NotificationHandler serves the authenticated user's in-app "bell" feed. Every
// route reads the caller's id from the verified token claims, so a user only
// ever sees or mutates their own notifications.
type NotificationHandler struct {
	uc *usecase.NotificationUseCase
}

func NewNotificationHandler(uc *usecase.NotificationUseCase) *NotificationHandler {
	return &NotificationHandler{uc: uc}
}

// defaultListLimit caps the feed when the client omits ?limit; maxListLimit
// clamps an over-large client value.
const (
	defaultListLimit = 30
	maxListLimit     = 100
)

// requireUser pulls the caller's verified claims from the request context,
// writing a 401 and returning ok=false when they are absent. Every route is
// scoped to claims.UserID, so this is the only authorization the feed needs.
func (h *NotificationHandler) requireUser(w http.ResponseWriter, r *http.Request) (string, bool) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return "", false
	}
	return claims.UserID, true
}

// List handles GET /api/v1/notifications[?limit=N] — the caller's notifications,
// newest first.
func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	limit := defaultListLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	items, err := h.uc.List(r.Context(), userID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load notifications")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// UnreadCount handles GET /api/v1/notifications/unread-count — the number behind
// the bell's badge.
func (h *NotificationHandler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	count, err := h.uc.UnreadCount(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count notifications")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"unread": count})
}

// MarkRead handles POST /api/v1/notifications/{id}/read — stamp one read.
func (h *NotificationHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "notification id is required")
		return
	}
	if err := h.uc.MarkRead(r.Context(), userID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update notification")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MarkAllRead handles POST /api/v1/notifications/read-all — stamp every unread
// notification read.
func (h *NotificationHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	if err := h.uc.MarkAllRead(r.Context(), userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update notifications")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
