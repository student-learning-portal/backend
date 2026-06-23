package domain

import "context"

// LogContext is the request-scoped logging data captured once per request by the
// WithLogContext middleware and read by the AnalyticsRecorder when building an
// envelope. It holds everything in §2 that comes from the transport (correlation,
// session, tracing, consent, and the context block) but not the actor, which is
// resolved separately once authentication has run.
type LogContext struct {
	CorrelationID string
	SessionID     string
	TraceID       string
	SpanID        string
	AnonymousID   string
	Consent       Consent
	Context       EventContext
}

type logCtxKey int

const (
	logContextKey logCtxKey = iota
	actorKey
)

// ContextWithLogContext stores the request-scoped LogContext in ctx.
func ContextWithLogContext(ctx context.Context, lc LogContext) context.Context {
	return context.WithValue(ctx, logContextKey, lc)
}

// LogContextFromContext returns the LogContext stored by WithLogContext, if any.
func LogContextFromContext(ctx context.Context) (LogContext, bool) {
	lc, ok := ctx.Value(logContextKey).(LogContext)
	return lc, ok
}

// ContextWithActor stores the resolved Actor in ctx. RequireAuth calls this after
// verifying the bearer token so downstream events are attributed to the user.
func ContextWithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey, a)
}

// ActorFromContext returns the authenticated Actor, if one was resolved.
func ActorFromContext(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey).(Actor)
	return a, ok
}
