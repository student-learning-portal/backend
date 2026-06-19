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

// ListCourses filters, searches, and paginates the course catalog.
func (uc *CatalogUseCase) ListCourses(search string, minPrice, maxPrice *float64, page, pageSize int) []domain.Course {
	courses, _, err := uc.repo.GetCourses(search, minPrice, maxPrice, page, pageSize)
	if err != nil {
		return []domain.Course{}
	}
	return courses
}
