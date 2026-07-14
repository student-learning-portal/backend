package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
)

// stubCatalogRepository implements domain.CatalogRepository for catalog and
// payment/analytics tests in this package.
type stubCatalogRepository struct {
	courses    []domain.Course
	coursesErr error
	course     domain.Course
	courseErr  error
}

func (s *stubCatalogRepository) GetCourses(_ domain.CourseListParams) ([]domain.Course, int, error) {
	return s.courses, len(s.courses), s.coursesErr
}

func (s *stubCatalogRepository) GetByID(_ context.Context, _ string) (domain.Course, error) {
	return s.course, s.courseErr
}

func (s *stubCatalogRepository) GetByTeacherID(_ context.Context, _ string) ([]domain.Course, error) {
	return s.courses, s.coursesErr
}

func (s *stubCatalogRepository) Create(_ context.Context, course domain.Course) (domain.Course, error) {
	return course, nil
}

func (s *stubCatalogRepository) Update(_ context.Context, course domain.Course) (domain.Course, error) {
	return course, nil
}

func (s *stubCatalogRepository) Delete(_ context.Context, _ string) error { return nil }

func (s *stubCatalogRepository) GetExternalCourseID(_ context.Context, _ string) (string, bool, error) {
	return "", false, nil
}

func (s *stubCatalogRepository) SetExternalCourseID(_ context.Context, _, _ string) error {
	return nil
}

func TestListCourses_ReturnsCourses(t *testing.T) {
	repo := &stubCatalogRepository{
		courses: []domain.Course{{ID: "c1", Title: "Math"}, {ID: "c2", Title: "Science"}},
	}
	uc := NewCatalogUseCase(repo, &fakeLessonRepo{})
	courses := uc.ListCourses(domain.CourseListParams{})
	if len(courses) != 2 {
		t.Errorf("len = %d, want 2", len(courses))
	}
	if courses[0].ID != "c1" {
		t.Errorf("courses[0].ID = %q, want c1", courses[0].ID)
	}
}

func TestListCourses_ErrorReturnsEmpty(t *testing.T) {
	repo := &stubCatalogRepository{coursesErr: errors.New("db down")}
	uc := NewCatalogUseCase(repo, &fakeLessonRepo{})
	courses := uc.ListCourses(domain.CourseListParams{})
	if len(courses) != 0 {
		t.Errorf("len = %d, want 0 on error", len(courses))
	}
}

func TestListCourses_EmptyDatabase(t *testing.T) {
	uc := NewCatalogUseCase(&stubCatalogRepository{}, &fakeLessonRepo{})
	courses := uc.ListCourses(domain.CourseListParams{})
	if len(courses) != 0 {
		t.Errorf("expected 0 courses for empty catalog, got %d", len(courses))
	}
}

func TestListCourses_SingleCourse(t *testing.T) {
	repo := &stubCatalogRepository{
		courses: []domain.Course{{ID: "c1", Title: "Intro to Go", Subject: "programming"}},
	}
	uc := NewCatalogUseCase(repo, &fakeLessonRepo{})
	courses := uc.ListCourses(domain.CourseListParams{Search: "Go", Subject: "programming"})
	if len(courses) != 1 {
		t.Fatalf("len = %d, want 1", len(courses))
	}
	if courses[0].Subject != "programming" {
		t.Errorf("subject = %q, want programming", courses[0].Subject)
	}
}
