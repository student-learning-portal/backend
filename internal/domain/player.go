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

type LessonRepository interface {
	GetLesson(ctx context.Context, courseID, lessonID string) (Lesson, error)
}
