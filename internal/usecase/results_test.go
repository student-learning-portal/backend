package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
)

type fakeResultsRepo struct {
	courses []domain.CourseResult
	err     error
}

func (f *fakeResultsRepo) StudentResults(_ context.Context, _ string) ([]domain.CourseResult, error) {
	return f.courses, f.err
}

func TestMyResults_Empty(t *testing.T) {
	uc := NewResultsUseCase(&fakeResultsRepo{courses: []domain.CourseResult{}})
	got, err := uc.MyResults(context.Background(), "u1")
	if err != nil {
		t.Fatalf("MyResults: %v", err)
	}
	if got.CoursesEnrolled != 0 || got.CoursesCompleted != 0 || got.OverallProgressPercent != 0 {
		t.Errorf("empty results = %+v, want all zero", got)
	}
	if got.Courses == nil {
		t.Error("Courses should be a non-nil slice so it serializes as []")
	}
}

func TestMyResults_PerCourseAndOverall(t *testing.T) {
	uc := NewResultsUseCase(&fakeResultsRepo{courses: []domain.CourseResult{
		{CourseID: "a", Title: "A", LessonsTotal: 5, LessonsCompleted: 2}, // 40%, partial
		{CourseID: "b", Title: "B", LessonsTotal: 3, LessonsCompleted: 3}, // 100%, complete
	}})

	got, err := uc.MyResults(context.Background(), "u1")
	if err != nil {
		t.Fatalf("MyResults: %v", err)
	}
	if got.Courses[0].ProgressPercent != 40 {
		t.Errorf("course A percent = %v, want 40", got.Courses[0].ProgressPercent)
	}
	if got.Courses[1].ProgressPercent != 100 {
		t.Errorf("course B percent = %v, want 100", got.Courses[1].ProgressPercent)
	}
	if got.CoursesEnrolled != 2 {
		t.Errorf("courses_enrolled = %d, want 2", got.CoursesEnrolled)
	}
	if got.CoursesCompleted != 1 {
		t.Errorf("courses_completed = %d, want 1 (only B)", got.CoursesCompleted)
	}
	// Overall is weighted by lesson count: (2 + 3) / (5 + 3) = 62.5%.
	if got.OverallProgressPercent != 62.5 {
		t.Errorf("overall = %v, want 62.5", got.OverallProgressPercent)
	}
}

func TestMyResults_RoundsToTwoDecimals(t *testing.T) {
	uc := NewResultsUseCase(&fakeResultsRepo{courses: []domain.CourseResult{
		{CourseID: "a", Title: "A", LessonsTotal: 3, LessonsCompleted: 1}, // 33.333...%
	}})
	got, err := uc.MyResults(context.Background(), "u1")
	if err != nil {
		t.Fatalf("MyResults: %v", err)
	}
	if got.Courses[0].ProgressPercent != 33.33 {
		t.Errorf("percent = %v, want 33.33", got.Courses[0].ProgressPercent)
	}
}

func TestMyResults_CourseWithNoLessons(t *testing.T) {
	uc := NewResultsUseCase(&fakeResultsRepo{courses: []domain.CourseResult{
		{CourseID: "a", Title: "A", LessonsTotal: 0, LessonsCompleted: 0},
	}})
	got, err := uc.MyResults(context.Background(), "u1")
	if err != nil {
		t.Fatalf("MyResults: %v", err)
	}
	if got.Courses[0].ProgressPercent != 0 {
		t.Errorf("no-lesson course percent = %v, want 0", got.Courses[0].ProgressPercent)
	}
	// A course with no lessons is not "completed".
	if got.CoursesCompleted != 0 {
		t.Errorf("courses_completed = %d, want 0", got.CoursesCompleted)
	}
	if got.OverallProgressPercent != 0 {
		t.Errorf("overall = %v, want 0", got.OverallProgressPercent)
	}
}

func TestMyResults_RepoError(t *testing.T) {
	uc := NewResultsUseCase(&fakeResultsRepo{err: errors.New("db down")})
	_, err := uc.MyResults(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}
