package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/student-learning-portal/backend/internal/domain"
)

const coursePublished = "published"

// allowed sort columns for the catalog endpoint; anything else falls back to the default.
var courseSortFields = map[string]bool{
	"title":      true,
	"price":      true,
	"subject":    true,
	"created_at": true,
}

type PostgresCatalogRepository struct {
	db *sql.DB
}

func NewPostgresCatalogRepository(db *sql.DB) domain.CatalogRepository {
	return &PostgresCatalogRepository{db: db}
}

func (r *PostgresCatalogRepository) GetCourses(params domain.CourseListParams) ([]domain.Course, int, error) {
	var (
		conditions []string
		args       []any
	)
	conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)+1))
	args = append(args, coursePublished)

	if params.Search != "" {
		titleIdx := len(args) + 1
		conditions = append(conditions, fmt.Sprintf("(title ILIKE $%d OR description ILIKE $%d)", titleIdx, titleIdx+1))
		args = append(args, "%"+params.Search+"%", "%"+params.Search+"%")
	}
	if params.Subject != "" {
		conditions = append(conditions, fmt.Sprintf("subject ILIKE $%d", len(args)+1))
		args = append(args, "%"+params.Subject+"%")
	}
	if params.MinPrice != nil {
		conditions = append(conditions, fmt.Sprintf("price >= $%d", len(args)+1))
		args = append(args, *params.MinPrice)
	}
	if params.MaxPrice != nil {
		conditions = append(conditions, fmt.Sprintf("price <= $%d", len(args)+1))
		args = append(args, *params.MaxPrice)
	}
	if params.Difficulty != "" {
		conditions = append(conditions, fmt.Sprintf("difficulty = $%d", len(args)+1))
		args = append(args, params.Difficulty)
	}

	where := "WHERE " + strings.Join(conditions, " AND ")

	var total int
	countQuery := "SELECT count(*) FROM courses " + where
	if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count courses: %w", err)
	}

	sortBy := strings.ToLower(params.SortBy)
	if !courseSortFields[sortBy] {
		sortBy = "title"
	}
	sortOrder := "ASC"
	if strings.EqualFold(params.SortOrder, "desc") {
		sortOrder = "DESC"
	}

	page, pageSize := params.Page, params.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	limitIdx := len(args) + 1
	//nolint:gosec // sortBy validated against allowlist; sortOrder is "ASC"/"DESC" only; where uses parameterized placeholders
	query := fmt.Sprintf(
		`SELECT id, teacher_id, title, description, subject, price, currency, status, difficulty, duration_minutes, created_at, updated_at
		 FROM courses %s
		 ORDER BY %s %s
		 LIMIT $%d OFFSET $%d`,
		where, sortBy, sortOrder, limitIdx, limitIdx+1,
	)
	args = append(args, pageSize, offset)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query courses: %w", err)
	}
	defer rows.Close()

	courses, err := scanCourseRows(rows)
	if err != nil {
		return nil, 0, err
	}
	return courses, total, nil
}

func (r *PostgresCatalogRepository) GetByTeacherID(ctx context.Context, teacherID string) ([]domain.Course, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, teacher_id, title, description, subject, price, currency, status, difficulty, duration_minutes, created_at, updated_at
		 FROM courses WHERE teacher_id = $1 ORDER BY created_at DESC`,
		teacherID,
	)
	if err != nil {
		return nil, fmt.Errorf("get courses by teacher: %w", err)
	}
	defer rows.Close()
	return scanCourseRows(rows)
}

// scanCourseRows drains a *sql.Rows result set into a Course slice.
// Callers are responsible for calling rows.Close().
func scanCourseRows(rows *sql.Rows) ([]domain.Course, error) {
	courses := []domain.Course{}
	for rows.Next() {
		var c domain.Course
		if err := rows.Scan(
			&c.ID, &c.TeacherID, &c.Title, &c.Description,
			&c.Subject, &c.Price, &c.Currency, &c.Status,
			&c.Difficulty, &c.DurationMinutes,
			&c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan course row: %w", err)
		}
		courses = append(courses, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate course rows: %w", err)
	}
	return courses, nil
}

// Create inserts a new course owned by c.TeacherID. Status/currency defaults
// (and difficulty, if unset) are applied by the caller (usecase layer) before
// this is called.
func (r *PostgresCatalogRepository) Create(ctx context.Context, c domain.Course) (domain.Course, error) {
	var out domain.Course
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO courses (teacher_id, title, description, subject, price, currency, difficulty, duration_minutes)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, teacher_id, title, description, subject, price, currency, status, difficulty, duration_minutes, created_at, updated_at`,
		c.TeacherID, c.Title, c.Description, c.Subject, c.Price, c.Currency, c.Difficulty, c.DurationMinutes,
	).Scan(
		&out.ID, &out.TeacherID, &out.Title, &out.Description, &out.Subject, &out.Price, &out.Currency, &out.Status,
		&out.Difficulty, &out.DurationMinutes, &out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		return domain.Course{}, fmt.Errorf("create course: %w", err)
	}
	return out, nil
}

