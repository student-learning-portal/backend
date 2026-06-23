// Package eventlog holds output adapters for the analytics event stream.
package eventlog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/student-learning-portal/backend/internal/domain"
)

// NDJSONSink writes each event as one newline-delimited JSON line (§1.1). This is
// the raw transport tier of the pipeline (§5.1): a broker/loader can tail it and
// load Postgres. Writes are serialized with a mutex so concurrent requests never
// interleave bytes within a line.
type NDJSONSink struct {
	mu  sync.Mutex
	w   io.Writer
	now func() time.Time
}

// NewNDJSONSink writes NDJSON events to w (typically os.Stdout, kept separate from
// human-readable operational logs on stderr).
func NewNDJSONSink(w io.Writer) *NDJSONSink {
	return &NDJSONSink{w: w, now: time.Now}
}

// Emit stamps the server-side ingest time and appends the event as one JSON line.
func (s *NDJSONSink) Emit(_ context.Context, e domain.Event) error {
	if e.IngestTs.IsZero() {
		e.IngestTs = s.now()
	}

	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	line = append(line, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.w.Write(line); err != nil {
		return fmt.Errorf("write event: %w", err)
	}
	return nil
}
