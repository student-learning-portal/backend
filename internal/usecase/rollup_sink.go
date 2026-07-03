package usecase

import (
	"context"
	"fmt"

	"github.com/student-learning-portal/backend/internal/domain"
)

// RollupRefreshSink keeps a learner's analytics_student_course row close to
// real-time: it recomputes exactly the (actor, course) pair a progress event
// touched, synchronously, instead of waiting for the periodic batch loader
// (cmd/analytics-loader) to sweep the whole table.
//
// It implements domain.EventSink so it plugs into the same best-effort fan-out
// AnalyticsRecorder already uses for the raw event log: a failure here is
// logged and swallowed by the recorder, never blocking the request that
// triggered it (analytics-ml-layer.md §2). The full loader keeps running as a
// reconciliation pass (e.g. for replayed/backfilled events); this is a
// read-your-own-writes shortcut, not a replacement for it.
type RollupRefreshSink struct {
	analytics domain.AnalyticsRepository
}

func NewRollupRefreshSink(analytics domain.AnalyticsRepository) *RollupRefreshSink {
	return &RollupRefreshSink{analytics: analytics}
}

// Emit recomputes the rollup row for events that change stored progress.
// player.lesson_open is intentionally excluded: it never writes progress, so
// refreshing on it would just be a pointless recompute on every lesson view.
func (s *RollupRefreshSink) Emit(ctx context.Context, e domain.Event) error {
	if e.EventName != domain.EventPlayerProgressSave {
		return nil
	}

	actorID := e.Actor.ActorID
	courseID, _ := e.Payload["course_id"].(string)
	if actorID == "" || courseID == "" {
		return nil
	}

	if err := s.analytics.RefreshStudentCourseRow(ctx, actorID, courseID); err != nil {
		return fmt.Errorf("rollup refresh sink: %w", err)
	}
	return nil
}
