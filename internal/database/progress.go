package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/student-learning-portal/backend/internal/domain"
)

type PostgresProgressRepository struct {
	db *sql.DB
}

func NewPostgresProgressRepository(db *sql.DB) domain.ProgressRepository {
	return &PostgresProgressRepository{db: db}
}

// Save upserts the learner's resume point. The progress_state primary key is
// (actor_id, course_id, lesson_id), so a repeat save overwrites the position and
// percent in place rather than creating a duplicate row.
func (r *PostgresProgressRepository) Save(ctx context.Context, p domain.ProgressState) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO progress_state (actor_id, course_id, lesson_id, position_ms, percent_complete, updated_at)
		 VALUES ($1, $2, $3, $4, $5, now())
		 ON CONFLICT (actor_id, course_id, lesson_id)
		 DO UPDATE SET position_ms = EXCLUDED.position_ms,
		               percent_complete = EXCLUDED.percent_complete,
		               updated_at = now()`,
		p.ActorID, p.CourseID, p.LessonID, p.PositionMs, p.PercentComplete,
	)
	if err != nil {
		return fmt.Errorf("save progress: %w", err)
	}
	return nil
}

// Get returns the learner's saved resume point for a lesson, or
// domain.ErrProgressNotFound if they have not started it yet.
func (r *PostgresProgressRepository) Get(ctx context.Context, actorID, courseID, lessonID string) (domain.ProgressState, error) {
	var p domain.ProgressState
	err := r.db.QueryRowContext(ctx,
		`SELECT actor_id, course_id, lesson_id, position_ms, percent_complete, updated_at
		 FROM progress_state
		 WHERE actor_id = $1 AND course_id = $2 AND lesson_id = $3`,
		actorID, courseID, lessonID,
	).Scan(&p.ActorID, &p.CourseID, &p.LessonID, &p.PositionMs, &p.PercentComplete, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ProgressState{}, domain.ErrProgressNotFound
	}
	if err != nil {
		return domain.ProgressState{}, fmt.Errorf("get progress: %w", err)
	}
	return p, nil
}
