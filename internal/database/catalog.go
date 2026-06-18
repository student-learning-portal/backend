package database

import (
	"regexp"
	"strings"

	"github.com/student-learning-portal/backend/internal/domain"
)

var seedCourses = []domain.Course{
	{ID: "course-1", Title: "Introduction to Go", Description: "Learn the basics of Go programming.", Category: "Programming", Price: 49.99},
	{ID: "course-2", Title: "Advanced Go Concurrency", Description: "Master goroutines and channels.", Category: "Programming", Price: 79.99},
	{ID: "course-3", Title: "Fullstack React", Description: "Build modern web apps with React.", Category: "Web Development", Price: 59.99},
	{ID: "course-4", Title: "Intro to Python", Description: "Start your journey in Python.", Category: "Programming", Price: 39.99},
	{ID: "course-5", Title: "Data Science with Pandas", Description: "Data analysis and visualization in Python.", Category: "Data Science", Price: 89.99},
	{ID: "course-6", Title: "Machine Learning A-Z", Description: "Learn to build ML models.", Category: "Data Science", Price: 99.99},
	{ID: "course-7", Title: "Docker for Beginners", Description: "Containerize your applications.", Category: "DevOps", Price: 29.99},
	{ID: "course-8", Title: "Kubernetes Mastery", Description: "Deploy and manage containers at scale.", Category: "DevOps", Price: 89.99},
	{ID: "course-9", Title: "Go Web Services", Description: "Build fast HTTP servers in Go.", Category: "Web Development", Price: 49.99},
	{ID: "course-10", Title: "React Native", Description: "Build mobile apps using React.", Category: "Mobile Development", Price: 59.99},
}

type MockCatalogRepository struct{}

func NewMockCatalogRepository() domain.CatalogRepository {
	return &MockCatalogRepository{}
}

// matchesSearch reports whether every word in the search query appears as a
// whole word in the course's title or description (case-insensitive)
func matchesSearch(search string, c domain.Course) bool {
	haystack := c.Title + " " + c.Description
	for _, word := range strings.Fields(search) {
		pattern := `(?i)\b` + regexp.QuoteMeta(word) + `\b`
		if matched, _ := regexp.MatchString(pattern, haystack); !matched {
			return false
		}
	}
	return true
}

func (m *MockCatalogRepository) GetCourses(search, category string, page, pageSize int) ([]domain.Course, int, error) {
	var filtered []domain.Course

	for _, c := range seedCourses {
		if category != "" && !strings.EqualFold(c.Category, category) {
			continue
		}
		if search != "" && !matchesSearch(search, c) {
			continue
		}
		filtered = append(filtered, c)
	}

	total := len(filtered)

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	start := (page - 1) * pageSize
	if start > total {
		return []domain.Course{}, total, nil
	}

	end := start + pageSize
	if end > total {
		end = total
	}

	return filtered[start:end], total, nil
}
