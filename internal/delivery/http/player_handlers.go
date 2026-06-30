package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

type PlayerHandler struct {
	playerUseCase *usecase.PlayerUseCase
	analytics     *usecase.AnalyticsRecorder
}

func NewPlayerHandler(uc *usecase.PlayerUseCase, analytics *usecase.AnalyticsRecorder) *PlayerHandler {
	return &PlayerHandler{playerUseCase: uc, analytics: analytics}
}

// materialDTO is a lesson attachment as returned to the player.
type materialDTO struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Type  string `json:"type"`
}

// lessonDataResponse is the GET-lesson payload. It carries the OpenAPI LessonData
// contract (lesson_id, content_url, last_progress_seconds) plus additive,
// backwards-compatible fields the player UI needs to render and resume.
type lessonDataResponse struct {
	LessonID            string        `json:"lesson_id"`
	CourseID            string        `json:"course_id"`
	Title               string        `json:"title"`
	LessonType          string        `json:"lesson_type"`
	Position            int           `json:"position"`
	ContentURL          string        `json:"content_url"`
	DurationSeconds     int           `json:"duration_seconds"`
	Materials           []materialDTO `json:"materials"`
	LastProgressSeconds int           `json:"last_progress_seconds"`
	PercentComplete     float64       `json:"percent_complete"`
}

// saveProgressRequest is the POST-progress body (OpenAPI: progress_seconds, completed).
type saveProgressRequest struct {
	ProgressSeconds *int `json:"progress_seconds"`
	Completed       bool `json:"completed"`
}

// progressResponse is the saved resume point returned by the progress endpoints.
type progressResponse struct {
	LessonID        string  `json:"lesson_id"`
	ProgressSeconds int     `json:"progress_seconds"`
	PercentComplete float64 `json:"percent_complete"`
	Completed       bool    `json:"completed"`
	UpdatedAt       string  `json:"updated_at"`
}

func toProgressResponse(p domain.ProgressState) progressResponse {
	return progressResponse{
		LessonID:        p.LessonID,
		ProgressSeconds: p.PositionMs / 1000, //nolint:mnd // ms -> s
		PercentComplete: p.PercentComplete,
		Completed:       p.PercentComplete >= 100, //nolint:mnd // 100 percent
		UpdatedAt:       p.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

// GetLesson handles GET /api/v1/player/courses/{course_id}/lessons/{lesson_id}.
// Entitlement is enforced by RequireEntitlement upstream; this returns the lesson
// content together with the learner's last saved resume point.
func (h *PlayerHandler) GetLesson(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}
	courseID := r.PathValue("course_id")
	lessonID := r.PathValue("lesson_id")

	content, err := h.playerUseCase.GetLessonContent(r.Context(), claims.UserID, courseID, lessonID)
	if err != nil {
		if errors.Is(err, domain.ErrLessonNotFound) {
			writeError(w, http.StatusNotFound, "lesson not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load lesson")
		return
	}

	contentURL, durationSeconds := "", 0
	if len(content.Media) > 0 {
		contentURL = content.Media[0].URL
		durationSeconds = content.Media[0].DurationMs / 1000 //nolint:mnd // ms -> s
	}

	materials := make([]materialDTO, 0, len(content.Materials))
	for _, m := range content.Materials {
		materials = append(materials, materialDTO{Title: m.Title, URL: m.URL, Type: m.Type})
	}

	h.analytics.Record(r.Context(), domain.EventPlayerLessonOpen, domain.PIINone, map[string]any{
		keyCourseID:       content.Lesson.CourseID,
		keyLessonID:       content.Lesson.ID,
		"lesson_type":     content.Lesson.Type,
		"resumed_from_ms": content.Progress.PositionMs,
	})

	writeJSON(w, http.StatusOK, lessonDataResponse{
		LessonID:            content.Lesson.ID,
		CourseID:            content.Lesson.CourseID,
		Title:               content.Lesson.Title,
		LessonType:          content.Lesson.Type,
		Position:            content.Lesson.Position,
		ContentURL:          contentURL,
		DurationSeconds:     durationSeconds,
		Materials:           materials,
		LastProgressSeconds: content.Progress.PositionMs / 1000, //nolint:mnd // ms -> s
		PercentComplete:     content.Progress.PercentComplete,
	})
}

// SaveProgress handles POST /api/v1/player/courses/{course_id}/lessons/{lesson_id}/progress.
// It upserts the authenticated learner's resume point for the lesson.
func (h *PlayerHandler) SaveProgress(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}
	courseID := r.PathValue("course_id")
	lessonID := r.PathValue("lesson_id")

	var req saveProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ProgressSeconds == nil {
		writeError(w, http.StatusBadRequest, "progress_seconds is required")
		return
	}
	if *req.ProgressSeconds < 0 {
		writeError(w, http.StatusBadRequest, "progress_seconds must not be negative")
		return
	}

	saved, err := h.playerUseCase.SaveProgress(
		r.Context(), claims.UserID, courseID, lessonID, *req.ProgressSeconds*1000, req.Completed, //nolint:mnd // s -> ms
	)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrLessonNotFound):
			writeError(w, http.StatusNotFound, "lesson not found")
		case errors.Is(err, usecase.ErrValidation):
			writeError(w, http.StatusBadRequest, "invalid progress")
		default:
			writeError(w, http.StatusInternalServerError, "failed to save progress")
		}
		return
	}

	h.analytics.Record(r.Context(), domain.EventPlayerProgressSave, domain.PIINone, map[string]any{
		keyCourseID:        courseID,
		keyLessonID:        lessonID,
		"position_ms":      saved.PositionMs,
		"percent_complete": saved.PercentComplete,
	})
	if req.Completed {
		h.analytics.Record(r.Context(), domain.EventPlayerLessonComplete, domain.PIINone, map[string]any{
			keyCourseID:      courseID,
			keyLessonID:      lessonID,
			"completion_pct": saved.PercentComplete,
		})
	}

	writeJSON(w, http.StatusOK, toProgressResponse(saved))
}

// GetProgress handles GET /api/v1/player/courses/{course_id}/lessons/{lesson_id}/progress.
// It returns the authenticated learner's saved resume point, or 404 if none exists yet.
func (h *PlayerHandler) GetProgress(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}
	courseID := r.PathValue("course_id")
	lessonID := r.PathValue("lesson_id")

	progress, err := h.playerUseCase.GetProgress(r.Context(), claims.UserID, courseID, lessonID)
	if err != nil {
		if errors.Is(err, domain.ErrProgressNotFound) {
			writeError(w, http.StatusNotFound, "no saved progress")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load progress")
		return
	}

	writeJSON(w, http.StatusOK, toProgressResponse(progress))
}
