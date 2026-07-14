package domain

import (
	"context"
	"errors"
	"time"
)

// ErrInvalidRating is returned when a submitted score falls outside the
// 1-10 scale.
var ErrInvalidRating = errors.New("rating must be between 1 and 10")

// ErrRatingNotFound is returned when a student has not yet rated the given
// course/teacher.
var ErrRatingNotFound = errors.New("rating not found")

// RatingSummary is the aggregate shown on a course or teacher's public page.
// It is always derived from the underlying rating rows, never stored/updated
// directly, so it stays correct-by-construction.
type RatingSummary struct {
	AverageScore float64
	RatingsCount int
}

// CourseRating is one student's 1-10 score for a course. A student holds at
// most one rating per course — resubmitting overwrites their existing score.
type CourseRating struct {
	ID        string
	StudentID string
	CourseID  string
	Score     int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CourseRatingRepository persists course ratings, always by an enrolled
// student, in a table dedicated to courses.
type CourseRatingRepository interface {
	// Upsert records studentID's score for courseID, overwriting any prior
	// score from the same student for the same course.
	Upsert(ctx context.Context, studentID, courseID string, score int) (CourseRating, error)
	Summary(ctx context.Context, courseID string) (RatingSummary, error)
	// GetByStudent returns studentID's own rating for courseID, or
	// ErrRatingNotFound if they haven't rated it.
	GetByStudent(ctx context.Context, studentID, courseID string) (CourseRating, error)
}

// TeacherRating is one student's 1-10 score for a teacher. A student holds at
// most one rating per teacher — resubmitting overwrites their existing score.
type TeacherRating struct {
	ID        string
	StudentID string
	TeacherID string
	Score     int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TeacherRatingRepository persists teacher ratings, always by an enrolled
// student, in a table dedicated to teachers.
type TeacherRatingRepository interface {
	// Upsert records studentID's score for teacherID, overwriting any prior
	// score from the same student for the same teacher.
	Upsert(ctx context.Context, studentID, teacherID string, score int) (TeacherRating, error)
	Summary(ctx context.Context, teacherID string) (RatingSummary, error)
	// GetByStudent returns studentID's own rating for teacherID, or
	// ErrRatingNotFound if they haven't rated them.
	GetByStudent(ctx context.Context, studentID, teacherID string) (TeacherRating, error)
}
