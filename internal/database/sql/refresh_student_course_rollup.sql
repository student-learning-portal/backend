-- ============================================================
-- Loader: recompute the analytics_student_course rollup from event_log.
--
-- Derived analytics layer (logging-architecture.md §5, analytics-ml-layer.md):
-- event_log is the source of truth; this aggregation is replayable and can be
-- re-run at any time. Behavioral metrics come from player.* events; course
-- structure (lesson count) and enrollment come from the normalized tables.
--
-- Runnable two ways with identical effect:
--   * Go loader: embedded via //go:embed in internal/database/analytics.go
--   * Directly:  psql -f internal/database/sql/refresh_student_course_rollup.sql
-- ============================================================

WITH lessons_per_course AS (
    SELECT course_id::text AS course_id, count(*) AS lessons_total
    FROM lessons
    GROUP BY course_id
),
-- Latest saved completion per (actor, course, lesson) from player.progress_save.
latest_pct AS (
    SELECT DISTINCT ON (actor_id, course_id, lesson_id)
           actor_id,
           course_id,
           lesson_id,
           COALESCE((payload->>'percent_complete')::numeric, 0) AS pct
    FROM event_log
    WHERE event_name = 'player.progress_save'
      AND actor_id IS NOT NULL
      AND course_id IS NOT NULL
      AND lesson_id IS NOT NULL
    ORDER BY actor_id, course_id, lesson_id, event_ts DESC
),
agg AS (
    SELECT actor_id,
           course_id,
           SUM(pct)                              AS sum_pct,
           count(*) FILTER (WHERE pct >= 100)    AS lessons_completed
    FROM latest_pct
    GROUP BY actor_id, course_id
),
-- Recency from any player.* behavioral event.
activity AS (
    SELECT actor_id, course_id, MAX(event_ts) AS last_activity_ts
    FROM event_log
    WHERE event_name LIKE 'player.%'
      AND actor_id IS NOT NULL
      AND course_id IS NOT NULL
    GROUP BY actor_id, course_id
),
-- Enrolled = holds an active (non-revoked) access grant.
enrolled AS (
    SELECT DISTINCT actor_id, course_id
    FROM access_grant
    WHERE revoked_at IS NULL
)
INSERT INTO analytics_student_course
    (actor_id, course_id, lessons_total, lessons_completed, progress_percent, last_activity_ts, computed_at)
SELECT
    e.actor_id,
    e.course_id,
    COALESCE(lc.lessons_total, 0),
    COALESCE(a.lessons_completed, 0),
    CASE
        WHEN COALESCE(lc.lessons_total, 0) > 0
        THEN LEAST(ROUND(COALESCE(a.sum_pct, 0) / lc.lessons_total, 2), 100)
        ELSE 0
    END AS progress_percent,
    act.last_activity_ts,
    now()
FROM enrolled e
LEFT JOIN lessons_per_course lc ON lc.course_id = e.course_id
LEFT JOIN agg a                 ON a.actor_id = e.actor_id AND a.course_id = e.course_id
LEFT JOIN activity act          ON act.actor_id = e.actor_id AND act.course_id = e.course_id
ON CONFLICT (actor_id, course_id) DO UPDATE SET
    lessons_total     = EXCLUDED.lessons_total,
    lessons_completed = EXCLUDED.lessons_completed,
    progress_percent  = EXCLUDED.progress_percent,
    last_activity_ts  = EXCLUDED.last_activity_ts,
    computed_at       = EXCLUDED.computed_at;
