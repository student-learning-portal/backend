package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/logging"
)

// AnalyticsRecorder builds event envelopes from request-scoped context and fans
// them out to the configured sinks (NDJSON transport + Postgres load). It is the
// single entry point the delivery layer uses to emit analytics events.
//
// Emission is best-effort: a sink error is logged but never returned, so a failed
// analytics write can never break the user action that triggered it — matching
// how the access-check audit is already fire-and-forget in the middleware.
type AnalyticsRecorder struct {
	sinks  []domain.EventSink
	source domain.Source
	now    func() time.Time
}

// NewAnalyticsRecorder wires a recorder to its sinks. source carries the
// per-instance fields (env, instance, release); the per-event service is derived
// from the event name. With no sinks the recorder is a no-op, which keeps tests
// and local runs without a configured stream cheap.
func NewAnalyticsRecorder(source domain.Source, sinks ...domain.EventSink) *AnalyticsRecorder {
	return &AnalyticsRecorder{sinks: sinks, source: source, now: time.Now}
}

// Record builds an envelope for the event and emits it to every sink. Identity is
// taken from the authenticated actor in ctx when present, otherwise the request's
// anonymous visitor id. course_id / lesson_id placed in payload are also promoted
// to the indexed event_log columns by the Postgres sink.
//
// Governance (§4): when the actor has not consented to analytics, only PIINone
// operational events are emitted; anything carrying PII is dropped here.
func (a *AnalyticsRecorder) Record(ctx context.Context, name string, pii domain.PIILevel, payload map[string]any) {
	if len(a.sinks) == 0 {
		return
	}

	lc, _ := domain.LogContextFromContext(ctx)

	if !lc.Consent.Analytics && pii != domain.PIINone {
		return
	}

	if payload == nil {
		payload = map[string]any{}
	}

	now := a.now()
	e := domain.Event{
		SchemaVersion: domain.SchemaVersion,
		EventID:       uuid.NewString(),
		EventName:     name,
		EventTs:       now,
		CorrelationID: lc.CorrelationID,
		SessionID:     lc.SessionID,
		TraceID:       lc.TraceID,
		SpanID:        lc.SpanID,
		Actor:         actorFor(ctx, lc),
		Source:        a.sourceFor(name),
		Context:       lc.Context,
		Payload:       payload,
		PIILevel:      pii,
		Consent:       lc.Consent,
	}

	for _, sink := range a.sinks {
		if err := sink.Emit(ctx, e); err != nil {
			logging.FromContext(ctx).Error("analytics: sink emit failed",
				slog.String("event_name", name),
				slog.String("sink", fmt.Sprintf("%T", sink)),
				slog.Any("error", err),
			)
		}
	}
}

// sourceFor stamps the per-event service onto the instance-level source.
func (a *AnalyticsRecorder) sourceFor(name string) domain.Source {
	s := a.source
	s.Service = domain.ServiceForEvent(name)
	return s
}

// actorFor resolves the event actor: the authenticated user if RequireAuth set
// one, otherwise an anonymous guest carrying the request's stable visitor id.
func actorFor(ctx context.Context, lc domain.LogContext) domain.Actor {
	if actor, ok := domain.ActorFromContext(ctx); ok {
		if actor.AnonymousID == "" {
			actor.AnonymousID = lc.AnonymousID
		}
		return actor
	}
	return domain.Actor{
		AnonymousID: lc.AnonymousID,
		Role:        domain.RoleGuest,
		AuthState:   domain.AuthStateAnonymous,
	}
}
