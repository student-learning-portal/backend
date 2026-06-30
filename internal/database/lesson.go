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

func (r *PostgresLessonRepository) GetLessonsByCourseID(ctx context.Context, courseID string) ([]domain.Lesson, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, course_id, title, lesson_type, position, created_at, updated_at
		 FROM lessons WHERE course_id = $1 ORDER BY position`,
		courseID,
	)
	if err != nil {
		return nil, fmt.Errorf("get lessons by course: %w", err)
	}
	defer rows.Close()

	lessons := []domain.Lesson{}
	for rows.Next() {
		var l domain.Lesson
		if err := rows.Scan(&l.ID, &l.CourseID, &l.Title, &l.Type, &l.Position, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan lesson row: %w", err)
		}
		lessons = append(lessons, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lesson rows: %w", err)
	}
	return lessons, nil
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

func (r *PostgresLessonRepository) GetLessonMedia(ctx context.Context, lessonID string) ([]domain.Media, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, lesson_id, url, COALESCE(duration_ms, 0), media_type
		 FROM media WHERE lesson_id = $1 ORDER BY created_at`,
		lessonID,
	)
	if err != nil {
		return nil, fmt.Errorf("get lesson media: %w", err)
	}
	defer rows.Close()

	media := []domain.Media{}
	for rows.Next() {
		var m domain.Media
		if err := rows.Scan(&m.ID, &m.LessonID, &m.URL, &m.DurationMs, &m.Type); err != nil {
			return nil, fmt.Errorf("scan media: %w", err)
		}
		media = append(media, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate media: %w", err)
	}
	return media, nil
}

func (r *PostgresLessonRepository) GetLessonMaterials(ctx context.Context, lessonID string) ([]domain.Material, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, lesson_id, title, url, material_type
		 FROM materials WHERE lesson_id = $1 ORDER BY created_at`,
		lessonID,
	)
	if err != nil {
		return nil, fmt.Errorf("get lesson materials: %w", err)
	}
	defer rows.Close()

	materials := []domain.Material{}
	for rows.Next() {
		var m domain.Material
		if err := rows.Scan(&m.ID, &m.LessonID, &m.Title, &m.URL, &m.Type); err != nil {
			return nil, fmt.Errorf("scan material: %w", err)
		}
		materials = append(materials, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate materials: %w", err)
	}
	return materials, nil
}