// Update overwrites every editable field of an existing course.
func (r *PostgresCatalogRepository) Update(ctx context.Context, c domain.Course) (domain.Course, error) {
	var out domain.Course
	err := r.db.QueryRowContext(ctx,
		`UPDATE courses
		 SET title = $1, description = $2, subject = $3, price = $4, currency = $5, status = $6,
		     difficulty = $7, duration_minutes = $8, updated_at = now()
		 WHERE id = $9
		 RETURNING id, teacher_id, title, description, subject, price, currency, status, difficulty, duration_minutes, created_at, updated_at`,
		c.Title, c.Description, c.Subject, c.Price, c.Currency, c.Status, c.Difficulty, c.DurationMinutes, c.ID,
	).Scan(
		&out.ID, &out.TeacherID, &out.Title, &out.Description, &out.Subject, &out.Price, &out.Currency, &out.Status,
		&out.Difficulty, &out.DurationMinutes, &out.CreatedAt, &out.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Course{}, domain.ErrCourseNotFound
	}
	if err != nil {
		return domain.Course{}, fmt.Errorf("update course: %w", err)
	}
	return out, nil
}

// GetExternalCourseID returns the practicum-team course ID this course has
// been mirrored to, if any.
func (r *PostgresCatalogRepository) GetExternalCourseID(ctx context.Context, courseID string) (string, bool, error) {
	var externalID sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT external_course_id FROM courses WHERE id = $1`, courseID,
	).Scan(&externalID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, domain.ErrCourseNotFound
	}
	if err != nil {
		return "", false, fmt.Errorf("get external course id: %w", err)
	}
	return externalID.String, externalID.Valid, nil
}

// SetExternalCourseID records the practicum-team course ID a mirror was
// created under.
func (r *PostgresCatalogRepository) SetExternalCourseID(ctx context.Context, courseID, externalID string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE courses SET external_course_id = $1 WHERE id = $2`, externalID, courseID,
	)
	if err != nil {
		return fmt.Errorf("set external course id: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrCourseNotFound
	}
	return nil
}

// FindByExternalCourseID looks up a course by the practicum team's course id
// (the reverse of GetExternalCourseID), used to make course import idempotent.
func (r *PostgresCatalogRepository) FindByExternalCourseID(ctx context.Context, externalID string) (domain.Course, bool, error) {
	var c domain.Course
	err := r.db.QueryRowContext(ctx,
		`SELECT id, teacher_id, title, description, subject, price, currency, status, difficulty, duration_minutes, created_at, updated_at
		 FROM courses WHERE external_course_id = $1`,
		externalID,
	).Scan(
		&c.ID, &c.TeacherID, &c.Title, &c.Description, &c.Subject, &c.Price, &c.Currency, &c.Status,
		&c.Difficulty, &c.DurationMinutes, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Course{}, false, nil
	}
	if err != nil {
		return domain.Course{}, false, fmt.Errorf("find course by external id: %w", err)
	}
	return c, true, nil
}

// Delete removes a course. Lessons/media/materials cascade via their FKs.
// The usecase layer only calls this for draft courses.
func (r *PostgresCatalogRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM courses WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete course: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrCourseNotFound
	}
	return nil
}

func (r *PostgresCatalogRepository) GetByID(ctx context.Context, id string) (domain.Course, error) {
	var c domain.Course
	err := r.db.QueryRowContext(ctx,
		`SELECT id, teacher_id, title, description, subject, price, currency, status, difficulty, duration_minutes, created_at, updated_at
		 FROM courses WHERE id = $1`,
		id,
	).Scan(
		&c.ID, &c.TeacherID, &c.Title, &c.Description, &c.Subject, &c.Price, &c.Currency, &c.Status,
		&c.Difficulty, &c.DurationMinutes, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Course{}, domain.ErrCourseNotFound
	}
	if err != nil {
		return domain.Course{}, fmt.Errorf("get course by id: %w", err)
	}
	return c, nil
}
