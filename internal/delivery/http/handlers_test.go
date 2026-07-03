package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

// catalogHandlerRepo implements domain.CatalogRepository for catalog handler tests.
type catalogHandlerRepo struct {
	courses    []domain.Course
	coursesErr error
}

func (c *catalogHandlerRepo) GetCourses(_ domain.CourseListParams) ([]domain.Course, int, error) {
	return c.courses, len(c.courses), c.coursesErr
}

func (c *catalogHandlerRepo) GetByID(_ context.Context, _ string) (domain.Course, error) {
	return domain.Course{}, nil
}

func (c *catalogHandlerRepo) GetByTeacherID(_ context.Context, _ string) ([]domain.Course, error) {
	return c.courses, c.coursesErr
}

func (c *catalogHandlerRepo) Create(_ context.Context, course domain.Course) (domain.Course, error) {
	return course, nil
}

func (c *catalogHandlerRepo) Update(_ context.Context, course domain.Course) (domain.Course, error) {
	return course, nil
}

func (c *catalogHandlerRepo) Delete(_ context.Context, _ string) error { return nil }

// --- HelloHandler ---

func TestHelloHandler_ReturnsHelloWorld(t *testing.T) {
	w := httptest.NewRecorder()
	HelloHandler(w, httptest.NewRequest(http.MethodGet, "http://x/hello", nil))

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp HelloResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Message != "Hello, World!" {
		t.Errorf("message = %q, want Hello, World!", resp.Message)
	}
}

func TestHelloHandler_ContentType(t *testing.T) {
	w := httptest.NewRecorder()
	HelloHandler(w, httptest.NewRequest(http.MethodGet, "http://x/hello", nil))
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

// --- CatalogHandler ---

func TestGetCourses_ReturnsCourses(t *testing.T) {
	repo := &catalogHandlerRepo{
		courses: []domain.Course{
			{ID: "c1", Title: "Math", Subject: "math"},
			{ID: "c2", Title: "Science", Subject: "science"},
		},
	}
	h := NewCatalogHandler(usecase.NewCatalogUseCase(repo, &stubLessonRepo{}))

	w := httptest.NewRecorder()
	h.GetCourses(w, httptest.NewRequest(http.MethodGet, "http://x/catalog/courses", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var courses []domain.Course
	if err := json.Unmarshal(w.Body.Bytes(), &courses); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(courses) != 2 {
		t.Errorf("len = %d, want 2", len(courses))
	}
}

func TestGetCourses_EmptyOnRepoError(t *testing.T) {
	repo := &catalogHandlerRepo{coursesErr: domain.ErrCourseNotFound}
	h := NewCatalogHandler(usecase.NewCatalogUseCase(repo, &stubLessonRepo{}))

	w := httptest.NewRecorder()
	h.GetCourses(w, httptest.NewRequest(http.MethodGet, "http://x/catalog/courses", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var courses []domain.Course
	if err := json.Unmarshal(w.Body.Bytes(), &courses); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(courses) != 0 {
		t.Errorf("expected empty slice on error, got %d items", len(courses))
	}
}

func TestGetCourses_QueryParamsParsed(t *testing.T) {
	dest := &domain.CourseListParams{}
	h := NewCatalogHandler(usecase.NewCatalogUseCase(&paramCaptureCatalogRepo{dest: dest}, &stubLessonRepo{}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "http://x/catalog?search=go&subject=programming&page=2&page_size=5&min_price=10&max_price=100", nil)
	h.GetCourses(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if dest.Search != "go" {
		t.Errorf("search = %q, want go", dest.Search)
	}
	if dest.Subject != "programming" {
		t.Errorf("subject = %q, want programming", dest.Subject)
	}
	if dest.Page != 2 {
		t.Errorf("page = %d, want 2", dest.Page)
	}
	if dest.PageSize != 5 {
		t.Errorf("page_size = %d, want 5", dest.PageSize)
	}
	if dest.MinPrice == nil || *dest.MinPrice != 10 {
		t.Errorf("min_price = %v, want 10", dest.MinPrice)
	}
	if dest.MaxPrice == nil || *dest.MaxPrice != 100 {
		t.Errorf("max_price = %v, want 100", dest.MaxPrice)
	}
}

func TestGetCourses_DefaultPagination(t *testing.T) {
	dest := &domain.CourseListParams{}
	h := NewCatalogHandler(usecase.NewCatalogUseCase(&paramCaptureCatalogRepo{dest: dest}, &stubLessonRepo{}))

	w := httptest.NewRecorder()
	h.GetCourses(w, httptest.NewRequest(http.MethodGet, "http://x/catalog", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if dest.Page != 1 {
		t.Errorf("default page = %d, want 1", dest.Page)
	}
	if dest.PageSize != 10 {
		t.Errorf("default page_size = %d, want 10", dest.PageSize)
	}
}

// paramCaptureCatalogRepo captures the params passed to GetCourses.
type paramCaptureCatalogRepo struct {
	dest *domain.CourseListParams
}

func (p *paramCaptureCatalogRepo) GetCourses(params domain.CourseListParams) ([]domain.Course, int, error) {
	*p.dest = params
	return nil, 0, nil
}

func (p *paramCaptureCatalogRepo) GetByID(_ context.Context, _ string) (domain.Course, error) {
	return domain.Course{}, nil
}

func (p *paramCaptureCatalogRepo) GetByTeacherID(_ context.Context, _ string) ([]domain.Course, error) {
	return nil, nil
}

func (p *paramCaptureCatalogRepo) Create(_ context.Context, c domain.Course) (domain.Course, error) {
	return c, nil
}

func (p *paramCaptureCatalogRepo) Update(_ context.Context, c domain.Course) (domain.Course, error) {
	return c, nil
}

func (p *paramCaptureCatalogRepo) Delete(_ context.Context, _ string) error { return nil }
