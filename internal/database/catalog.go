package database

import (
	"database/sql"
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
		conditions = append(conditions, fmt.Sprintf("(title ILIKE $%d OR description ILIKE $%d)", len(args)+1, len(args)+2))
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

	query := fmt.Sprintf(
		`SELECT id, teacher_id, title, description, subject, price, currency, status, created_at, updated_at
		 FROM courses %s
		 ORDER BY %s %s
		 LIMIT $%d OFFSET $%d`,
		where, sortBy, sortOrder, len(args)+1, len(args)+2,
	)
	args = append(args, pageSize, offset)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query courses: %w", err)
	}
	defer rows.Close()

	courses := []domain.Course{}
	for rows.Next() {
		var c domain.Course
		if err := rows.Scan(&c.ID, &c.TeacherID, &c.Title, &c.Description, &c.Subject, &c.Price, &c.Currency, &c.Status, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan course: %w", err)
		}
		courses = append(courses, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate courses: %w", err)
	}

	return courses, total, nil
}
