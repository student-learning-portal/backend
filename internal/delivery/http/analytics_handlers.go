package http

import (
	"errors"
	"net/http"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

type AnalyticsHandler struct {
	uc *usecase.AnalyticsUseCase
}

func NewAnalyticsHandler(uc *usecase.AnalyticsUseCase) *AnalyticsHandler {
	return &AnalyticsHandler{uc: uc}
}

// dashboardStudentDTO carries the OpenAPI teacher-dashboard student contract
// (student_id, progress_percentage, status) plus additive, backwards-compatible
// fields the UI finds useful (full_name, days_inactive).
type dashboardStudentDTO struct {
	StudentID          string  `json:"student_id"`
	ProgressPercentage float64 `json:"progress_percentage"`
	Status             string  `json:"status"`
	FullName           string  `json:"full_name,omitempty"`
	DaysInactive       int     `json:"days_inactive"`
}

type teacherDashboardDTO struct {
	AtRiskStudents int                   `json:"at_risk_students"`
	Students       []dashboardStudentDTO `json:"students"`
}

// TeacherDashboard handles GET /api/v1/analytics/teacher/dashboard?course_id=...
// RequireAuth runs upstream; access is further restricted to the teacher who
// owns the requested course.
func (h *AnalyticsHandler) TeacherDashboard(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}
	if claims.Role != domain.RoleTeacher {
		writeError(w, http.StatusForbidden, "teacher role required")
		return
	}

	courseID := r.URL.Query().Get("course_id")
	if courseID == "" {
		writeError(w, http.StatusBadRequest, "course_id is required")
		return
	}

	result, err := h.uc.TeacherDashboard(r.Context(), claims.UserID, courseID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrCourseNotFound):
			writeError(w, http.StatusNotFound, "course not found")
		case errors.Is(err, domain.ErrForbidden):
			writeError(w, http.StatusForbidden, "you do not own this course")
		default:
			writeError(w, http.StatusInternalServerError, "failed to load dashboard")
		}
		return
	}

	students := make([]dashboardStudentDTO, 0, len(result.Students))
	for _, s := range result.Students {
		students = append(students, dashboardStudentDTO{
			StudentID:          s.StudentID,
			ProgressPercentage: s.ProgressPercent,
			Status:             s.Status,
			FullName:           s.FullName,
			DaysInactive:       s.DaysInactive,
		})
	}

	writeJSON(w, http.StatusOK, teacherDashboardDTO{
		AtRiskStudents: result.AtRiskStudents,
		Students:       students,
	})
}

// dashboardCourseDTO carries one enrolled course's row in the student's own
// dashboard: the OpenAPI-documented aggregate fields are on the parent object,
// this is the additive per-course breakdown the UI needs to render it.
type dashboardCourseDTO struct {
	CourseID         string  `json:"course_id"`
	CourseTitle      string  `json:"course_title"`
	ProgressPercent  float64 `json:"progress_percentage"`
	LessonsCompleted int     `json:"lessons_completed"`
	LessonsTotal     int     `json:"lessons_total"`
	Status           string  `json:"status"`
	DaysInactive     int     `json:"days_inactive"`
}

type studentDashboardDTO struct {
	OverallProgress  float64              `json:"overall_progress"`
	CoursesCompleted int                  `json:"courses_completed"`
	Courses          []dashboardCourseDTO `json:"courses"`
}

// StudentDashboard handles GET /api/v1/analytics/student/me. RequireAuth runs
// upstream; the caller only ever sees their own rollup, scoped by the token's
// subject, so no further ownership check is needed.
func (h *AnalyticsHandler) StudentDashboard(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}
	if claims.Role != domain.RoleStudent {
		writeError(w, http.StatusForbidden, "student role required")
		return
	}

	result, err := h.uc.StudentDashboard(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load dashboard")
		return
	}

	courses := make([]dashboardCourseDTO, 0, len(result.Courses))
	for _, c := range result.Courses {
		courses = append(courses, dashboardCourseDTO{
			CourseID:         c.CourseID,
			CourseTitle:      c.CourseTitle,
			ProgressPercent:  c.ProgressPercent,
			LessonsCompleted: c.LessonsCompleted,
			LessonsTotal:     c.LessonsTotal,
			Status:           c.Status,
			DaysInactive:     c.DaysInactive,
		})
	}

	writeJSON(w, http.StatusOK, studentDashboardDTO{
		OverallProgress:  result.OverallProgress,
		CoursesCompleted: result.CoursesCompleted,
		Courses:          courses,
	})
}
