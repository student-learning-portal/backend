-- ============================================================
-- Derived analytics rollup (analytics-ml-layer.md)
--
-- Per (student, course) standing, recomputed from event_log by the loader
-- (internal/database/sql/refresh_student_course_rollup.sql). Stores raw metrics
-- only; the at-risk classification lives in the application (domain.ClassifyRisk)
-- so thresholds stay in one place.
-- ============================================================
CREATE TABLE IF NOT EXISTS analytics_student_course (
    actor_id          TEXT         NOT NULL,
    course_id         TEXT         NOT NULL,
    lessons_total     INTEGER      NOT NULL DEFAULT 0,
    lessons_completed INTEGER      NOT NULL DEFAULT 0,
    progress_percent  NUMERIC(5,2) NOT NULL DEFAULT 0,   -- course completion, 0–100
    last_activity_ts  TIMESTAMPTZ,                        -- NULL = enrolled, no activity
    computed_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    PRIMARY KEY (actor_id, course_id)
);

CREATE INDEX IF NOT EXISTS ix_ascp_course ON analytics_student_course (course_id);
