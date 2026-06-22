package domain

import (
	"context"
	"errors"
	"time"
)

var ErrLessonNotFound = errors.New("lesson not found")

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

type LessonRepository interface {
	GetLesson(ctx context.Context, courseID, lessonID string) (Lesson, error)
	GetLessonMedia(ctx context.Context, lessonID string) ([]Media, error)
	GetLessonMaterials(ctx context.Context, lessonID string) ([]Material, error)
}
