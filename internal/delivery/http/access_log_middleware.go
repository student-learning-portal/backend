package http

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/student-learning-portal/backend/internal/logging"
)

// statusRecorder wraps a ResponseWriter to capture the status code and byte
// count actually written, neither of which http.ResponseWriter exposes after
// the fact.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// WithAccessLog logs one structured line per request (method, path, status,
// duration, byte count, plus whatever correlation/actor identifiers are on
// the context) so every request is debuggable from Grafana without needing to
// reproduce it locally. It also recovers panics: a handler bug logs with a
// stack trace and returns 500 instead of killing the connection with no
// record of what happened. Must run inside WithLogContext so the request
// already carries its correlation id by the time it logs.
func WithAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}

		defer func() {
			log := logging.FromContext(r.Context())

			if rerr := recover(); rerr != nil {
				log.Error("http request panicked",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.Any("panic", rerr),
					slog.String("stack", string(debug.Stack())),
				)
				if rec.status == 0 {
					http.Error(rec, "internal server error", http.StatusInternalServerError)
				}
				return
			}

			level := slog.LevelInfo
			switch {
			case rec.status >= http.StatusInternalServerError:
				level = slog.LevelError
			case rec.status >= http.StatusBadRequest:
				level = slog.LevelWarn
			}
			log.LogAttrs(r.Context(), level, "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				slog.Int("bytes", rec.bytes),
				slog.String("remote_ip", truncateIP(clientIP(r))),
			)
		}()

		next.ServeHTTP(rec, r)
	})
}
