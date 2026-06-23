package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

// captureSink records emitted events so handler tests can assert on the stream.
type captureSink struct{ events []domain.Event }

func (c *captureSink) Emit(_ context.Context, e domain.Event) error {
	c.events = append(c.events, e)
	return nil
}

func (c *captureSink) names() []string {
	out := make([]string, 0, len(c.events))
	for _, e := range c.events {
		out = append(out, e.EventName)
	}
	return out
}

func contains(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

func TestSaveProgress_EmitsProgressAndCompleteEvents(t *testing.T) {
	lessons := &stubLessonRepo{
		lesson: domain.Lesson{ID: "lesson-1", CourseID: "course-1"},
		media:  []domain.Media{{DurationMs: 60_000}},
	}
	sink := &captureSink{}
	recorder := usecase.NewAnalyticsRecorder(domain.Source{Env: "test"}, sink)
	h := NewPlayerHandler(usecase.NewPlayerUseCase(lessons, &stubProgressRepo{store: map[string]domain.ProgressState{}}), recorder)

	r := withClaimsAndPath(http.MethodPost, `{"progress_seconds":60,"completed":true}`, "course-1", "lesson-1")
	r = r.WithContext(domain.ContextWithLogContext(r.Context(), domain.LogContext{Consent: domain.Consent{Analytics: true}}))

	h.SaveProgress(httptest.NewRecorder(), r)

	names := sink.names()
	if !contains(names, domain.EventPlayerProgressSave) {
		t.Errorf("expected %s in emitted events, got %v", domain.EventPlayerProgressSave, names)
	}
	if !contains(names, domain.EventPlayerLessonComplete) {
		t.Errorf("expected %s in emitted events, got %v", domain.EventPlayerLessonComplete, names)
	}
}

func TestSaveProgress_NotCompletedSkipsCompleteEvent(t *testing.T) {
	lessons := &stubLessonRepo{
		lesson: domain.Lesson{ID: "lesson-1", CourseID: "course-1"},
		media:  []domain.Media{{DurationMs: 60_000}},
	}
	sink := &captureSink{}
	recorder := usecase.NewAnalyticsRecorder(domain.Source{Env: "test"}, sink)
	h := NewPlayerHandler(usecase.NewPlayerUseCase(lessons, &stubProgressRepo{store: map[string]domain.ProgressState{}}), recorder)

	r := withClaimsAndPath(http.MethodPost, `{"progress_seconds":30,"completed":false}`, "course-1", "lesson-1")
	r = r.WithContext(domain.ContextWithLogContext(r.Context(), domain.LogContext{Consent: domain.Consent{Analytics: true}}))

	h.SaveProgress(httptest.NewRecorder(), r)

	if contains(sink.names(), domain.EventPlayerLessonComplete) {
		t.Errorf("lesson_complete must not be emitted when completed=false, got %v", sink.names())
	}
}
