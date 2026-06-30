package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/student-learning-portal/backend/internal/domain"
)

// stubAnalyticsRepository implements domain.AnalyticsRepository for analytics dashboard tests.
type stubAnalyticsRepository struct {
	rows []domain.StudentProgress
	err  error
}

func (s *stubAnalyticsRepository) CourseStudentProgress(_ context.Context, _ string) ([]domain.StudentProgress, error) {
	return s.rows, s.err
}
func (s *stubAnalyticsRepository) RefreshStudentCourseRollup(_ context.Context) error { return nil }

func newDashboardUC(course domain.Course, courseErr error, rows []domain.StudentProgress) *AnalyticsUseCase {
	return NewAnalyticsUseCase(
		&stubAnalyticsRepository{rows: rows},
		&stubCatalogRepository{course: course, courseErr: courseErr},
		domain.DefaultRiskThresholds,
	)
}

func TestTeacherDashboard_Success(t *testing.T) {
	now := time.Now()
	recent := now.Add(-2 * 24 * time.Hour)
	stale := now.Add(-30 * 24 * time.Hour)

	rows := []domain.StudentProgress{
		{StudentID: "s-ok", ProgressPercent: 80, LastActivity: &recent},  // on track
		{StudentID: "s-low", ProgressPercent: 10, LastActivity: &recent}, // at risk: low progress
		{StudentID: "s-old", ProgressPercent: 90, LastActivity: &stale},  // at risk: inactivity
		{StudentID: "s-none", ProgressPercent: 0, LastActivity: nil},     // at risk: no activity
	}
	uc := newDashboardUC(domain.Course{ID: "c1", TeacherID: "teacher-1"}, nil, rows)

	res, err := uc.TeacherDashboard(context.Background(), "teacher-1", "c1")
	if err != nil {
		t.Fatalf("TeacherDashboard: %v", err)
	}
	if res.AtRiskStudents != 3 {
		t.Errorf("at_risk_students = %d, want 3", res.AtRiskStudents)
	}
	if len(res.Students) != 4 {
		t.Fatalf("students = %d, want 4", len(res.Students))
	}
}

func TestTeacherDashboard_ForeignCourse(t *testing.T) {
	uc := newDashboardUC(domain.Course{ID: "c1", TeacherID: "someone-else"}, nil, nil)
	_, err := uc.TeacherDashboard(context.Background(), "teacher-1", "c1")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("err = %v, want ErrForbidden", err)
	}
}

func TestTeacherDashboard_CourseNotFound(t *testing.T) {
	uc := newDashboardUC(domain.Course{}, domain.ErrCourseNotFound, nil)
	_, err := uc.TeacherDashboard(context.Background(), "teacher-1", "missing")
	if !errors.Is(err, domain.ErrCourseNotFound) {
		t.Errorf("err = %v, want ErrCourseNotFound", err)
	}
}

func TestTeacherDashboard_EmptyCourse(t *testing.T) {
	uc := newDashboardUC(domain.Course{ID: "c1", TeacherID: "teacher-1"}, nil, nil)
	res, err := uc.TeacherDashboard(context.Background(), "teacher-1", "c1")
	if err != nil {
		t.Fatalf("TeacherDashboard: %v", err)
	}
	if res.AtRiskStudents != 0 {
		t.Errorf("at_risk_students = %d, want 0", res.AtRiskStudents)
	}
	if len(res.Students) != 0 {
		t.Errorf("students = %d, want 0", len(res.Students))
	}
}

func TestTeacherDashboard_StudentStatusFields(t *testing.T) {
	now := time.Now()
	recent := now.Add(-1 * 24 * time.Hour)
	rows := []domain.StudentProgress{
		{StudentID: "s-1", FullName: "Alice", ProgressPercent: 75, LastActivity: &recent},
	}
	uc := newDashboardUC(domain.Course{ID: "c1", TeacherID: "t-1"}, nil, rows)

	res, err := uc.TeacherDashboard(context.Background(), "t-1", "c1")
	if err != nil {
		t.Fatalf("TeacherDashboard: %v", err)
	}
	if len(res.Students) != 1 {
		t.Fatalf("students = %d, want 1", len(res.Students))
	}
	s := res.Students[0]
	if s.StudentID != "s-1" {
		t.Errorf("student_id = %q, want s-1", s.StudentID)
	}
	if s.FullName != "Alice" {
		t.Errorf("full_name = %q, want Alice", s.FullName)
	}
	if s.ProgressPercent != 75 {
		t.Errorf("progress_percent = %v, want 75", s.ProgressPercent)
	}
	if s.Status != domain.RiskOnTrack {
		t.Errorf("status = %q, want ON_TRACK", s.Status)
	}
}

func TestTeacherDashboard_AnalyticsError(t *testing.T) {
	analyticsRepo := &stubAnalyticsRepository{err: errors.New("rollup unavailable")}
	uc := NewAnalyticsUseCase(
		analyticsRepo,
		&stubCatalogRepository{course: domain.Course{ID: "c1", TeacherID: "t-1"}},
		domain.DefaultRiskThresholds,
	)
	_, err := uc.TeacherDashboard(context.Background(), "t-1", "c1")
	if err == nil {
		t.Fatal("expected error when analytics repo fails")
	}
}
