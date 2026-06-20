package http

import (
	"errors"
	"net/http"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

type PlayerHandler struct {
	playerUseCase *usecase.PlayerUseCase
}

func NewPlayerHandler(uc *usecase.PlayerUseCase) *PlayerHandler {
	return &PlayerHandler{playerUseCase: uc}
}

// GetLesson handles GET /api/v1/player/courses/{course_id}/lessons/{lesson_id}
func (h *PlayerHandler) GetLesson(w http.ResponseWriter, r *http.Request) {
	courseID := r.PathValue("course_id")
	lessonID := r.PathValue("lesson_id")

	lesson, err := h.playerUseCase.GetLesson(r.Context(), courseID, lessonID)
	if err != nil {
		if errors.Is(err, domain.ErrLessonNotFound) {
			writeError(w, http.StatusNotFound, "lesson not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load lesson")
		return
	}

	writeJSON(w, http.StatusOK, lesson)
}
