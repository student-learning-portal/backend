package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

// TeacherContentHandler serves the teacher-only course/lesson authoring
// endpoints: create/edit/delete a course, its lessons, and their media and
// materials. Ownership of the course is enforced in the usecase layer; here
// we only gate on role.
type TeacherContentHandler struct {
	catalogUseCase *usecase.CatalogUseCase
}

func NewTeacherContentHandler(uc *usecase.CatalogUseCase) *TeacherContentHandler {
	return &TeacherContentHandler{catalogUseCase: uc}
}

// teacherLessonDTO carries a lesson's fields under the lesson_id/lesson_type
// keys the frontend uses elsewhere (LessonSummary/LessonData), rather than
// domain.Lesson's raw id/type field names.
type teacherLessonDTO struct {
	LessonID   string `json:"lesson_id"`
	CourseID   string `json:"course_id"`
	Title      string `json:"title"`
	LessonType string `json:"lesson_type"`
	Position   int    `json:"position"`
}

func toTeacherLessonDTO(l domain.Lesson) teacherLessonDTO {
	return teacherLessonDTO{
		LessonID:   l.ID,
		CourseID:   l.CourseID,
		Title:      l.Title,
		LessonType: l.Type,
		Position:   l.Position,
	}
}

type mediaDTO struct {
	ID              string `json:"id"`
	URL             string `json:"url"`
	DurationSeconds int    `json:"duration_seconds"`
	MediaType       string `json:"media_type"`
}

func toMediaDTO(m domain.Media) mediaDTO {
	const msPerSecond = 1000
	return mediaDTO{ID: m.ID, URL: m.URL, DurationSeconds: m.DurationMs / msPerSecond, MediaType: m.Type}
}

type materialResponseDTO struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
	Type  string `json:"type"`
}

func toMaterialResponseDTO(m domain.Material) materialResponseDTO {
	return materialResponseDTO{ID: m.ID, Title: m.Title, URL: m.URL, Type: m.Type}
}

// requireTeacher returns the authenticated teacher's claims, writing the
// appropriate error response and returning ok=false if the caller isn't an
// authenticated teacher.
func requireTeacher(w http.ResponseWriter, r *http.Request) (domain.Claims, bool) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return domain.Claims{}, false
	}
	if claims.Role != domain.RoleTeacher {
		writeError(w, http.StatusForbidden, "teacher role required")
		return domain.Claims{}, false
	}
	return claims, true
}

// writeCourseUseCaseError maps the shared course/lesson authoring errors to
// HTTP status codes, consistent with the errors.Is switch pattern used across
// the other handler files (analytics_handlers.go, payment_handlers.go, ...).
func writeCourseUseCaseError(w http.ResponseWriter, err error, notFoundMsg string) {
	switch {
	case errors.Is(err, domain.ErrCourseNotFound):
		writeError(w, http.StatusNotFound, "course not found")
	case errors.Is(err, domain.ErrLessonNotFound):
		writeError(w, http.StatusNotFound, "lesson not found")
	case errors.Is(err, domain.ErrMaterialNotFound):
		writeError(w, http.StatusNotFound, "material not found")
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, http.StatusForbidden, "you do not own this course")
	case errors.Is(err, domain.ErrCourseNotDraft):
		writeError(w, http.StatusConflict, "only draft courses can be deleted; archive it instead")
	case errors.Is(err, domain.ErrLessonOrderMismatch):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, usecase.ErrValidation):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, notFoundMsg)
	}
}

type courseRequest struct {
	Title           string  `json:"title"`
	Description     string  `json:"description"`
	Subject         string  `json:"subject"`
	Price           float64 `json:"price"`
	Currency        string  `json:"currency"`
	Status          string  `json:"status"`
	Difficulty      string  `json:"difficulty"`
	DurationMinutes int     `json:"duration_minutes"`
}

// CreateCourse handles POST /api/v1/teacher/courses.
func (h *TeacherContentHandler) CreateCourse(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireTeacher(w, r)
	if !ok {
		return
	}

	var req courseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	course, err := h.catalogUseCase.CreateCourse(r.Context(), claims.UserID, usecase.CourseInput{
		Title:           req.Title,
		Description:     req.Description,
		Subject:         req.Subject,
		Price:           req.Price,
		Currency:        req.Currency,
		Difficulty:      domain.DifficultyLevel(req.Difficulty),
		DurationMinutes: req.DurationMinutes,
	})
	if err != nil {
		writeCourseUseCaseError(w, err, "failed to create course")
		return
	}
	writeJSON(w, http.StatusCreated, course)
}

// UpdateCourse handles PATCH /api/v1/teacher/courses/{course_id}.
func (h *TeacherContentHandler) UpdateCourse(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireTeacher(w, r)
	if !ok {
		return
	}
	courseID := r.PathValue(keyCourseID)

	var req courseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	course, err := h.catalogUseCase.UpdateCourse(r.Context(), claims.UserID, courseID, usecase.CourseInput{
		Title:           req.Title,
		Description:     req.Description,
		Subject:         req.Subject,
		Price:           req.Price,
		Currency:        req.Currency,
		Difficulty:      domain.DifficultyLevel(req.Difficulty),
		DurationMinutes: req.DurationMinutes,
	}, req.Status)
	if err != nil {
		writeCourseUseCaseError(w, err, "failed to update course")
		return
	}
	writeJSON(w, http.StatusOK, course)
}

