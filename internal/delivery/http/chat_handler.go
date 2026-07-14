package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

type ChatHandler struct {
	uc *usecase.ChatUseCase
}

func NewChatHandler(uc *usecase.ChatUseCase) *ChatHandler {
	return &ChatHandler{uc: uc}
}

type sendMessageRequest struct {
	Body     string  `json:"body"`
	LessonID *string `json:"lesson_id,omitempty"`
}

// StudentThread handles GET /api/v1/courses/{course_id}/messages — the calling
// student's own conversation with the course's teacher.
func (h *ChatHandler) StudentThread(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.requireStudent(w, r)
	if !ok {
		return
	}
	messages, err := h.uc.StudentThread(r.Context(), claims.UserID, r.PathValue(keyCourseID))
	if err != nil {
		writeChatError(w, err, "failed to load messages")
		return
	}
	writeJSON(w, http.StatusOK, messages)
}

// StudentSend handles POST /api/v1/courses/{course_id}/messages — a student
// sends a message to the course's teacher.
func (h *ChatHandler) StudentSend(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.requireStudent(w, r)
	if !ok {
		return
	}
	req, ok := decodeSendMessage(w, r)
	if !ok {
		return
	}
	msg, err := h.uc.StudentSend(r.Context(), claims.UserID, r.PathValue(keyCourseID), req.LessonID, req.Body)
	if err != nil {
		writeChatError(w, err, "failed to send message")
		return
	}
	writeJSON(w, http.StatusCreated, msg)
}

// TeacherThreads handles GET /api/v1/teacher/courses/{course_id}/threads — the
// owning teacher's inbox of student conversations.
func (h *ChatHandler) TeacherThreads(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.requireTeacher(w, r)
	if !ok {
		return
	}
	threads, err := h.uc.TeacherThreads(r.Context(), claims.UserID, r.PathValue(keyCourseID))
	if err != nil {
		writeChatError(w, err, "failed to load threads")
		return
	}
	writeJSON(w, http.StatusOK, threads)
}

// TeacherThread handles GET /api/v1/teacher/courses/{course_id}/threads/{student_id}/messages.
func (h *ChatHandler) TeacherThread(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.requireTeacher(w, r)
	if !ok {
		return
	}
	messages, err := h.uc.TeacherThread(r.Context(), claims.UserID, r.PathValue(keyCourseID), r.PathValue("student_id"))
	if err != nil {
		writeChatError(w, err, "failed to load messages")
		return
	}
	writeJSON(w, http.StatusOK, messages)
}

// TeacherSend handles POST /api/v1/teacher/courses/{course_id}/threads/{student_id}/messages.
func (h *ChatHandler) TeacherSend(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.requireTeacher(w, r)
	if !ok {
		return
	}
	req, ok := decodeSendMessage(w, r)
	if !ok {
		return
	}
	msg, err := h.uc.TeacherSend(r.Context(), claims.UserID, r.PathValue(keyCourseID), r.PathValue("student_id"), req.LessonID, req.Body)
	if err != nil {
		writeChatError(w, err, "failed to send message")
		return
	}
	writeJSON(w, http.StatusCreated, msg)
}

func (h *ChatHandler) requireStudent(w http.ResponseWriter, r *http.Request) (domain.Claims, bool) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return domain.Claims{}, false
	}
	if claims.Role != domain.RoleStudent {
		writeError(w, http.StatusForbidden, "student role required")
		return domain.Claims{}, false
	}
	return claims, true
}

func (h *ChatHandler) requireTeacher(w http.ResponseWriter, r *http.Request) (domain.Claims, bool) {
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

func decodeSendMessage(w http.ResponseWriter, r *http.Request) (sendMessageRequest, bool) {
	var req sendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return sendMessageRequest{}, false
	}
	return req, true
}

func writeChatError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, domain.ErrEmptyMessage):
		writeError(w, http.StatusBadRequest, "message body is required")
	case errors.Is(err, domain.ErrCourseNotFound):
		writeError(w, http.StatusNotFound, "course not found")
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, http.StatusForbidden, "not allowed for this course")
	default:
		writeError(w, http.StatusInternalServerError, fallback)
	}
}
