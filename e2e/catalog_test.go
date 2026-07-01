package e2e

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/student-learning-portal/backend/internal/domain"
)

// course is a minimal view of the catalog's Course JSON for assertions.
type course struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Subject string  `json:"subject"`
	Price   float64 `json:"price"`
	Status  string  `json:"status"`
}

type lessonView struct {
	ID       string `json:"id"`
	CourseID string `json:"course_id"`
	Title    string `json:"title"`
	Type     string `json:"type"`
	Position int    `json:"position"`
}

func TestCatalog_ListsOnlyPublishedCourses(t *testing.T) {
	e := newTestEnv(t)
	teacher, _ := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)

	e.insertCourse(teacher, "Go Basics", "Programming", 49.99, "published")
	e.insertCourse(teacher, "Advanced Go", "Programming", 79.99, "published")
	e.insertCourse(teacher, "Secret Draft", "Programming", 10.00, "draft")
	e.insertCourse(teacher, "Old Course", "Programming", 5.00, "archived")

	resp := e.do(http.MethodGet, "/api/v1/catalog/courses", "", nil)
	e.requireStatus(resp, http.StatusOK)

	var courses []course
	e.decode(resp, &courses)
	if len(courses) != 2 {
		t.Fatalf("got %d courses, want 2 (published only); body=%s", len(courses), resp.body)
	}
	for _, c := range courses {
		if c.Status != "published" {
			t.Errorf("course %q has status %q, want published", c.Title, c.Status)
		}
	}
}

func TestCatalog_SearchSubjectAndPriceFilters(t *testing.T) {
	e := newTestEnv(t)
	teacher, _ := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	e.insertCourse(teacher, "Go Basics", "Programming", 20.00, "published")
	e.insertCourse(teacher, "React Fundamentals", "Web Development", 50.00, "published")
	e.insertCourse(teacher, "Kubernetes", "DevOps", 90.00, "published")

	// search matches title (case-insensitive ILIKE)
	resp := e.do(http.MethodGet, "/api/v1/catalog/courses?search=react", "", nil)
	e.requireStatus(resp, http.StatusOK)
	var bySearch []course
	e.decode(resp, &bySearch)
	if len(bySearch) != 1 || bySearch[0].Title != "React Fundamentals" {
		t.Fatalf("search=react returned %+v, want only React Fundamentals", bySearch)
	}

	// subject filter (partial, case-insensitive)
	resp = e.do(http.MethodGet, "/api/v1/catalog/courses?subject=program", "", nil)
	var bySubject []course
	e.decode(resp, &bySubject)
	if len(bySubject) != 1 || bySubject[0].Subject != "Programming" {
		t.Fatalf("subject=program returned %+v, want only Programming", bySubject)
	}

	// price window
	resp = e.do(http.MethodGet, "/api/v1/catalog/courses?min_price=40&max_price=80", "", nil)
	var byPrice []course
	e.decode(resp, &byPrice)
	if len(byPrice) != 1 || byPrice[0].Title != "React Fundamentals" {
		t.Fatalf("price 40-80 returned %+v, want only React Fundamentals", byPrice)
	}
}

func TestCatalog_SortAndPaginate(t *testing.T) {
	e := newTestEnv(t)
	teacher, _ := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	e.insertCourse(teacher, "Cheap", "X", 10.00, "published")
	e.insertCourse(teacher, "Mid", "X", 50.00, "published")
	e.insertCourse(teacher, "Pricey", "X", 90.00, "published")

	// sort by price descending
	resp := e.do(http.MethodGet, "/api/v1/catalog/courses?sort_by=price&sort_order=desc", "", nil)
	var sorted []course
	e.decode(resp, &sorted)
	if len(sorted) != 3 || sorted[0].Price != 90.00 || sorted[2].Price != 10.00 {
		t.Fatalf("price desc order wrong: %+v", sorted)
	}

	// pagination: page_size=1 returns a single course
	resp = e.do(http.MethodGet, "/api/v1/catalog/courses?page=1&page_size=1", "", nil)
	var page []course
	e.decode(resp, &page)
	if len(page) != 1 {
		t.Fatalf("page_size=1 returned %d courses, want 1", len(page))
	}
}

func TestCatalog_CourseLessonsOrderedByPosition(t *testing.T) {
	e := newTestEnv(t)
	teacher, _ := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	courseID := e.insertCourse(teacher, "Go Basics", "Programming", 49.99, "published")
	// Insert out of order to prove the ORDER BY position.
	e.insertLesson(courseID, "Third", "video", 3)
	e.insertLesson(courseID, "First", "video", 1)
	e.insertLesson(courseID, "Second", "text", 2)

	resp := e.do(http.MethodGet, "/api/v1/catalog/courses/"+courseID+"/lessons", "", nil)
	e.requireStatus(resp, http.StatusOK)
	var lessons []lessonView
	e.decode(resp, &lessons)
	if len(lessons) != 3 {
		t.Fatalf("got %d lessons, want 3", len(lessons))
	}
	wantOrder := []string{"First", "Second", "Third"}
	for i, l := range lessons {
		if l.Title != wantOrder[i] {
			t.Errorf("lesson[%d] = %q, want %q", i, l.Title, wantOrder[i])
		}
	}
}

func TestCatalog_UnknownCourseLessonsReturnsEmptyArray(t *testing.T) {
	e := newTestEnv(t)
	// No course existence check: an unknown (well-formed) id yields 200 [], not 404.
	resp := e.do(http.MethodGet, "/api/v1/catalog/courses/"+uuid.NewString()+"/lessons", "", nil)
	e.requireStatus(resp, http.StatusOK)
	var lessons []lessonView
	e.decode(resp, &lessons)
	if len(lessons) != 0 {
		t.Fatalf("unknown course returned %d lessons, want 0", len(lessons))
	}
}
