package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/student-learning-portal/backend/internal/domain"
)

type PostgresEventSink struct {
	db *sql.DB
}

// NewPostgresEventSink loads behavioral events into the event_log table (§5.2):
// the envelope's hot-filtered fields land in typed columns and the domain payload
// stays in the JSONB column. ingest_ts is left to the column default so it
// reflects server-side load time, never the client clock (§4 ordering).
func NewPostgresEventSink(db *sql.DB) domain.EventSink {
	return &PostgresEventSink{db: db}
}

// Emit inserts one event. The insert is idempotent on the event_id primary key
// (§4 dedupe): a replayed event is silently ignored rather than erroring.
func (s *PostgresEventSink) Emit(ctx context.Context, e domain.Event) error {
	payload, err := json.Marshal(e.Payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	// course_id / lesson_id are promoted to indexed columns; read them off the
	// payload so callers only ever populate the payload map once.
	courseID := payloadString(e.Payload, "course_id")
	lessonID := payloadString(e.Payload, "lesson_id")

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO event_log
		   (event_id, event_name, schema_version, event_ts, correlation_id,
		    session_id, actor_id, anonymous_id, role, service, env,
		    course_id, lesson_id, payload, pii_level)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		 ON CONFLICT (event_id) DO NOTHING`,
		e.EventID,
		e.EventName,
		e.SchemaVersion,
		e.EventTs,
		nullableUUID(e.CorrelationID),
		nullableUUID(e.SessionID),
		nullableText(e.Actor.ActorID),
		nullableUUID(e.Actor.AnonymousID),
		string(e.Actor.Role),
		string(e.Source.Service),
		e.Source.Env,
		nullableText(courseID),
		nullableText(lessonID),
		string(payload),
		string(e.PIILevel),
	)
	if err != nil {
		return fmt.Errorf("insert event_log: %w", err)
	}
	return nil
}

// nullableUUID returns the string for a valid UUID column value, or nil so the
// column is stored as NULL. Non-UUID identifiers (e.g. a client-supplied session
// id) are dropped here rather than failing the insert; the raw value is still
// preserved in the NDJSON envelope.
func nullableUUID(s string) any {
	if _, err := uuid.Parse(s); err != nil {
		return nil
	}
	return s
}

// nullableText returns nil for an empty string so the column is stored as NULL
// (and the role/decision CHECK constraints are not tripped by an empty value).
func nullableText(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func payloadString(payload map[string]any, key string) string {
	if v, ok := payload[key].(string); ok {
		return v
	}
	return ""
}
