package usecase

import (
	"context"

	"github.com/student-learning-portal/backend/internal/domain"
)

// ReviewUseCase is a thin pass-through to domain.ReviewRepository (backed by
// internal/practicum). There is no local gating logic to apply — the
// practicum-team service already enforces enrollment/progress rules and
// review uniqueness; duplicating that here would be exactly the kind of
// reimplementation the integration is meant to avoid.
type ReviewUseCase struct {
	reviews domain.ReviewRepository
}

func NewReviewUseCase(reviews domain.ReviewRepository) *ReviewUseCase {
	return &ReviewUseCase{reviews: reviews}
}

// CreateReview submits studentID's rating (and optional text) for courseID.
func (uc *ReviewUseCase) CreateReview(
	ctx context.Context, studentID, courseID string, rating int, text string,
) (domain.CourseReview, error) {
	return uc.reviews.CreateReview(ctx, studentID, courseID, rating, text)
}

// RatingSummary returns the aggregate rating shown on a course's public page.
func (uc *ReviewUseCase) RatingSummary(ctx context.Context, courseID string) (domain.CourseRatingSummary, error) {
	return uc.reviews.RatingSummary(ctx, courseID)
}
