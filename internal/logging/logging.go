// Package logging provides the structured operational logger (as opposed to
// the business-event NDJSON stream in internal/domain/event.go, which is
// specified by logging-architecture.md). It emits one JSON object per line to
// stdout so a Grafana stack (Loki/Promtail scraping container logs, or any
// other JSON log collector) can index and query on fields like level,
// correlation_id, and actor_id without any parsing rules.
package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/student-learning-portal/backend/internal/domain"
)

var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

// Init configures the process-wide structured logger for service (e.g.
// "portal", "analytics-loader") and installs it as the slog default, so
// third-party code calling slog.Info/Error also lands in the same JSON
// stream. Level is controlled by LOG_LEVEL (debug|info|warn|error, default
// info); AddSource is always on since a file:line pointer is what turns a
// log line into an actionable debugging lead.
func Init(service string) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level:     parseLevel(os.Getenv("LOG_LEVEL")),
		AddSource: true,
	}
	l := slog.New(slog.NewJSONHandler(os.Stdout, opts)).With(
		slog.String("service", service),
		slog.String("env", envOrDefault("APP_ENV", "dev")),
		slog.String("instance", envOrDefault("HOSTNAME", "local")),
		slog.String("release", envOrDefault("RELEASE", "unknown")),
	)
	slog.SetDefault(l)
	logger = l
	return l
}

// L returns the process-wide structured logger. Safe to call before Init
// (falls back to an unconfigured JSON logger) so package-level code and tests
// never see a nil logger.
func L() *slog.Logger {
	return logger
}

// FromContext returns the process logger enriched with whatever
// request-scoped identifiers are available: the correlation/session/trace ids
// captured by WithLogContext, and the authenticated actor once RequireAuth has
// run. Use this inside request handling so operational log lines can be
// joined in Grafana with the matching event_log rows and traces via
// correlation_id/trace_id.
func FromContext(ctx context.Context) *slog.Logger {
	l := logger
	if lc, ok := domain.LogContextFromContext(ctx); ok {
		if lc.CorrelationID != "" {
			l = l.With(slog.String("correlation_id", lc.CorrelationID))
		}
		if lc.SessionID != "" {
			l = l.With(slog.String("session_id", lc.SessionID))
		}
		if lc.TraceID != "" {
			l = l.With(slog.String("trace_id", lc.TraceID))
		}
		if lc.SpanID != "" {
			l = l.With(slog.String("span_id", lc.SpanID))
		}
	}
	if actor, ok := domain.ActorFromContext(ctx); ok {
		if actor.ActorID != "" {
			l = l.With(slog.String("actor_id", actor.ActorID))
		}
		if actor.Role != "" {
			l = l.With(slog.String("role", string(actor.Role)))
		}
	}
	return l
}

func parseLevel(v string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
