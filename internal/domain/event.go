package domain

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// SchemaVersion is the semver of the event envelope (logging-architecture.md §1.1).
// Breaking changes to the envelope require a major bump.
const SchemaVersion = "1.0.0"

// tsLayout renders ISO 8601, UTC, millisecond precision: 2026-06-12T14:03:55.812Z
// (logging-architecture.md §1.1). For a UTC time the Z07:00 zone prints as "Z".
const tsLayout = "2006-01-02T15:04:05.000Z07:00"

// Event names follow the domain.action / snake_case taxonomy (§3).
const (
	EventAuthSignup = "auth.signup"
	EventAuthLogin  = "auth.login"
	EventAuthLogout = "auth.logout"

	EventAccessCheckoutStart    = "access.checkout_start"
	EventAccessPaymentSucceeded = "access.payment_succeeded"
	EventAccessPaymentFailed    = "access.payment_failed"
	EventAccessGranted          = "access.granted"
	EventAccessRevoked          = "access.revoked"
	EventAccessRefundCompleted  = "access.refund_completed"
	EventAccessCheck            = "access.check"
	EventAccessDenied           = "access.denied"

	EventPlayerLessonOpen     = "player.lesson_open"
	EventPlayerProgressSave   = "player.progress_save"
	EventPlayerLessonComplete = "player.lesson_complete"

	// admin.* records the moderation decisions an administrator takes on the
	// teacher approval queue. ServiceForEvent leaves them on the gateway
	// surface, the same as auth.*.
	EventAdminTeacherApproved = "admin.teacher_approved"
	EventAdminTeacherRejected = "admin.teacher_rejected"
)

// Service identifies the emitting surface (§2 source.service). The set is fixed
// by the event_log.service CHECK constraint.
type Service string

const (
	ServiceCatalog   Service = "catalog"
	ServiceAccess    Service = "access"
	ServicePlayer    Service = "player"
	ServiceAnalytics Service = "analytics"
	ServiceGateway   Service = "gateway"
)

// ServiceForEvent maps an event name to its owning service by domain prefix.
// auth.* and session.* are gateway concerns; report.* is the analytics surface.
func ServiceForEvent(name string) Service {
	switch {
	case strings.HasPrefix(name, "catalog."):
		return ServiceCatalog
	case strings.HasPrefix(name, "access."):
		return ServiceAccess
	case strings.HasPrefix(name, "player."), strings.HasPrefix(name, "assessment."):
		return ServicePlayer
	case strings.HasPrefix(name, "report."):
		return ServiceAnalytics
	default:
		return ServiceGateway
	}
}

// PIILevel tags the privacy sensitivity of an event's payload (§4). Consent
// gating keys off this: non-consenting actors get only PIINone operational logs.
type PIILevel string

const (
	PIINone PIILevel = "none"
	PIILow  PIILevel = "low"
	PIIHigh PIILevel = "high"
)

// Roles as recorded in the envelope actor (§2). RoleGuest/RoleSystem extend the
// account roles in user.go for the logging actor model.
const (
	RoleGuest  Role = "guest"
	RoleSystem Role = "system"
)

// Auth states for the envelope actor (§2).
const (
	AuthStateAuthenticated = "authenticated"
	AuthStateAnonymous     = "anonymous"
)

// Actor is the identity that triggered an event (§2). ActorID is empty for
// anonymous traffic, in which case AnonymousID carries the stable visitor id.
type Actor struct {
	ActorID     string `json:"actor_id,omitempty"`
	AnonymousID string `json:"anonymous_id,omitempty"`
	Role        Role   `json:"role"`
	AuthState   string `json:"auth_state"`
}

// Source describes the service instance that produced the event (§2).
type Source struct {
	Service  Service `json:"service"`
	Env      string  `json:"env"`
	Instance string  `json:"instance"`
	Release  string  `json:"release"`
}

// EventContext is the request-derived context block (§2). PII is minimised here:
// IPTrunc is the caller IP with its host bits zeroed (§4).
type EventContext struct {
	IPTrunc    string `json:"ip_trunc,omitempty"`
	UserAgent  string `json:"user_agent,omitempty"`
	DeviceType string `json:"device_type,omitempty"`
	Locale     string `json:"locale,omitempty"`
	Referrer   string `json:"referrer,omitempty"`
	PageURL    string `json:"page_url,omitempty"`
}

// Consent records the actor's logging consent (§2 / §4).
type Consent struct {
	Analytics bool `json:"analytics"`
	Marketing bool `json:"marketing"`
}

// Event is the full envelope every event conforms to (§2). The domain-specific
// data lives under Payload; the surrounding fields are the common envelope.
type Event struct {
	SchemaVersion string
	EventID       string
	EventName     string
	EventTs       time.Time
	IngestTs      time.Time
	CorrelationID string
	SessionID     string
	TraceID       string
	SpanID        string
	Actor         Actor
	Source        Source
	Context       EventContext
	Payload       map[string]any
	PIILevel      PIILevel
	Consent       Consent
}

// MarshalJSON renders the envelope as the NDJSON shape in §2: snake_case keys,
// nested actor/source/context/consent objects, and ISO 8601 millisecond
// timestamps. Timestamps are kept as time.Time on the struct for the SQL sink
// and formatted only here for the raw-transport representation.
func (e Event) MarshalJSON() ([]byte, error) {
	type envelope struct {
		SchemaVersion string         `json:"schema_version"`
		EventID       string         `json:"event_id"`
		EventName     string         `json:"event_name"`
		EventTs       string         `json:"event_ts"`
		IngestTs      string         `json:"ingest_ts"`
		CorrelationID string         `json:"correlation_id,omitempty"`
		SessionID     string         `json:"session_id,omitempty"`
		TraceID       string         `json:"trace_id,omitempty"`
		SpanID        string         `json:"span_id,omitempty"`
		Actor         Actor          `json:"actor"`
		Source        Source         `json:"source"`
		Context       EventContext   `json:"context"`
		Payload       map[string]any `json:"payload"`
		PIILevel      PIILevel       `json:"pii_level"`
		Consent       Consent        `json:"consent"`
	}
	payload := e.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	return json.Marshal(envelope{
		SchemaVersion: e.SchemaVersion,
		EventID:       e.EventID,
		EventName:     e.EventName,
		EventTs:       e.EventTs.UTC().Format(tsLayout),
		IngestTs:      e.IngestTs.UTC().Format(tsLayout),
		CorrelationID: e.CorrelationID,
		SessionID:     e.SessionID,
		TraceID:       e.TraceID,
		SpanID:        e.SpanID,
		Actor:         e.Actor,
		Source:        e.Source,
		Context:       e.Context,
		Payload:       payload,
		PIILevel:      e.PIILevel,
		Consent:       e.Consent,
	})
}

// EventSink is the output port for the event stream. Implementations are the raw
// NDJSON transport and the Postgres event_log load (§5.1). Emit must be
// best-effort from the caller's perspective: the recorder logs and swallows
// errors so analytics can never fail a user action.
type EventSink interface {
	Emit(ctx context.Context, e Event) error
}