// DeleteCourse handles DELETE /api/v1/teacher/courses/{course_id}.
func (h *TeacherContentHandler) DeleteCourse(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireTeacher(w, r)
	if !ok {
		return
	}
	courseID := r.PathValue(keyCourseID)

	if err := h.catalogUseCase.DeleteCourse(r.Context(), claims.UserID, courseID); err != nil {
		writeCourseUseCaseError(w, err, "failed to delete course")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type lessonRequest struct {
	Title      string `json:"title"`
	LessonType string `json:"lesson_type"`
}

// CreateLesson handles POST /api/v1/teacher/courses/{course_id}/lessons.
func (h *TeacherContentHandler) CreateLesson(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireTeacher(w, r)
	if !ok {
		return
	}
	courseID := r.PathValue(keyCourseID)

	var req lessonRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	lesson, err := h.catalogUseCase.CreateLesson(r.Context(), claims.UserID, courseID, req.Title, req.LessonType)
	if err != nil {
		writeCourseUseCaseError(w, err, "failed to create lesson")
		return
	}
	writeJSON(w, http.StatusCreated, toTeacherLessonDTO(lesson))
}

// UpdateLesson handles PATCH /api/v1/teacher/courses/{course_id}/lessons/{lesson_id}.
func (h *TeacherContentHandler) UpdateLesson(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireTeacher(w, r)
	if !ok {
		return
	}
	courseID := r.PathValue(keyCourseID)
	lessonID := r.PathValue(keyLessonID)

	var req lessonRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	lesson, err := h.catalogUseCase.UpdateLesson(r.Context(), claims.UserID, courseID, lessonID, req.Title, req.LessonType)
	if err != nil {
		writeCourseUseCaseError(w, err, "failed to update lesson")
		return
	}
	writeJSON(w, http.StatusOK, toTeacherLessonDTO(lesson))
}

// DeleteLesson handles DELETE /api/v1/teacher/courses/{course_id}/lessons/{lesson_id}.
func (h *TeacherContentHandler) DeleteLesson(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireTeacher(w, r)
	if !ok {
		return
	}
	courseID := r.PathValue(keyCourseID)
	lessonID := r.PathValue(keyLessonID)

	if err := h.catalogUseCase.DeleteLesson(r.Context(), claims.UserID, courseID, lessonID); err != nil {
		writeCourseUseCaseError(w, err, "failed to delete lesson")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type reorderRequest struct {
	LessonIDs []string `json:"lesson_ids"`
}

// ReorderLessons handles PUT /api/v1/teacher/courses/{course_id}/lessons/order.
func (h *TeacherContentHandler) ReorderLessons(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireTeacher(w, r)
	if !ok {
		return
	}
	courseID := r.PathValue(keyCourseID)

	var req reorderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.catalogUseCase.ReorderLessons(r.Context(), claims.UserID, courseID, req.LessonIDs); err != nil {
		writeCourseUseCaseError(w, err, "failed to reorder lessons")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type mediaRequest struct {
	URL             string `json:"url"`
	DurationSeconds int    `json:"duration_seconds"`
	MediaType       string `json:"media_type"`
}

// SetLessonMedia handles PUT /api/v1/teacher/courses/{course_id}/lessons/{lesson_id}/media.
func (h *TeacherContentHandler) SetLessonMedia(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireTeacher(w, r)
	if !ok {
		return
	}
	courseID := r.PathValue(keyCourseID)
	lessonID := r.PathValue(keyLessonID)

	var req mediaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	media, err := h.catalogUseCase.SetLessonMedia(r.Context(), claims.UserID, courseID, lessonID, usecase.MediaInput{
		URL:             req.URL,
		DurationSeconds: req.DurationSeconds,
		MediaType:       req.MediaType,
	})
	if err != nil {
		writeCourseUseCaseError(w, err, "failed to set lesson media")
		return
	}
	writeJSON(w, http.StatusOK, toMediaDTO(media))
}

// DeleteLessonMedia handles DELETE /api/v1/teacher/courses/{course_id}/lessons/{lesson_id}/media.
func (h *TeacherContentHandler) DeleteLessonMedia(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireTeacher(w, r)
	if !ok {
		return
	}
	courseID := r.PathValue(keyCourseID)
	lessonID := r.PathValue(keyLessonID)

	if err := h.catalogUseCase.DeleteLessonMedia(r.Context(), claims.UserID, courseID, lessonID); err != nil {
		writeCourseUseCaseError(w, err, "failed to remove lesson media")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type materialRequest struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Type  string `json:"type"`
}

// AddMaterial handles POST /api/v1/teacher/courses/{course_id}/lessons/{lesson_id}/materials.
func (h *TeacherContentHandler) AddMaterial(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireTeacher(w, r)
	if !ok {
		return
	}
	courseID := r.PathValue(keyCourseID)
	lessonID := r.PathValue(keyLessonID)

	var req materialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	material, err := h.catalogUseCase.AddMaterial(r.Context(), claims.UserID, courseID, lessonID, usecase.MaterialInput{
		Title: req.Title,
		URL:   req.URL,
		Type:  req.Type,
	})
	if err != nil {
		writeCourseUseCaseError(w, err, "failed to add material")
		return
	}
	writeJSON(w, http.StatusCreated, toMaterialResponseDTO(material))
}

// DeleteMaterial handles DELETE /api/v1/teacher/courses/{course_id}/lessons/{lesson_id}/materials/{material_id}.
func (h *TeacherContentHandler) DeleteMaterial(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireTeacher(w, r)
	if !ok {
		return
	}
	courseID := r.PathValue(keyCourseID)
	lessonID := r.PathValue(keyLessonID)
	materialID := r.PathValue("material_id")

	if err := h.catalogUseCase.DeleteMaterial(r.Context(), claims.UserID, courseID, lessonID, materialID); err != nil {
		writeCourseUseCaseError(w, err, "failed to delete material")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
