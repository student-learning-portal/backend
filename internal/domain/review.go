package domain

import (
	"context"
	"errors"
	"time"
)

// These map 1:1 onto error codes returned by the practicum-team integration
// service (internal/practicum) — see ReviewRepository.
var (
	// ErrNotEnrolled: their NOT_ENROLLED.
	ErrNotEnrolled = errors.New("student is not enrolled in this course")
	// ErrInsufficientProgress: their INSUFFICIENT_PROGRESS (more than 50% of
	// lessons must be completed before reviewing).
	ErrInsufficientProgress = errors.New("must complete more than 50% of the course's lessons before reviewing")
	// ErrReviewAlreadyExists: their COMMENT_ALREADY_EXISTS (one review per
	// course per student).
	ErrReviewAlreadyExists = errors.New("course already reviewed by this student")
	// ErrInvalidReview: their INVALID_RATING / COMMENT_TEXT_TOO_LONG / BAD_REQUEST.
	ErrInvalidReview = errors.New("invalid review")
)

// CourseReview is one student's rating (and optional text) for a course.
// The data itself lives in the practicum-team service, not our own database —
// see ReviewRepository.
type CourseReview struct {
	ID        string
	CourseID  string
	StudentID string
	Rating    int
	Text      string
	CreatedAt time.Time
}

// CourseRatingSummary is the aggregate rating shown on a course's public page.
type CourseRatingSummary struct {
	AverageRating float64
	ReviewsCount  int
}

// ReviewRepository proxies course ratings and reviews to the practicum-team
// integration service (internal/practicum), which already implements the
// enrollment + progress-gated review flow. We do not reimplement that
// business logic here — this interface only exists so the usecase layer
// doesn't depend on the HTTP integration package directly.
type ReviewRepository interface {
	// CreateReview submits a review on behalf of studentID for courseID.
	// Returns ErrNotEnrolled, ErrInsufficientProgress, ErrReviewAlreadyExists,
	// ErrInvalidReview, or ErrCourseNotFound per the remote service's response.
	CreateReview(ctx context.Context, studentID, courseID string, rating int, text string) (CourseReview, error)
	// RatingSummary aggregates every review for a course.
	RatingSummary(ctx context.Context, courseID string) (CourseRatingSummary, error)
}
