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
	rows       []domain.StudentProgress
	err        error
	courseRows []domain.CourseProgress
	courseErr  error
}

func (s *stubAnalyticsRepository) CourseStudentProgress(_ context.Context, _ string) ([]domain.StudentProgress, error) {
	return s.rows, s.err
}
func (s *stubAnalyticsRepository) StudentCourseProgress(_ context.Context, _ string) ([]domain.CourseProgress, error) {
	return s.courseRows, s.courseErr
}
func (s *stubAnalyticsRepository) RefreshStudentCourseRow(_ context.Context, _, _ string) error {
	return nil
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
	// A learner with recorded activity carries their timestamp through; one who
	// has never started keeps a nil LastActivity so the UI can tell them apart.
	if got := res.Students[0]; got.LastActivity == nil || !got.LastActivity.Equal(recent) {
		t.Errorf("s-ok last_activity = %v, want %v", got.LastActivity, recent)
	}
	if got := res.Students[3]; got.LastActivity != nil {
		t.Errorf("s-none last_activity = %v, want nil (never started)", got.LastActivity)
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
		{StudentID: "s-1", FullName: "Alice", ProgressPercent: 75, LessonsCompleted: 3, LessonsTotal: 4, LastActivity: &recent},
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
	if s.LessonsCompleted != 3 || s.LessonsTotal != 4 {
		t.Errorf("lessons = %d/%d, want 3/4", s.LessonsCompleted, s.LessonsTotal)
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

func TestStudentDashboard_Success(t *testing.T) {
	now := time.Now()
	recent := now.Add(-1 * 24 * time.Hour)
	stale := now.Add(-30 * 24 * time.Hour)

	uc := NewAnalyticsUseCase(
		&stubAnalyticsRepository{courseRows: []domain.CourseProgress{
			{CourseID: "c-done", CourseTitle: "Go", ProgressPercent: 100, LessonsCompleted: 5, LessonsTotal: 5, LastActivity: &recent},
			{CourseID: "c-mid", CourseTitle: "SQL", ProgressPercent: 60, LessonsCompleted: 3, LessonsTotal: 5, LastActivity: &recent},
			{CourseID: "c-stale", CourseTitle: "Rust", ProgressPercent: 80, LessonsCompleted: 4, LessonsTotal: 5, LastActivity: &stale},
		}},
		&stubCatalogRepository{},
		domain.DefaultRiskThresholds,
	)

	res, err := uc.StudentDashboard(context.Background(), "student-1")
	if err != nil {
		t.Fatalf("StudentDashboard: %v", err)
	}
	if res.CoursesCompleted != 1 {
		t.Errorf("courses_completed = %d, want 1", res.CoursesCompleted)
	}
	wantOverall := (100.0 + 60.0 + 80.0) / 3
	if res.OverallProgress != wantOverall {
		t.Errorf("overall_progress = %v, want %v", res.OverallProgress, wantOverall)
	}
	if len(res.Courses) != 3 {
		t.Fatalf("courses = %d, want 3", len(res.Courses))
	}
	if res.Courses[2].Status != domain.RiskAtRisk {
		t.Errorf("c-stale status = %q, want AT_RISK (inactive)", res.Courses[2].Status)
	}
}

func TestStudentDashboard_NoEnrollments(t *testing.T) {
	uc := NewAnalyticsUseCase(&stubAnalyticsRepository{}, &stubCatalogRepository{}, domain.DefaultRiskThresholds)
	res, err := uc.StudentDashboard(context.Background(), "student-1")
	if err != nil {
		t.Fatalf("StudentDashboard: %v", err)
	}
	if res.OverallProgress != 0 || res.CoursesCompleted != 0 || len(res.Courses) != 0 {
		t.Fatalf("no-enrollment dashboard = %+v, want zero values", res)
	}
}

func TestStudentDashboard_AnalyticsError(t *testing.T) {
	analyticsRepo := &stubAnalyticsRepository{courseErr: errors.New("rollup unavailable")}
	uc := NewAnalyticsUseCase(analyticsRepo, &stubCatalogRepository{}, domain.DefaultRiskThresholds)
	_, err := uc.StudentDashboard(context.Background(), "student-1")
	if err == nil {
		t.Fatal("expected error when analytics repo fails")
	}
}
