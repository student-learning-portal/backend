package http

import (
	"net/http"

	"github.com/student-learning-portal/backend/internal/usecase"
)

type UserCoursesHandler struct {
	uc *usecase.UserCoursesUseCase
}

func NewUserCoursesHandler(uc *usecase.UserCoursesUseCase) *UserCoursesHandler {
	return &UserCoursesHandler{uc: uc}
}

// MyCourses handles GET /api/v1/users/me/courses.
// Returns courses the caller owns (teacher) or is enrolled in (student).
func (h *UserCoursesHandler) MyCourses(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	courses, err := h.uc.MyCourses(r.Context(), claims.UserID, claims.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load courses")
		return
	}

	writeJSON(w, http.StatusOK, courses)
}
