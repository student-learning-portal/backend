package practicum

import (
	"context"
	"fmt"
	"time"

	"github.com/student-learning-portal/backend/internal/domain"
)

const commentTimeLayout = "2006-01-02T15:04:05Z07:00" // matches their timeRFC3339 constant

// ReviewRepository implements domain.ReviewRepository by proxying to the
// practicum-team service. Course IDs are never shared between our database
// and theirs (both are server-generated UUIDs in independent Postgres
// instances), so the first rating/comment request for a course lazily
// mirrors it into their system via createCourse and caches the returned ID
// on our own courses row (CatalogRepository.SetExternalCourseID) — every
// later call reuses that mapping instead of mirroring again.
type ReviewRepository struct {
	client               *Client
	catalog              domain.CatalogRepository
	integrationTeacherID string
}

// NewReviewRepository wires the integration. integrationTeacherID must be a
// teacher account already registered in the practicum-team system (their
// POST /auth/register_teacher, done once — see docs/practicum-integration.md);
// every mirrored course is owned by this account regardless of which of our
// teachers authored the original.
func NewReviewRepository(client *Client, catalog domain.CatalogRepository, integrationTeacherID string) *ReviewRepository {
	return &ReviewRepository{client: client, catalog: catalog, integrationTeacherID: integrationTeacherID}
}

func (r *ReviewRepository) ensureExternalCourse(ctx context.Context, courseID string) (string, error) {
	externalID, ok, err := r.catalog.GetExternalCourseID(ctx, courseID)
	if err != nil {
		return "", err
	}
	if ok {
		return externalID, nil
	}

	course, err := r.catalog.GetByID(ctx, courseID)
	if err != nil {
		return "", err
	}
	externalID, err = r.client.createCourse(ctx, r.integrationTeacherID, course)
	if err != nil {
		return "", fmt.Errorf("mirror course to practicum service: %w", err)
	}
	if err := r.catalog.SetExternalCourseID(ctx, courseID, externalID); err != nil {
		return "", fmt.Errorf("store external course id: %w", err)
	}
	return externalID, nil
}

// CreateReview mirrors courseID if needed, then submits the review as
// studentID. Their service enforces enrollment + progress gating and
// one-review-per-student — see domain.ReviewRepository's error docs.
func (r *ReviewRepository) CreateReview(
	ctx context.Context, studentID, courseID string, rating int, text string,
) (domain.CourseReview, error) {
	externalID, err := r.ensureExternalCourse(ctx, courseID)
	if err != nil {
		return domain.CourseReview{}, err
	}

	resp, err := r.client.createComment(ctx, studentID, externalID, rating, text)
	if err != nil {
		return domain.CourseReview{}, err
	}

	createdAt, _ := time.Parse(commentTimeLayout, resp.CreatedAt)
	return domain.CourseReview{
		ID: resp.ID,
		// CourseID is ours, not theirs — callers only ever know our course ID.
		CourseID:  courseID,
		StudentID: studentID,
		Rating:    resp.Rating,
		Text:      resp.Text,
		CreatedAt: createdAt,
	}, nil
}

// RatingSummary mirrors courseID if needed, then aggregates its reviews.
func (r *ReviewRepository) RatingSummary(ctx context.Context, courseID string) (domain.CourseRatingSummary, error) {
	externalID, err := r.ensureExternalCourse(ctx, courseID)
	if err != nil {
		return domain.CourseRatingSummary{}, err
	}

	resp, err := r.client.getCourseRating(ctx, externalID)
	if err != nil {
		return domain.CourseRatingSummary{}, err
	}
	return domain.CourseRatingSummary{AverageRating: resp.AverageRating, ReviewsCount: resp.ReviewsCount}, nil
}
