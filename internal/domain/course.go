package domain

import (
	"context"
	"errors"
	"time"
)

var ErrCourseNotFound = errors.New("course not found")

// ErrCourseNotDraft is returned when a teacher tries to delete a course that
// has already left the draft stage — published/archived courses may have
// been purchased, so they can only be archived, never deleted.
var ErrCourseNotDraft = errors.New("course must be a draft to delete")

// DifficultyLevel is a course's target skill level, shown in the catalog and
// filterable via CourseListParams.Difficulty.
type DifficultyLevel string

const (
	DifficultyBeginner     DifficultyLevel = "beginner"
	DifficultyIntermediate DifficultyLevel = "intermediate"
	DifficultyAdvanced     DifficultyLevel = "advanced"
	DifficultyAllLevels    DifficultyLevel = "all_levels"
)

// Valid reports whether d is one of the known difficulty levels.
func (d DifficultyLevel) Valid() bool {
	switch d {
	case DifficultyBeginner, DifficultyIntermediate, DifficultyAdvanced, DifficultyAllLevels:
		return true
	}
	return false
}

// Course represents the core data structure for a learning course
type Course struct {
	ID          string  `json:"id"`
	TeacherID   string  `json:"teacher_id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Subject     string  `json:"subject"`
	Price       float64 `json:"price"`
	Currency    string  `json:"currency"`
	Status      string  `json:"status"`
	// Difficulty defaults to DifficultyAllLevels when not set by the teacher.
	Difficulty DifficultyLevel `json:"difficulty"`
	// DurationMinutes is a teacher-supplied estimate of total course length,
	// not derived from lesson media (see Media.DurationMs for that).
	DurationMinutes int       `json:"duration_minutes"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// CourseListParams bundles the catalog's search, filter, sort, and
// pagination inputs so the repository signature doesn't keep growing.
type CourseListParams struct {
	Search     string
	Subject    string
	Difficulty string
	MinPrice   *float64
	MaxPrice   *float64
	SortBy     string
	SortOrder  string
	Page       int
	PageSize   int
}

// CatalogRepository persists and queries courses: the public,
// filterable/paginated catalog listing plus per-teacher authoring CRUD.
type CatalogRepository interface {
	GetCourses(params CourseListParams) ([]Course, int, error)
	GetByID(ctx context.Context, id string) (Course, error)
	GetByTeacherID(ctx context.Context, teacherID string) ([]Course, error)
	Create(ctx context.Context, c Course) (Course, error)
	Update(ctx context.Context, c Course) (Course, error)
	Delete(ctx context.Context, id string) error

	// GetExternalCourseID returns the practicum-team course ID this course has
	// been mirrored to (internal/practicum), if any. ok is false if it has
	// never been mirrored — courses.external_course_id is NULL.
	GetExternalCourseID(ctx context.Context, courseID string) (externalID string, ok bool, err error)
	// SetExternalCourseID records the practicum-team course ID a mirror was
	// created under, so future requests reuse it instead of mirroring again.
	SetExternalCourseID(ctx context.Context, courseID, externalID string) error

	// FindByExternalCourseID looks up a course by the reverse direction of the
	// external_course_id link: given the practicum team's own course id, find
	// the local course it was imported as (see the import use case in
	// internal/usecase). ok is false if no course has that external id.
	FindByExternalCourseID(ctx context.Context, externalID string) (Course, bool, error)
}
