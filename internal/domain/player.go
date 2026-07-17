package domain

import (
	"context"
	"errors"
	"time"
)

var ErrLessonNotFound = errors.New("lesson not found")

// ErrMaterialNotFound is returned when a material id doesn't exist under the
// requested lesson.
var ErrMaterialNotFound = errors.New("material not found")

// ErrLessonOrderMismatch is returned when a reorder request's lesson id list
// doesn't exactly match the course's current lessons.
var ErrLessonOrderMismatch = errors.New("lesson order does not match the course's lessons")

// Lesson is one unit of a course's content, ordered within the course by
// Position (see LessonRepository.ReorderLessons).
type Lesson struct {
	ID        string    `json:"id"`
	CourseID  string    `json:"course_id"`
	Title     string    `json:"title"`
	Type      string    `json:"type"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Media is a playable asset (video/audio) attached to a lesson.
type Media struct {
	ID         string `json:"id"`
	LessonID   string `json:"lesson_id"`
	URL        string `json:"url"`
	DurationMs int    `json:"duration_ms"`
	Type       string `json:"type"`
}

// Material is a downloadable attachment (pdf, link, etc.) for a lesson.
type Material struct {
	ID       string `json:"id"`
	LessonID string `json:"lesson_id"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Type     string `json:"type"`
}

// LessonRepository persists lessons and their attached media/materials, and
// backs both the teacher-authoring endpoints and the learner-facing player.
type LessonRepository interface {
	GetLessonsByCourseID(ctx context.Context, courseID string) ([]Lesson, error)
	GetLesson(ctx context.Context, courseID, lessonID string) (Lesson, error)
	GetLessonMedia(ctx context.Context, lessonID string) ([]Media, error)
	GetLessonMaterials(ctx context.Context, lessonID string) ([]Material, error)

	// CreateLesson appends a new lesson to the end of the course (position =
	// max existing position + 1).
	CreateLesson(ctx context.Context, courseID, title, lessonType string) (Lesson, error)
	// UpdateLesson changes a lesson's title/type in place; position is
	// changed only via ReorderLessons.
	UpdateLesson(ctx context.Context, courseID, lessonID, title, lessonType string) (Lesson, error)
	// ReorderLessons sets each lesson's position to its index in orderedIDs.
	// orderedIDs must contain exactly the course's current lesson ids.
	ReorderLessons(ctx context.Context, courseID string, orderedIDs []string) error
	DeleteLesson(ctx context.Context, courseID, lessonID string) error

	// SetLessonMedia replaces the lesson's media asset (a lesson has at most
	// one — the player only ever reads the first one) with the given one.
	SetLessonMedia(ctx context.Context, lessonID, url string, durationMs int, mediaType string) (Media, error)
	DeleteLessonMedia(ctx context.Context, lessonID string) error

	AddMaterial(ctx context.Context, lessonID, title, url, materialType string) (Material, error)
	DeleteMaterial(ctx context.Context, lessonID, materialID string) error

	// FindByExternalID looks up a lesson previously imported from the
	// practicum team's service by their lesson id, so the one-shot import
	// command (internal/usecase) can skip lessons it already copied over
	// instead of duplicating them on a re-run. ok is false if none matches.
	FindByExternalID(ctx context.Context, externalLessonID string) (Lesson, bool, error)
	// SetExternalLessonID records the practicum-team lesson id a lesson was
	// imported from.
	SetExternalLessonID(ctx context.Context, lessonID, externalID string) error
}
