package usecase

import (
	"context"
	"fmt"

	"github.com/student-learning-portal/backend/internal/domain"
)

type CatalogUseCase struct {
	repo    domain.CatalogRepository
	lessons domain.LessonRepository
}

// NewCatalogUseCase wires the use case to its catalog and lesson repositories.
func NewCatalogUseCase(repo domain.CatalogRepository, lessons domain.LessonRepository) *CatalogUseCase {
	return &CatalogUseCase{repo: repo, lessons: lessons}
}

// ListCourses filters, searches, sorts, and paginates the course catalog.
func (uc *CatalogUseCase) ListCourses(params domain.CourseListParams) []domain.Course {
	courses, _, err := uc.repo.GetCourses(params)
	if err != nil {
		return []domain.Course{}
	}
	return courses
}

// GetCourseLessons returns all lessons for the given course ordered by position.
// Returns an empty slice (not an error) when the course exists but has no lessons.
func (uc *CatalogUseCase) GetCourseLessons(ctx context.Context, courseID string) ([]domain.Lesson, error) {
	if courseID == "" {
		return nil, fmt.Errorf("%w: course_id is required", ErrValidation)
	}
	return uc.lessons.GetLessonsByCourseID(ctx, courseID)
}
