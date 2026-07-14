package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

// RatingHandler serves the local 1-10 rating system for courses and
// teachers. Unlike ReviewHandler (which proxies course reviews/ratings to
// the practicum-team service), ratings here are stored and aggregated
// locally — the practicum integration has no notion of teachers at all.
type RatingHandler struct {
	uc *usecase.RatingUseCase
}

func NewRatingHandler(uc *usecase.RatingUseCase) *RatingHandler {
	return &RatingHandler{uc: uc}
}

type rateRequest struct {
	Score int `json:"score"`
}

type ratingSummaryResponse struct {
	AverageScore float64 `json:"average_score"`
	RatingsCount int     `json:"ratings_count"`
}

func writeRatingError(w http.ResponseWriter, err error, notFoundMsg string) {
	switch {
	case errors.Is(err, domain.ErrInvalidRating):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrCourseNotFound), errors.Is(err, domain.ErrUserNotFound):
		writeError(w, http.StatusNotFound, notFoundMsg)
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, http.StatusForbidden, "you must be enrolled to rate this")
	default:
		writeError(w, http.StatusInternalServerError, "failed to save rating")
	}
}

type courseRatingRecordResponse struct {
	ID        string    `json:"id"`
	CourseID  string    `json:"course_id"`
	StudentID string    `json:"student_id"`
	Score     int       `json:"score"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RateCourse handles POST /api/v1/catalog/courses/{course_id}/ratings.
// Student only; the student must hold an active access grant for the course.
func (h *RatingHandler) RateCourse(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}
	if claims.Role != domain.RoleStudent {
		writeError(w, http.StatusForbidden, "only students may rate courses")
		return
	}

	courseID := r.PathValue(keyCourseID)

	var req rateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	rating, err := h.uc.RateCourse(r.Context(), claims.UserID, courseID, req.Score)
	if err != nil {
		writeRatingError(w, err, "course not found")
		return
	}

	writeJSON(w, http.StatusCreated, courseRatingRecordResponse{
		ID:        rating.ID,
		CourseID:  rating.CourseID,
		StudentID: rating.StudentID,
		Score:     rating.Score,
		CreatedAt: rating.CreatedAt,
		UpdatedAt: rating.UpdatedAt,
	})
}

// MyCourseRating handles GET /api/v1/catalog/courses/{course_id}/ratings/me.
// Student only; returns 404 if the caller hasn't rated this course yet.
func (h *RatingHandler) MyCourseRating(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}
	if claims.Role != domain.RoleStudent {
		writeError(w, http.StatusForbidden, "only students have course ratings")
		return
	}

	courseID := r.PathValue(keyCourseID)

	rating, err := h.uc.MyCourseRating(r.Context(), claims.UserID, courseID)
	if err != nil {
		if errors.Is(err, domain.ErrRatingNotFound) {
			writeError(w, http.StatusNotFound, "you have not rated this course yet")
			return
		}
		writeRatingError(w, err, "course not found")
		return
	}

	writeJSON(w, http.StatusOK, courseRatingRecordResponse{
		ID:        rating.ID,
		CourseID:  rating.CourseID,
		StudentID: rating.StudentID,
		Score:     rating.Score,
		CreatedAt: rating.CreatedAt,
		UpdatedAt: rating.UpdatedAt,
	})
}

// CourseRatingSummary handles GET /api/v1/catalog/courses/{course_id}/ratings. Public.
func (h *RatingHandler) CourseRatingSummary(w http.ResponseWriter, r *http.Request) {
	courseID := r.PathValue(keyCourseID)

	summary, err := h.uc.CourseRatingSummary(r.Context(), courseID)
	if err != nil {
		writeRatingError(w, err, "course not found")
		return
	}

	writeJSON(w, http.StatusOK, ratingSummaryResponse{
		AverageScore: summary.AverageScore,
		RatingsCount: summary.RatingsCount,
	})
}

type teacherRatingResponse struct {
	ID        string    `json:"id"`
	TeacherID string    `json:"teacher_id"`
	StudentID string    `json:"student_id"`
	Score     int       `json:"score"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RateTeacher handles POST /api/v1/teachers/{teacher_id}/ratings. Student
// only; the student must be enrolled in at least one course by this teacher.
func (h *RatingHandler) RateTeacher(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}
	if claims.Role != domain.RoleStudent {
		writeError(w, http.StatusForbidden, "only students may rate teachers")
		return
	}

	teacherID := r.PathValue("teacher_id")

	var req rateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	rating, err := h.uc.RateTeacher(r.Context(), claims.UserID, teacherID, req.Score)
	if err != nil {
		writeRatingError(w, err, "teacher not found")
		return
	}

	writeJSON(w, http.StatusCreated, teacherRatingResponse{
		ID:        rating.ID,
		TeacherID: rating.TeacherID,
		StudentID: rating.StudentID,
		Score:     rating.Score,
		CreatedAt: rating.CreatedAt,
		UpdatedAt: rating.UpdatedAt,
	})
}

// MyTeacherRating handles GET /api/v1/teachers/{teacher_id}/ratings/me.
// Student only; returns 404 if the caller hasn't rated this teacher yet.
func (h *RatingHandler) MyTeacherRating(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}
	if claims.Role != domain.RoleStudent {
		writeError(w, http.StatusForbidden, "only students have teacher ratings")
		return
	}

	teacherID := r.PathValue("teacher_id")

	rating, err := h.uc.MyTeacherRating(r.Context(), claims.UserID, teacherID)
	if err != nil {
		if errors.Is(err, domain.ErrRatingNotFound) {
			writeError(w, http.StatusNotFound, "you have not rated this teacher yet")
			return
		}
		writeRatingError(w, err, "teacher not found")
		return
	}

	writeJSON(w, http.StatusOK, teacherRatingResponse{
		ID:        rating.ID,
		TeacherID: rating.TeacherID,
		StudentID: rating.StudentID,
		Score:     rating.Score,
		CreatedAt: rating.CreatedAt,
		UpdatedAt: rating.UpdatedAt,
	})
}

// TeacherRatingSummary handles GET /api/v1/teachers/{teacher_id}/ratings. Public.
func (h *RatingHandler) TeacherRatingSummary(w http.ResponseWriter, r *http.Request) {
	teacherID := r.PathValue("teacher_id")

	summary, err := h.uc.TeacherRatingSummary(r.Context(), teacherID)
	if err != nil {
		writeRatingError(w, err, "teacher not found")
		return
	}

	writeJSON(w, http.StatusOK, ratingSummaryResponse{
		AverageScore: summary.AverageScore,
		RatingsCount: summary.RatingsCount,
	})
}
