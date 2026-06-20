package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/student-learning-portal/backend/internal/domain"
)

type PostgresLessonRepository struct {
	db *sql.DB
}

func NewPostgresLessonRepository(db *sql.DB) domain.LessonRepository {
	return &PostgresLessonRepository{db: db}
}

func (r *PostgresLessonRepository) GetLesson(ctx context.Context, courseID, lessonID string) (domain.Lesson, error) {
	var l domain.Lesson
	err := r.db.QueryRowContext(ctx,
		`SELECT id, course_id, title, lesson_type, position, created_at, updated_at
		 FROM lessons WHERE id = $1 AND course_id = $2`,
		lessonID, courseID,
	).Scan(&l.ID, &l.CourseID, &l.Title, &l.Type, &l.Position, &l.CreatedAt, &l.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Lesson{}, domain.ErrLessonNotFound
	}
	if err != nil {
		return domain.Lesson{}, fmt.Errorf("get lesson: %w", err)
	}
	return l, nil
}
