package usecase

import (
	"github.com/student-learning-portal/backend/internal/domain"
)

type CatalogUseCase struct {
	repo domain.CatalogRepository
}

// NewCatalogUseCase wires the use case to its catalog repository.
func NewCatalogUseCase(repo domain.CatalogRepository) *CatalogUseCase {
	return &CatalogUseCase{repo: repo}
}

// ListCourses filters, searches, sorts, and paginates the course catalog.
func (uc *CatalogUseCase) ListCourses(params domain.CourseListParams) []domain.Course {
	courses, _, err := uc.repo.GetCourses(params)
	if err != nil {
		return []domain.Course{}
	}
	return courses
}
