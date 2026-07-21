package http

import (
	"errors"
	"net/http"
	"time"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

// AdminHandler serves the administrator's moderation surface: the queue of
// teacher registrations awaiting confirmation and the approve/reject decision
// on each. Every route is behind RequireAdmin (see router.go), so the handlers
// themselves can assume the caller is an administrator.
type AdminHandler struct {
	adminUseCase *usecase.AdminUseCase
	analytics    *usecase.AnalyticsRecorder
}

func NewAdminHandler(uc *usecase.AdminUseCase, analytics *usecase.AnalyticsRecorder) *AdminHandler {
	return &AdminHandler{adminUseCase: uc, analytics: analytics}
}

// teacherApplicationDTO is one row of the review queue. It carries only what an
// administrator needs to decide — never the password hash or wallet balance.
type teacherApplicationDTO struct {
	ID           string     `json:"id"`
	Email        string     `json:"email"`
	FullName     string     `json:"full_name"`
	Status       string     `json:"status"`
	RegisteredAt time.Time  `json:"registered_at"`
	ReviewedAt   *time.Time `json:"reviewed_at,omitempty"`
}

func toTeacherApplicationDTO(u domain.User) teacherApplicationDTO {
	dto := teacherApplicationDTO{
		ID:           u.ID,
		Email:        u.Email,
		FullName:     u.FullName,
		Status:       string(u.TeacherStatus),
		RegisteredAt: u.CreatedAt,
	}
	// A still-pending application has never been decided on; the column then
	// only holds the signup time, which would read as a bogus review date.
	if u.TeacherStatus != domain.TeacherStatusPending && !u.TeacherStatusUpdatedAt.IsZero() {
		reviewed := u.TeacherStatusUpdatedAt
		dto.ReviewedAt = &reviewed
	}
	return dto
}

type teacherApplicationsDTO struct {
	Pending int                     `json:"pending"`
	Items   []teacherApplicationDTO `json:"items"`
}

// ListTeachers handles GET /api/v1/admin/teachers?status=pending — the review
// queue. status defaults to pending; status=all returns every teacher account
// so the admin can also see what they already decided.
func (h *AdminHandler) ListTeachers(w http.ResponseWriter, r *http.Request) {
	status := domain.TeacherStatus(r.URL.Query().Get("status"))
	switch status {
	case "":
		status = domain.TeacherStatusPending
	case "all":
		status = ""
	}

	teachers, err := h.adminUseCase.Teachers(r.Context(), status)
	if err != nil {
		if errors.Is(err, usecase.ErrValidation) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load teacher applications")
		return
	}

	items := make([]teacherApplicationDTO, 0, len(teachers))
	pending := 0
	for _, t := range teachers {
		if t.TeacherStatus == domain.TeacherStatusPending {
			pending++
		}
		items = append(items, toTeacherApplicationDTO(t))
	}

	writeJSON(w, http.StatusOK, teacherApplicationsDTO{Pending: pending, Items: items})
}

// ApproveTeacher handles POST /api/v1/admin/teachers/{user_id}/approve.
func (h *AdminHandler) ApproveTeacher(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, domain.TeacherStatusApproved)
}

// RejectTeacher handles POST /api/v1/admin/teachers/{user_id}/reject.
func (h *AdminHandler) RejectTeacher(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, domain.TeacherStatusRejected)
}

// decide applies one approval decision and mirrors it to the analytics stream.
// Approve and reject differ only in the recorded status and event name, so they
// share this body rather than duplicating the claims/error/response handling.
func (h *AdminHandler) decide(w http.ResponseWriter, r *http.Request, status domain.TeacherStatus) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	teacherID := r.PathValue("user_id")

	var (
		teacher domain.User
		err     error
	)
	if status == domain.TeacherStatusApproved {
		teacher, err = h.adminUseCase.ApproveTeacher(r.Context(), teacherID, claims.UserID)
	} else {
		teacher, err = h.adminUseCase.RejectTeacher(r.Context(), teacherID, claims.UserID)
	}
	if err != nil {
		switch {
		case errors.Is(err, usecase.ErrValidation):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, domain.ErrUserNotFound):
			writeError(w, http.StatusNotFound, "teacher not found")
		case errors.Is(err, domain.ErrNotTeacher):
			writeError(w, http.StatusConflict, "this account is not a teacher")
		default:
			writeError(w, http.StatusInternalServerError, "failed to update teacher status")
		}
		return
	}

	eventName := domain.EventAdminTeacherApproved
	if status == domain.TeacherStatusRejected {
		eventName = domain.EventAdminTeacherRejected
	}
	h.analytics.Record(r.Context(), eventName, domain.PIINone, map[string]any{
		"teacher_id": teacher.ID,
		"status":     string(teacher.TeacherStatus),
	})

	writeJSON(w, http.StatusOK, toTeacherApplicationDTO(teacher))
}
