package practicum

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ListCourses_NoAuthPublic(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/courses" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(courseListResponse{Courses: []remoteCourse{
			{ID: "c1", Name: "Go Basics", Subject: "programming", Duration: 60, Price: 500, Difficulty: "beginner"},
		}})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "shared-secret")
	got, err := c.listCourses(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("expected no Authorization header, got %q", gotAuth)
	}
	if len(got) != 1 || got[0].ID != "c1" || got[0].Name != "Go Basics" {
		t.Errorf("got %+v", got)
	}
}

func TestClient_ListLessons_NoAuthPublic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/courses/c1/lessons" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(lessonListResponse{Lessons: []remoteLesson{
			{ID: "l1", Name: "Intro"},
		}})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "shared-secret")
	got, err := c.listLessons(context.Background(), "c1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "l1" || got[0].Name != "Intro" {
		t.Errorf("got %+v", got)
	}
}

func TestClient_ListLessonFiles_SendsStudentToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/lessons/l1/files" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fileListResponse{Files: []remoteFile{
			{ID: "f1", OriginalFilename: "video.mp4", MimeType: "video/mp4"},
		}})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "shared-secret")
	got, err := c.listLessonFiles(context.Background(), "l1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth == "" || gotAuth == "Bearer " {
		t.Errorf("expected a Bearer token, got %q", gotAuth)
	}
	if len(got) != 1 || got[0].ID != "f1" || got[0].MimeType != "video/mp4" {
		t.Errorf("got %+v", got)
	}
}

func TestClient_DownloadFile_ReturnsRawBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/files/f1/download" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth == "" {
			t.Error("expected a Bearer token")
		}
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("fake-video-bytes"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "shared-secret")
	got, err := c.downloadFile(context.Background(), "f1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "fake-video-bytes" {
		t.Errorf("got %q", got)
	}
}

func TestClient_DownloadFile_MapsErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(errorResponse{Error: errorDetail{Code: "COURSE_NOT_FOUND", Message: "nope"}})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "shared-secret")
	_, err := c.downloadFile(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected a non-nil error")
	}
}

func TestImportRepository_ListRemoteCourses_MapsFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(courseListResponse{Courses: []remoteCourse{
			{
				ID: "c1", Name: "Go Basics", Subject: "programming", Description: "desc",
				Duration: 90, Price: 700, Difficulty: "advanced",
			},
		}})
	}))
	defer srv.Close()

	repo := NewImportRepository(NewClient(srv.URL, "shared-secret"))
	got, err := repo.ListRemoteCourses(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d courses, want 1", len(got))
	}
	c := got[0]
	if c.ID != "c1" || c.Title != "Go Basics" || c.Subject != "programming" || c.Description != "desc" ||
		c.Price != 700 || c.DurationMinutes != 90 || c.Difficulty != "advanced" {
		t.Errorf("got %+v", c)
	}
}
