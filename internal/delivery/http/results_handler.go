package http

import (
	"net/http"

	"github.com/student-learning-portal/backend/internal/usecase"
)

type ResultsHandler struct {
	uc *usecase.ResultsUseCase
}

func NewResultsHandler(uc *usecase.ResultsUseCase) *ResultsHandler {
	return &ResultsHandler{uc: uc}
}

// MyResults handles GET /api/v1/users/me/results. It returns the authenticated
// learner's completion percentage and lesson progress per enrolled course, plus
// an overall roll-up across all of them.
func (h *ResultsHandler) MyResults(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	results, err := h.uc.MyResults(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load results")
		return
	}

	writeJSON(w, http.StatusOK, results)
}
