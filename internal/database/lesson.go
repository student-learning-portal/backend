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

// CreateLesson appends a new lesson at the end of the course (position = the
// current max + 1, computed in the same INSERT so there's no read-then-write race).
func (r *PostgresLessonRepository) CreateLesson(ctx context.Context, courseID, title, lessonType string) (domain.Lesson, error) {
	var l domain.Lesson
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO lessons (course_id, title, lesson_type, position)
		 SELECT $1, $2, $3, COALESCE(MAX(position), -1) + 1 FROM lessons WHERE course_id = $1
		 RETURNING id, course_id, title, lesson_type, position, created_at, updated_at`,
		courseID, title, lessonType,
	).Scan(&l.ID, &l.CourseID, &l.Title, &l.Type, &l.Position, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		return domain.Lesson{}, fmt.Errorf("create lesson: %w", err)
	}
	return l, nil
}

// UpdateLesson changes a lesson's title/type in place; position is left untouched.
func (r *PostgresLessonRepository) UpdateLesson(ctx context.Context, courseID, lessonID, title, lessonType string) (domain.Lesson, error) {
	var l domain.Lesson
	err := r.db.QueryRowContext(ctx,
		`UPDATE lessons SET title = $1, lesson_type = $2, updated_at = now()
		 WHERE id = $3 AND course_id = $4
		 RETURNING id, course_id, title, lesson_type, position, created_at, updated_at`,
		title, lessonType, lessonID, courseID,
	).Scan(&l.ID, &l.CourseID, &l.Title, &l.Type, &l.Position, &l.CreatedAt, &l.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Lesson{}, domain.ErrLessonNotFound
	}
	if err != nil {
		return domain.Lesson{}, fmt.Errorf("update lesson: %w", err)
	}
	return l, nil
}

// ReorderLessons sets each lesson's position to its index in orderedIDs.
// orderedIDs must contain exactly the course's current lesson ids (validated
// by comparing counts), else usecase.ErrValidation is returned.
//
// The update happens in two phases inside one transaction: first every
// position is bumped far out of the target 0..N-1 range, then each lesson is
// set to its final position. Without the first phase, updating lessons one at
// a time in the new order could momentarily collide with another lesson still
// holding the position being assigned, tripping the UNIQUE(course_id, position)
// constraint.
func (r *PostgresLessonRepository) ReorderLessons(ctx context.Context, courseID string, orderedIDs []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("reorder lessons: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // best-effort rollback; no-op after Commit

	var count int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM lessons WHERE course_id = $1`, courseID).Scan(&count); err != nil {
		return fmt.Errorf("reorder lessons: count: %w", err)
	}
	if count != len(orderedIDs) {
		return fmt.Errorf("%w: order must list exactly the course's %d lesson(s)", domain.ErrLessonOrderMismatch, count)
	}

	const positionOffset = 100000
	if _, err = tx.ExecContext(ctx, `UPDATE lessons SET position = position + $1 WHERE course_id = $2`, positionOffset, courseID); err != nil {
		return fmt.Errorf("reorder lessons: offset: %w", err)
	}

	var res sql.Result
	for idx, id := range orderedIDs {
		res, err = tx.ExecContext(ctx,
			`UPDATE lessons SET position = $1, updated_at = now() WHERE id = $2 AND course_id = $3`,
			idx, id, courseID,
		)
		if err != nil {
			return fmt.Errorf("reorder lessons: set position: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: lesson %s does not belong to this course", domain.ErrLessonOrderMismatch, id)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("reorder lessons: commit: %w", err)
	}
	return nil
}

// DeleteLesson removes a lesson; its media/materials cascade via their FKs.
func (r *PostgresLessonRepository) DeleteLesson(ctx context.Context, courseID, lessonID string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM lessons WHERE id = $1 AND course_id = $2`, lessonID, courseID)
	if err != nil {
		return fmt.Errorf("delete lesson: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrLessonNotFound
	}
	return nil
}

// SetLessonMedia replaces any existing media row for the lesson with a single
// new one — the player only ever reads the first media row for a lesson, so
// this keeps "one playable asset per lesson" true at the write path too.
func (r *PostgresLessonRepository) SetLessonMedia(
	ctx context.Context, lessonID, url string, durationMs int, mediaType string,
) (domain.Media, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Media{}, fmt.Errorf("set lesson media: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // best-effort rollback; no-op after Commit

	if _, err = tx.ExecContext(ctx, `DELETE FROM media WHERE lesson_id = $1`, lessonID); err != nil {
		return domain.Media{}, fmt.Errorf("set lesson media: clear: %w", err)
	}

	var m domain.Media
	err = tx.QueryRowContext(ctx,
		`INSERT INTO media (lesson_id, url, duration_ms, media_type)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, lesson_id, url, COALESCE(duration_ms, 0), media_type`,
		lessonID, url, durationMs, mediaType,
	).Scan(&m.ID, &m.LessonID, &m.URL, &m.DurationMs, &m.Type)
	if err != nil {
		return domain.Media{}, fmt.Errorf("set lesson media: insert: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return domain.Media{}, fmt.Errorf("set lesson media: commit: %w", err)
	}
	return m, nil
}

// DeleteLessonMedia removes the lesson's media asset, if any.
func (r *PostgresLessonRepository) DeleteLessonMedia(ctx context.Context, lessonID string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM media WHERE lesson_id = $1`, lessonID); err != nil {
		return fmt.Errorf("delete lesson media: %w", err)
	}
	return nil
}

// AddMaterial attaches a new downloadable material to a lesson.
func (r *PostgresLessonRepository) AddMaterial(ctx context.Context, lessonID, title, url, materialType string) (domain.Material, error) {
	var m domain.Material
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO materials (lesson_id, title, url, material_type)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, lesson_id, title, url, material_type`,
		lessonID, title, url, materialType,
	).Scan(&m.ID, &m.LessonID, &m.Title, &m.URL, &m.Type)
	if err != nil {
		return domain.Material{}, fmt.Errorf("add material: %w", err)
	}
	return m, nil
}

// DeleteMaterial removes a single material from a lesson.
func (r *PostgresLessonRepository) DeleteMaterial(ctx context.Context, lessonID, materialID string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM materials WHERE id = $1 AND lesson_id = $2`, materialID, lessonID)
	if err != nil {
		return fmt.Errorf("delete material: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrMaterialNotFound
	}
	return nil
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
