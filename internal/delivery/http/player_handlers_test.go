package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

// --- in-memory repos for handler-through-usecase tests ---

type stubLessonRepo struct {
	lesson    domain.Lesson
	lessonErr error
	media     []domain.Media
	materials []domain.Material
}

func (s *stubLessonRepo) GetLesson(_ context.Context, _, _ string) (domain.Lesson, error) {
	return s.lesson, s.lessonErr
}
func (s *stubLessonRepo) GetLessonMedia(_ context.Context, _ string) ([]domain.Media, error) {
	return s.media, nil
}
func (s *stubLessonRepo) GetLessonMaterials(_ context.Context, _ string) ([]domain.Material, error) {
	return s.materials, nil
}

type stubProgressRepo struct {
	store map[string]domain.ProgressState
}

func (s *stubProgressRepo) Save(_ context.Context, p domain.ProgressState) error {
	s.store[p.ActorID+p.LessonID] = p
	return nil
}
func (s *stubProgressRepo) Get(_ context.Context, actor, _, lesson string) (domain.ProgressState, error) {
	p, ok := s.store[actor+lesson]
	if !ok {
		return domain.ProgressState{}, domain.ErrProgressNotFound
	}
	return p, nil
}

// noopRecorder is an AnalyticsRecorder with no sinks; its Record is a no-op, so
// handler tests exercise behaviour without asserting on emitted events.
func noopRecorder() *usecase.AnalyticsRecorder {
	return usecase.NewAnalyticsRecorder(domain.Source{})
}

func newPlayerHandler(lessons domain.LessonRepository, progress domain.ProgressRepository) *PlayerHandler {
	return NewPlayerHandler(usecase.NewPlayerUseCase(lessons, progress), noopRecorder())
}

// withClaimsAndPath builds a request carrying auth claims and the player path values,
// mimicking what RequireAuth + ServeMux supply at runtime.
func withClaimsAndPath(method, body, courseID, lessonID string) *http.Request {
	r := httptest.NewRequest(method, "http://x/", strings.NewReader(body))
	r = r.WithContext(context.WithValue(r.Context(), claimsContextKey, domain.Claims{UserID: "user-1"}))
	r.SetPathValue("course_id", courseID)
	r.SetPathValue("lesson_id", lessonID)
	return r
}

func TestGetLesson_ReturnsContentAndResumePoint(t *testing.T) {
	lessons := &stubLessonRepo{
		lesson: domain.Lesson{ID: "lesson-1", CourseID: "course-1", Title: "Intro", Type: "video", Position: 1},
		media:  []domain.Media{{URL: "https://cdn/x.mp4", DurationMs: 100_000, Type: "video"}},
	}
	progress := &stubProgressRepo{store: map[string]domain.ProgressState{
		"user-1lesson-1": {ActorID: "user-1", CourseID: "course-1", LessonID: "lesson-1", PositionMs: 30_000, PercentComplete: 30},
	}}
	h := newPlayerHandler(lessons, progress)

	w := httptest.NewRecorder()
	h.GetLesson(w, withClaimsAndPath(http.MethodGet, "", "course-1", "lesson-1"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp lessonDataResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ContentURL != "https://cdn/x.mp4" {
		t.Errorf("content_url = %q", resp.ContentURL)
	}
	if resp.LastProgressSeconds != 30 {
		t.Errorf("last_progress_seconds = %d, want 30", resp.LastProgressSeconds)
	}
}

func TestGetLesson_NotFound(t *testing.T) {
	h := newPlayerHandler(&stubLessonRepo{lessonErr: domain.ErrLessonNotFound}, &stubProgressRepo{store: map[string]domain.ProgressState{}})
	w := httptest.NewRecorder()
	h.GetLesson(w, withClaimsAndPath(http.MethodGet, "", "course-1", "missing"))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestSaveProgress_PersistsAndEchoes(t *testing.T) {
	lessons := &stubLessonRepo{
		lesson: domain.Lesson{ID: "lesson-1", CourseID: "course-1"},
		media:  []domain.Media{{DurationMs: 60_000}},
	}
	progress := &stubProgressRepo{store: map[string]domain.ProgressState{}}
	h := newPlayerHandler(lessons, progress)

	w := httptest.NewRecorder()
	h.SaveProgress(w, withClaimsAndPath(http.MethodPost, `{"progress_seconds":30,"completed":false}`, "course-1", "lesson-1"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp progressResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ProgressSeconds != 30 {
		t.Errorf("progress_seconds = %d, want 30", resp.ProgressSeconds)
	}
	if resp.PercentComplete != 50 {
		t.Errorf("percent_complete = %v, want 50", resp.PercentComplete)
	}
	if _, ok := progress.store["user-1lesson-1"]; !ok {
		t.Errorf("progress was not persisted")
	}
}

func TestSaveProgress_MissingFieldRejected(t *testing.T) {
	h := newPlayerHandler(&stubLessonRepo{lesson: domain.Lesson{ID: "lesson-1"}}, &stubProgressRepo{store: map[string]domain.ProgressState{}})
	w := httptest.NewRecorder()
	h.SaveProgress(w, withClaimsAndPath(http.MethodPost, `{"completed":true}`, "course-1", "lesson-1"))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestSaveProgress_NegativeRejected(t *testing.T) {
	h := newPlayerHandler(&stubLessonRepo{lesson: domain.Lesson{ID: "lesson-1"}}, &stubProgressRepo{store: map[string]domain.ProgressState{}})
	w := httptest.NewRecorder()
	h.SaveProgress(w, withClaimsAndPath(http.MethodPost, `{"progress_seconds":-5}`, "course-1", "lesson-1"))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestGetProgress_NotFoundReturns404(t *testing.T) {
	h := newPlayerHandler(&stubLessonRepo{lesson: domain.Lesson{ID: "lesson-1"}}, &stubProgressRepo{store: map[string]domain.ProgressState{}})
	w := httptest.NewRecorder()
	h.GetProgress(w, withClaimsAndPath(http.MethodGet, "", "course-1", "lesson-1"))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}
