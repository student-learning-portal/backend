-- ============================================================
-- Support incremental rollup refresh (internal/database/analytics.go).
--
-- The loader used to recompute analytics_student_course from the entire
-- event_log/access_grant history on every run. As event_log grows this full
-- rescan gets more expensive per run even though most runs only need to
-- reflect a handful of new events. analytics_rollup_state tracks a
-- watermark (the ingest_ts of the last processed event) so each run only
-- rescans (actor, course) pairs touched since then.
-- ============================================================
CREATE INDEX IF NOT EXISTS ix_event_ingest_ts ON event_log (ingest_ts);

-- last_ingest_ts defaults to the Unix epoch, not '-infinity': pgx cannot scan
-- Postgres' infinity sentinel into a Go time.Time (it comes back as a raw
-- string and errors), and every real event_log/access_grant timestamp is
-- already after 1970, so epoch behaves identically as a "beginning of time"
-- watermark.
CREATE TABLE IF NOT EXISTS analytics_rollup_state (
    id             SMALLINT    PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    last_ingest_ts TIMESTAMPTZ NOT NULL DEFAULT 'epoch'
);

INSERT INTO analytics_rollup_state (id, last_ingest_ts)
VALUES (1, 'epoch')
ON CONFLICT (id) DO NOTHING;
