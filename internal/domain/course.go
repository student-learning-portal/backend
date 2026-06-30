package domain

import (
	"context"
	"errors"
	"time"
)

var ErrCourseNotFound = errors.New("course not found")

// Course represents the core data structure for a learning course
type Course struct {
	ID          string    `json:"id"`
	TeacherID   string    `json:"teacher_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Subject     string    `json:"subject"`
	Price       float64   `json:"price"`
	Currency    string    `json:"currency"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CourseListParams bundles the catalog's search, filter, sort, and
// pagination inputs so the repository signature doesn't keep growing.
type CourseListParams struct {
	Search    string
	Subject   string
	MinPrice  *float64
	MaxPrice  *float64
	SortBy    string
	SortOrder string
	Page      int
	PageSize  int
}

type CatalogRepository interface {
	GetCourses(params CourseListParams) ([]Course, int, error)
	GetByID(ctx context.Context, id string) (Course, error)
	GetByTeacherID(ctx context.Context, teacherID string) ([]Course, error)
}
