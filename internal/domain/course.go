package domain

// Course represents the core data structure for a learning course
type Course struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Category    string  `json:"category"`
	Price       float64 `json:"price"`
}

type CatalogRepository interface {
	GetCourses(search, category string, page, pageSize int) ([]Course, int, error)
}
