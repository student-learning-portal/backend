package usecase

import (
	"context"
	"fmt"

	"github.com/student-learning-portal/backend/internal/domain"
)

type UserCoursesUseCase struct {
	catalog      domain.CatalogRepository
	entitlements domain.EntitlementRepository
}

func NewUserCoursesUseCase(catalog domain.CatalogRepository, entitlements domain.EntitlementRepository) *UserCoursesUseCase {
	return &UserCoursesUseCase{catalog: catalog, entitlements: entitlements}
}

// MyCourses returns courses owned by the actor based on their role:
// teachers get every course they created; students get every course they
// have an active (non-revoked) entitlement for.
func (uc *UserCoursesUseCase) MyCourses(ctx context.Context, actorID string, role domain.Role) ([]domain.Course, error) {
	switch role {
	case domain.RoleTeacher:
		courses, err := uc.catalog.GetByTeacherID(ctx, actorID)
		if err != nil {
			return nil, fmt.Errorf("my courses: %w", err)
		}
		return courses, nil
	case domain.RoleStudent:
		courses, err := uc.entitlements.GetEnrolledCourses(ctx, actorID)
		if err != nil {
			return nil, fmt.Errorf("my courses: %w", err)
		}
		return courses, nil
	default:
		return nil, fmt.Errorf("%w: unknown role %q", ErrValidation, role)
	}
}
