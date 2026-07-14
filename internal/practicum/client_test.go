package practicum

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
)

func TestMapError_KnownCodes(t *testing.T) {
	cases := []struct {
		code string
		want error
	}{
		{"COURSE_NOT_FOUND", domain.ErrCourseNotFound},
		{"INVALID_COURSE_ID", domain.ErrCourseNotFound},
		{"TEACHER_NOT_FOUND", domain.ErrCourseNotFound},
		{"NOT_ENROLLED", domain.ErrNotEnrolled},
		{"INSUFFICIENT_PROGRESS", domain.ErrInsufficientProgress},
		{"COMMENT_ALREADY_EXISTS", domain.ErrReviewAlreadyExists},
		{"INVALID_RATING", domain.ErrInvalidReview},
		{"COMMENT_TEXT_TOO_LONG", domain.ErrInvalidReview},
		{"BAD_REQUEST", domain.ErrInvalidReview},
	}
	for _, c := range cases {
		body, _ := json.Marshal(errorResponse{Error: errorDetail{Code: c.code, Message: "x"}})
		got := mapError(http.StatusBadRequest, body)
		if !errors.Is(got, c.want) {
			t.Errorf("mapError(%q) = %v, want %v", c.code, got, c.want)
		}
	}
}

func TestMapError_UnknownCode(t *testing.T) {
	body, _ := json.Marshal(errorResponse{Error: errorDetail{Code: "SOMETHING_NEW", Message: "boom"}})
	err := mapError(http.StatusInternalServerError, body)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
}

func TestMapError_UnparseableBody(t *testing.T) {
	err := mapError(http.StatusInternalServerError, []byte("not json"))
	if err == nil {
		t.Fatal("expected non-nil error")
	}
}

func TestClient_DoSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/courses/c1/rating" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("expected no Authorization header, got %q", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(courseRatingResponse{AverageRating: 4.5, ReviewsCount: 3})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "shared-secret")
	got, err := c.getCourseRating(context.Background(), "c1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AverageRating != 4.5 || got.ReviewsCount != 3 {
		t.Errorf("got %+v", got)
	}
}

func TestClient_DoMapsErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(errorResponse{Error: errorDetail{Code: "NOT_ENROLLED", Message: "nope"}})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "shared-secret")
	_, err := c.createComment(context.Background(), "student-1", "c1", 5, "great")
	if !errors.Is(err, domain.ErrNotEnrolled) {
		t.Errorf("err = %v, want ErrNotEnrolled", err)
	}
}

func TestClient_CreateCourse_ForwardsAuthAndBody(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(courseResponse{ID: "external-1"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "shared-secret")
	id, err := c.createCourse(context.Background(), "teacher-1", domain.Course{
		Title: "Go Basics", Subject: "programming", Difficulty: domain.DifficultyBeginner, DurationMinutes: 60,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "external-1" {
		t.Errorf("id = %q, want external-1", id)
	}
	if gotAuth == "" || gotAuth == "Bearer " {
		t.Errorf("expected a Bearer token, got %q", gotAuth)
	}
	if gotBody == "" {
		t.Error("expected a request body")
	}
}

func TestClient_CreateCourse_MissingIntegrationTeacher(t *testing.T) {
	c := NewClient("http://example.invalid", "shared-secret")
	_, err := c.createCourse(context.Background(), "", domain.Course{Title: "x"})
	if !errors.Is(err, errMissingIntegrationTeacher) {
		t.Errorf("err = %v, want errMissingIntegrationTeacher", err)
	}
}
