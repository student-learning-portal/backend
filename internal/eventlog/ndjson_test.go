package eventlog

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/student-learning-portal/backend/internal/domain"
)

// fixedEventTs is 2023-11-14T22:13:20.812Z — a stable instant with millisecond
// precision so the formatted timestamp assertion is deterministic.
var fixedEventTs = time.Unix(1_700_000_000, 812_000_000).UTC()

func sampleEvent() domain.Event {
	return domain.Event{
		SchemaVersion: domain.SchemaVersion,
		EventID:       "evt-1",
		EventName:     domain.EventAuthLogin,
		EventTs:       fixedEventTs,
		Actor:         domain.Actor{ActorID: "u1", Role: domain.RoleStudent, AuthState: domain.AuthStateAuthenticated},
		Source:        domain.Source{Service: domain.ServiceGateway, Env: "dev"},
		Payload:       map[string]any{"method": "password"},
		PIILevel:      domain.PIINone,
		Consent:       domain.Consent{Analytics: true},
	}
}

func TestNDJSONSink_WritesOneEnvelopeLine(t *testing.T) {
	var buf strings.Builder
	sink := NewNDJSONSink(&buf)

	if err := sink.Emit(context.Background(), sampleEvent()); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	out := buf.String()
	if strings.Count(out, "\n") != 1 || !strings.HasSuffix(out, "\n") {
		t.Fatalf("want exactly one trailing newline, got %q", out)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &decoded); err != nil {
		t.Fatalf("line is not valid JSON: %v", err)
	}

	// Timestamp must be ISO 8601, UTC, millisecond precision (§1.1).
	if decoded["event_ts"] != "2023-11-14T22:13:20.812Z" {
		t.Errorf("event_ts = %v, want 2023-11-14T22:13:20.812Z", decoded["event_ts"])
	}
	if decoded["event_name"] != "auth.login" {
		t.Errorf("event_name = %v", decoded["event_name"])
	}
	if decoded["schema_version"] != domain.SchemaVersion {
		t.Errorf("schema_version = %v", decoded["schema_version"])
	}

	actor, ok := decoded["actor"].(map[string]any)
	if !ok || actor["actor_id"] != "u1" {
		t.Errorf("actor block not rendered as expected: %v", decoded["actor"])
	}
}

func TestNDJSONSink_StampsIngestWhenZero(t *testing.T) {
	var buf strings.Builder
	sink := NewNDJSONSink(&buf)

	if err := sink.Emit(context.Background(), sampleEvent()); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	var decoded map[string]any
	_ = json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &decoded)
	ingest, _ := decoded["ingest_ts"].(string)
	if ingest == "" || ingest == "0001-01-01T00:00:00.000Z" {
		t.Errorf("ingest_ts was not stamped at write time: %q", ingest)
	}
}
