package database

import (
	"regexp"
	"strings"
	"time"

	"github.com/student-learning-portal/backend/internal/domain"
)

var seedCoursesCreatedAt = time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)

var seedCourses = []domain.Course{
	{ID: "8f14e45f-ceea-4a3e-8e3f-000000000001", TeacherID: "1b9d6bcd-bbfd-4b2d-9b5d-ab8dfbbd4bed", Title: "Introduction to Go", Description: "Learn the basics of Go programming.", Price: 49.99, Currency: "USD", Status: "published", CreatedAt: seedCoursesCreatedAt, UpdatedAt: seedCoursesCreatedAt},
	{ID: "8f14e45f-ceea-4a3e-8e3f-000000000002", TeacherID: "1b9d6bcd-bbfd-4b2d-9b5d-ab8dfbbd4bed", Title: "Advanced Go Concurrency", Description: "Master goroutines and channels.", Price: 79.99, Currency: "USD", Status: "published", CreatedAt: seedCoursesCreatedAt, UpdatedAt: seedCoursesCreatedAt},
	{ID: "8f14e45f-ceea-4a3e-8e3f-000000000003", TeacherID: "2c8e7cde-ccfe-4c3e-8c6e-bc9efccf5cfe", Title: "Fullstack React", Description: "Build modern web apps with React.", Price: 59.99, Currency: "USD", Status: "published", CreatedAt: seedCoursesCreatedAt, UpdatedAt: seedCoursesCreatedAt},
	{ID: "8f14e45f-ceea-4a3e-8e3f-000000000004", TeacherID: "2c8e7cde-ccfe-4c3e-8c6e-bc9efccf5cfe", Title: "Intro to Python", Description: "Start your journey in Python.", Price: 39.99, Currency: "USD", Status: "published", CreatedAt: seedCoursesCreatedAt, UpdatedAt: seedCoursesCreatedAt},
	{ID: "8f14e45f-ceea-4a3e-8e3f-000000000005", TeacherID: "3d7f8def-ddff-4d4f-8d7f-cdaeffd06d0f", Title: "Data Science with Pandas", Description: "Data analysis and visualization in Python.", Price: 89.99, Currency: "USD", Status: "published", CreatedAt: seedCoursesCreatedAt, UpdatedAt: seedCoursesCreatedAt},
	{ID: "8f14e45f-ceea-4a3e-8e3f-000000000006", TeacherID: "3d7f8def-ddff-4d4f-8d7f-cdaeffd06d0f", Title: "Machine Learning A-Z", Description: "Learn to build ML models.", Price: 99.99, Currency: "USD", Status: "published", CreatedAt: seedCoursesCreatedAt, UpdatedAt: seedCoursesCreatedAt},
	{ID: "8f14e45f-ceea-4a3e-8e3f-000000000007", TeacherID: "4e6f9eff-eeff-4e5f-9e8f-deaf00f17e1f", Title: "Docker for Beginners", Description: "Containerize your applications.", Price: 29.99, Currency: "USD", Status: "published", CreatedAt: seedCoursesCreatedAt, UpdatedAt: seedCoursesCreatedAt},
	{ID: "8f14e45f-ceea-4a3e-8e3f-000000000008", TeacherID: "4e6f9eff-eeff-4e5f-9e8f-deaf00f17e1f", Title: "Kubernetes Mastery", Description: "Deploy and manage containers at scale.", Price: 89.99, Currency: "USD", Status: "published", CreatedAt: seedCoursesCreatedAt, UpdatedAt: seedCoursesCreatedAt},
	{ID: "8f14e45f-ceea-4a3e-8e3f-000000000009", TeacherID: "1b9d6bcd-bbfd-4b2d-9b5d-ab8dfbbd4bed", Title: "Go Web Services", Description: "Build fast HTTP servers in Go.", Price: 49.99, Currency: "USD", Status: "published", CreatedAt: seedCoursesCreatedAt, UpdatedAt: seedCoursesCreatedAt},
	{ID: "8f14e45f-ceea-4a3e-8e3f-000000000010", TeacherID: "2c8e7cde-ccfe-4c3e-8c6e-bc9efccf5cfe", Title: "React Native", Description: "Build mobile apps using React.", Price: 59.99, Currency: "USD", Status: "published", CreatedAt: seedCoursesCreatedAt, UpdatedAt: seedCoursesCreatedAt},
	{ID: "8f14e45f-ceea-4a3e-8e3f-000000000011", TeacherID: "1b9d6bcd-bbfd-4b2d-9b5d-ab8dfbbd4bed", Title: "Go Generics Deep Dive", Description: "Unreleased course on Go generics, still being authored.", Price: 69.99, Currency: "USD", Status: "draft", CreatedAt: seedCoursesCreatedAt, UpdatedAt: seedCoursesCreatedAt},
	{ID: "8f14e45f-ceea-4a3e-8e3f-000000000012", TeacherID: "4e6f9eff-eeff-4e5f-9e8f-deaf00f17e1f", Title: "Legacy Jenkins Pipelines", Description: "Retired course on Jenkins CI/CD pipelines.", Price: 19.99, Currency: "USD", Status: "archived", CreatedAt: seedCoursesCreatedAt, UpdatedAt: seedCoursesCreatedAt},
}

const coursePublished = "published"

type MockCatalogRepository struct{}

func NewMockCatalogRepository() domain.CatalogRepository {
	return &MockCatalogRepository{}
}

// matchesSearch reports whether every word in the search query appears as a
// whole word in the course's title or description (case-insensitive), so a
// search for "go" doesn't match substrings like "algorithm".
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

func (m *MockCatalogRepository) GetCourses(search string, minPrice, maxPrice *float64, page, pageSize int) ([]domain.Course, int, error) {
	var filtered []domain.Course

	for _, c := range seedCourses {
		if c.Status != coursePublished {
			continue
		}
		if search != "" && !matchesSearch(search, c) {
			continue
		}
		if minPrice != nil && c.Price < *minPrice {
			continue
		}
		if maxPrice != nil && c.Price > *maxPrice {
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
