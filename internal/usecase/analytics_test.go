package usecase

import (
	"context"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
)

// captureSink records every emitted event for assertions.
type captureSink struct{ events []domain.Event }

func (c *captureSink) Emit(_ context.Context, e domain.Event) error {
	c.events = append(c.events, e)
	return nil
}

func ctxWithLog(lc domain.LogContext) context.Context {
	return domain.ContextWithLogContext(context.Background(), lc)
}

func TestRecord_BuildsEnvelopeWithDerivedServiceAndActor(t *testing.T) {
	sink := &captureSink{}
	rec := NewAnalyticsRecorder(domain.Source{Env: "dev", Instance: "i-1", Release: "abc123"}, sink)

	lc := domain.LogContext{CorrelationID: "corr-1", AnonymousID: "anon-1", Consent: domain.Consent{Analytics: true}}
	ctx := domain.ContextWithActor(ctxWithLog(lc),
		domain.Actor{ActorID: "u1", Role: domain.RoleStudent, AuthState: domain.AuthStateAuthenticated})

	rec.Record(ctx, domain.EventPlayerProgressSave, domain.PIINone, map[string]any{
		"course_id": "course-1", "lesson_id": "lesson-1",
	})

	if len(sink.events) != 1 {
		t.Fatalf("events = %d, want 1", len(sink.events))
	}
	e := sink.events[0]
	if e.SchemaVersion != domain.SchemaVersion {
		t.Errorf("schema_version = %q", e.SchemaVersion)
	}
	if e.EventID == "" {
		t.Error("event_id was not generated")
	}
	if e.Source.Service != domain.ServicePlayer {
		t.Errorf("service = %q, want player (derived from event name)", e.Source.Service)
	}
	if e.Source.Env != "dev" || e.Source.Release != "abc123" {
		t.Errorf("source instance fields not carried: %+v", e.Source)
	}
	if e.Actor.ActorID != "u1" || e.Actor.AuthState != domain.AuthStateAuthenticated {
		t.Errorf("actor = %+v", e.Actor)
	}
	if e.CorrelationID != "corr-1" {
		t.Errorf("correlation_id = %q", e.CorrelationID)
	}
}

func TestRecord_AnonymousActorFallback(t *testing.T) {
	sink := &captureSink{}
	rec := NewAnalyticsRecorder(domain.Source{}, sink)

	ctx := ctxWithLog(domain.LogContext{AnonymousID: "anon-9", Consent: domain.Consent{Analytics: true}})
	rec.Record(ctx, domain.EventAuthLogin, domain.PIINone, nil)

	e := sink.events[0]
	if e.Actor.AuthState != domain.AuthStateAnonymous {
		t.Errorf("auth_state = %q, want anonymous", e.Actor.AuthState)
	}
	if e.Actor.Role != domain.RoleGuest {
		t.Errorf("role = %q, want guest", e.Actor.Role)
	}
	if e.Actor.AnonymousID != "anon-9" {
		t.Errorf("anonymous_id = %q", e.Actor.AnonymousID)
	}
}

func TestRecord_ConsentGateDropsPIIButKeepsOperational(t *testing.T) {
	sink := &captureSink{}
	rec := NewAnalyticsRecorder(domain.Source{}, sink)

	ctx := ctxWithLog(domain.LogContext{Consent: domain.Consent{Analytics: false}})
	rec.Record(ctx, "catalog.search", domain.PIILow, nil)        // dropped: no consent + PII
	rec.Record(ctx, domain.EventAccessCheck, domain.PIINone, nil) // kept: operational

	if len(sink.events) != 1 {
		t.Fatalf("events = %d, want 1 (PII event must be gated)", len(sink.events))
	}
	if sink.events[0].EventName != domain.EventAccessCheck {
		t.Errorf("kept event = %q, want access.check", sink.events[0].EventName)
	}
}

func TestRecord_NoSinksIsNoop(t *testing.T) {
	rec := NewAnalyticsRecorder(domain.Source{})
	// Must not panic with no sinks configured.
	rec.Record(context.Background(), domain.EventAuthLogin, domain.PIINone, nil)
}
