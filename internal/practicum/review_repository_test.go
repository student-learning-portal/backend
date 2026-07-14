package practicum

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
)

// fakeCatalog implements domain.CatalogRepository for mirroring tests. Only
// the methods ReviewRepository actually uses are meaningfully implemented.
type fakeCatalog struct {
	course     domain.Course
	externalID string
	hasMirror  bool
	setCalls   int
}

func (f *fakeCatalog) GetCourses(domain.CourseListParams) ([]domain.Course, int, error) {
	return nil, 0, nil
}

func (f *fakeCatalog) GetByID(context.Context, string) (domain.Course, error) { return f.course, nil }

func (f *fakeCatalog) GetByTeacherID(context.Context, string) ([]domain.Course, error) {
	return nil, nil
}

func (f *fakeCatalog) Create(_ context.Context, c domain.Course) (domain.Course, error) {
	return c, nil
}

func (f *fakeCatalog) Update(_ context.Context, c domain.Course) (domain.Course, error) {
	return c, nil
}
func (f *fakeCatalog) Delete(context.Context, string) error { return nil }

func (f *fakeCatalog) GetExternalCourseID(context.Context, string) (string, bool, error) {
	return f.externalID, f.hasMirror, nil
}

func (f *fakeCatalog) SetExternalCourseID(_ context.Context, _, externalID string) error {
	f.setCalls++
	f.externalID = externalID
	f.hasMirror = true
	return nil
}

func TestEnsureExternalCourse_MirrorsOnlyOnce(t *testing.T) {
	var createCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		createCalls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(courseResponse{ID: "external-42"})
	}))
	defer srv.Close()

	catalog := &fakeCatalog{course: domain.Course{ID: "c1", Title: "Go"}}
	repo := NewReviewRepository(NewClient(srv.URL, "secret"), catalog, "teacher-integration-1")

	id1, err := repo.ensureExternalCourse(context.Background(), "c1")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if id1 != "external-42" {
		t.Errorf("id1 = %q, want external-42", id1)
	}
	if createCalls != 1 {
		t.Errorf("createCalls after first call = %d, want 1", createCalls)
	}

	id2, err := repo.ensureExternalCourse(context.Background(), "c1")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if id2 != "external-42" {
		t.Errorf("id2 = %q, want external-42", id2)
	}
	if createCalls != 1 {
		t.Errorf("createCalls after second call = %d, want still 1 (cached)", createCalls)
	}
	if catalog.setCalls != 1 {
		t.Errorf("SetExternalCourseID calls = %d, want 1", catalog.setCalls)
	}
}

func TestRatingSummary_UsesMirroredID(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(courseRatingResponse{AverageRating: 4, ReviewsCount: 2})
	}))
	defer srv.Close()

	catalog := &fakeCatalog{externalID: "already-mirrored", hasMirror: true}
	repo := NewReviewRepository(NewClient(srv.URL, "secret"), catalog, "teacher-integration-1")

	summary, err := repo.RatingSummary(context.Background(), "c1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.AverageRating != 4 || summary.ReviewsCount != 2 {
		t.Errorf("got %+v", summary)
	}
	if gotPath != "/courses/already-mirrored/rating" {
		t.Errorf("path = %q, want to use the cached external id", gotPath)
	}
}
