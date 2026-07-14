package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
)

type refreshCall struct {
	actorID, courseID string
}

// spyAnalyticsRepo implements domain.AnalyticsRepository, recording every
// RefreshStudentCourseRow call so tests can assert on what the sink triggered.
type spyAnalyticsRepo struct {
	calls []refreshCall
	err   error
}

func (s *spyAnalyticsRepo) CourseStudentProgress(_ context.Context, _ string) ([]domain.StudentProgress, error) {
	return nil, nil
}

func (s *spyAnalyticsRepo) StudentCourseProgress(_ context.Context, _ string) ([]domain.CourseProgress, error) {
	return nil, nil
}
func (s *spyAnalyticsRepo) RefreshStudentCourseRollup(_ context.Context) error { return nil }

func (s *spyAnalyticsRepo) RefreshStudentCourseRow(_ context.Context, actorID, courseID string) error {
	s.calls = append(s.calls, refreshCall{actorID, courseID})
	return s.err
}

func TestRollupRefreshSink_RefreshesOnProgressSave(t *testing.T) {
	repo := &spyAnalyticsRepo{}
	sink := NewRollupRefreshSink(repo)

	err := sink.Emit(context.Background(), domain.Event{
		EventName: domain.EventPlayerProgressSave,
		Actor:     domain.Actor{ActorID: "student-1"},
		Payload:   map[string]any{"course_id": "course-1"},
	})
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if len(repo.calls) != 1 || repo.calls[0] != (refreshCall{"student-1", "course-1"}) {
		t.Fatalf("calls = %+v, want one call for student-1/course-1", repo.calls)
	}
}

func TestRollupRefreshSink_IgnoresOtherEvents(t *testing.T) {
	repo := &spyAnalyticsRepo{}
	sink := NewRollupRefreshSink(repo)

	names := []string{domain.EventPlayerLessonOpen, domain.EventPlayerLessonComplete, domain.EventAuthLogin, domain.EventAccessGranted}
	for _, name := range names {
		if err := sink.Emit(context.Background(), domain.Event{
			EventName: name,
			Actor:     domain.Actor{ActorID: "student-1"},
			Payload:   map[string]any{"course_id": "course-1"},
		}); err != nil {
			t.Fatalf("Emit(%s): %v", name, err)
		}
	}
	if len(repo.calls) != 0 {
		t.Fatalf("calls = %+v, want none", repo.calls)
	}
}

func TestRollupRefreshSink_SkipsIncompletePayload(t *testing.T) {
	repo := &spyAnalyticsRepo{}
	sink := NewRollupRefreshSink(repo)

	cases := []domain.Event{
		{EventName: domain.EventPlayerProgressSave, Actor: domain.Actor{}, Payload: map[string]any{"course_id": "course-1"}},
		{EventName: domain.EventPlayerProgressSave, Actor: domain.Actor{ActorID: "student-1"}, Payload: map[string]any{}},
		{EventName: domain.EventPlayerProgressSave, Actor: domain.Actor{ActorID: "student-1"}, Payload: nil},
	}
	for _, e := range cases {
		if err := sink.Emit(context.Background(), e); err != nil {
			t.Fatalf("Emit: %v", err)
		}
	}
	if len(repo.calls) != 0 {
		t.Fatalf("calls = %+v, want none (missing actor or course id)", repo.calls)
	}
}

func TestRollupRefreshSink_PropagatesRepositoryError(t *testing.T) {
	repo := &spyAnalyticsRepo{err: errors.New("db down")}
	sink := NewRollupRefreshSink(repo)

	err := sink.Emit(context.Background(), domain.Event{
		EventName: domain.EventPlayerProgressSave,
		Actor:     domain.Actor{ActorID: "student-1"},
		Payload:   map[string]any{"course_id": "course-1"},
	})
	if err == nil {
		t.Fatal("expected error to propagate to caller (AnalyticsRecorder logs and swallows it)")
	}
}
