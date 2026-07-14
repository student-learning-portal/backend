package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

// ReviewHandler serves a course's public rating summary and lets students
// leave a review. Both proxy to the practicum-team integration
// (internal/practicum) — eligibility (enrollment + progress) is enforced by
// their service, not here.
type ReviewHandler struct {
	uc *usecase.ReviewUseCase
}

func NewReviewHandler(uc *usecase.ReviewUseCase) *ReviewHandler {
	return &ReviewHandler{uc: uc}
}

type createReviewRequest struct {
	Rating int    `json:"rating"`
	Text   string `json:"text"`
}

type courseReviewResponse struct {
	ID        string    `json:"id"`
	CourseID  string    `json:"course_id"`
	StudentID string    `json:"student_id"`
	Rating    int       `json:"rating"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateReview handles POST /api/v1/catalog/courses/{course_id}/comments.
// Student only; the practicum-team service enforces enrollment + >50%
// lesson-completion and one-review-per-student (see usecase.ReviewUseCase).
func (h *ReviewHandler) CreateReview(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}
	if claims.Role != domain.RoleStudent {
		writeError(w, http.StatusForbidden, "only students may review courses")
		return
	}

	courseID := r.PathValue(keyCourseID)

	var req createReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	review, err := h.uc.CreateReview(r.Context(), claims.UserID, courseID, req.Rating, req.Text)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidReview):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, domain.ErrCourseNotFound):
			writeError(w, http.StatusNotFound, "course not found")
		case errors.Is(err, domain.ErrNotEnrolled):
			writeError(w, http.StatusForbidden, err.Error())
		case errors.Is(err, domain.ErrInsufficientProgress):
			writeError(w, http.StatusForbidden, err.Error())
		case errors.Is(err, domain.ErrReviewAlreadyExists):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "failed to create review")
		}
		return
	}

	writeJSON(w, http.StatusCreated, courseReviewResponse{
		ID:        review.ID,
		CourseID:  review.CourseID,
		StudentID: review.StudentID,
		Rating:    review.Rating,
		Text:      review.Text,
		CreatedAt: review.CreatedAt,
	})
}

type courseRatingResponse struct {
	AverageRating float64 `json:"average_rating"`
	ReviewsCount  int     `json:"reviews_count"`
}

// RatingSummary handles GET /api/v1/catalog/courses/{course_id}/rating. Public.
func (h *ReviewHandler) RatingSummary(w http.ResponseWriter, r *http.Request) {
	courseID := r.PathValue(keyCourseID)

	summary, err := h.uc.RatingSummary(r.Context(), courseID)
	if err != nil {
		if errors.Is(err, domain.ErrCourseNotFound) {
			writeError(w, http.StatusNotFound, "course not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load course rating")
		return
	}

	writeJSON(w, http.StatusOK, courseRatingResponse{
		AverageRating: summary.AverageRating,
		ReviewsCount:  summary.ReviewsCount,
	})
}
