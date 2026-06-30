package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

type stubAnalyticsRepo struct {
	rows []domain.StudentProgress
	err  error
}

func (s *stubAnalyticsRepo) CourseStudentProgress(_ context.Context, _ string) ([]domain.StudentProgress, error) {
	return s.rows, s.err
}
func (s *stubAnalyticsRepo) RefreshStudentCourseRollup(_ context.Context) error { return nil }

type stubCatalogRepo struct {
	course domain.Course
	err    error
}

func (s *stubCatalogRepo) GetCourses(_ domain.CourseListParams) ([]domain.Course, int, error) {
	return nil, 0, nil
}

func (s *stubCatalogRepo) GetByID(_ context.Context, _ string) (domain.Course, error) {
	return s.course, s.err
}

func (s *stubCatalogRepo) GetByTeacherID(_ context.Context, _ string) ([]domain.Course, error) {
	return nil, nil
}

func newAnalyticsHandler(course domain.Course, courseErr error, rows []domain.StudentProgress) *AnalyticsHandler {
	uc := usecase.NewAnalyticsUseCase(
		&stubAnalyticsRepo{rows: rows},
		&stubCatalogRepo{course: course, err: courseErr},
		domain.DefaultRiskThresholds,
	)
	return NewAnalyticsHandler(uc)
}

func dashboardReq(courseID string, role domain.Role) *http.Request {
	url := "http://x/api/v1/analytics/teacher/dashboard"
	if courseID != "" {
		url += "?course_id=" + courseID
	}
	r := httptest.NewRequest(http.MethodGet, url, nil)
	return r.WithContext(context.WithValue(r.Context(), claimsContextKey,
		domain.Claims{UserID: "teacher-1", Role: role}))
}

func TestTeacherDashboard_OK(t *testing.T) {
	now := time.Now()
	recent := now.Add(-1 * 24 * time.Hour)
	stale := now.Add(-30 * 24 * time.Hour)

	rows := []domain.StudentProgress{
		{StudentID: "s-low", ProgressPercent: 10, LastActivity: &recent},  // at risk: progress
		{StudentID: "s-ok", ProgressPercent: 80, LastActivity: &recent},   // on track
		{StudentID: "s-stale", ProgressPercent: 95, LastActivity: &stale}, // at risk: inactivity
		{StudentID: "s-none", ProgressPercent: 0, LastActivity: nil},      // at risk: never active
	}
	h := newAnalyticsHandler(domain.Course{ID: "course-1", TeacherID: "teacher-1"}, nil, rows)

	w := httptest.NewRecorder()
	h.TeacherDashboard(w, dashboardReq("course-1", domain.RoleTeacher))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp teacherDashboardDTO
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.AtRiskStudents != 3 {
		t.Errorf("at_risk_students = %d, want 3", resp.AtRiskStudents)
	}
	if len(resp.Students) != 4 {
		t.Fatalf("students = %d, want 4", len(resp.Students))
	}
	if resp.Students[1].Status != domain.RiskOnTrack {
		t.Errorf("s-ok status = %q, want ON_TRACK", resp.Students[1].Status)
	}
}

func TestTeacherDashboard_RequiresTeacherRole(t *testing.T) {
	h := newAnalyticsHandler(domain.Course{ID: "course-1", TeacherID: "teacher-1"}, nil, nil)
	w := httptest.NewRecorder()
	h.TeacherDashboard(w, dashboardReq("course-1", domain.RoleStudent))
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestTeacherDashboard_ForeignCourse(t *testing.T) {
	h := newAnalyticsHandler(domain.Course{ID: "course-1", TeacherID: "someone-else"}, nil, nil)
	w := httptest.NewRecorder()
	h.TeacherDashboard(w, dashboardReq("course-1", domain.RoleTeacher))
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestTeacherDashboard_MissingCourseID(t *testing.T) {
	h := newAnalyticsHandler(domain.Course{ID: "course-1", TeacherID: "teacher-1"}, nil, nil)
	w := httptest.NewRecorder()
	h.TeacherDashboard(w, dashboardReq("", domain.RoleTeacher))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestTeacherDashboard_CourseNotFound(t *testing.T) {
	h := newAnalyticsHandler(domain.Course{}, domain.ErrCourseNotFound, nil)
	w := httptest.NewRecorder()
	h.TeacherDashboard(w, dashboardReq("missing", domain.RoleTeacher))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}
