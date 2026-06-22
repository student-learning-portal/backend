package domain

import (
	"context"
	"errors"
	"time"
)

// ErrProgressNotFound is returned when a learner has no saved progress for a lesson yet.
var ErrProgressNotFound = errors.New("progress not found")

// ProgressState is a learner's current resume point within a single lesson.
// It is mutable (one row per actor+course+lesson); the full history lives in event_log.
type ProgressState struct {
	ActorID         string
	CourseID        string
	LessonID        string
	PositionMs      int
	PercentComplete float64
	UpdatedAt       time.Time
}

// ProgressRepository persists and retrieves per-learner, per-lesson resume points.
type ProgressRepository interface {
	// Save upserts the learner's resume point for a lesson. Re-saving the same
	// (actor, course, lesson) overwrites the previous position rather than inserting.
	Save(ctx context.Context, p ProgressState) error
	// Get returns the learner's saved progress for a lesson, or ErrProgressNotFound
	// if they have not started it yet.
	Get(ctx context.Context, actorID, courseID, lessonID string) (ProgressState, error)
}
