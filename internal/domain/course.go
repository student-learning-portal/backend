package domain

import "time"

// Course represents the core data structure for a learning course
type Course struct {
	ID          string    `json:"id"`
	TeacherID   string    `json:"teacher_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Price       float64   `json:"price"`
	Currency    string    `json:"currency"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CatalogRepository interface {
	GetCourses(search string, minPrice, maxPrice *float64, page, pageSize int) ([]Course, int, error)
}
