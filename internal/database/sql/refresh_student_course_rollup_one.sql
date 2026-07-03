-- ============================================================
-- Point refresh: recompute a single (actor, course) row in the
-- analytics_student_course rollup.
--
-- Same aggregation as refresh_student_course_rollup.sql (analytics-ml-layer.md
-- §2), scoped down to one learner + course so it is cheap enough to run
-- synchronously on the request path (usecase.RollupRefreshSink) right after a
-- progress-changing event, instead of waiting for the periodic full-table
-- loader (cmd/analytics-loader). The full loader keeps running as a
-- reconciliation pass (e.g. for replayed/backfilled events) — this is a
-- read-your-own-writes shortcut, not a replacement for it.
--
-- Params: $1 = actor_id, $2 = course_id
-- ============================================================

WITH lessons_per_course AS (
    SELECT count(*) AS lessons_total
    FROM lessons
    WHERE course_id::text = $2
),
-- Latest saved completion per lesson from player.progress_save.
latest_pct AS (
    SELECT DISTINCT ON (lesson_id)
           lesson_id,
           COALESCE((payload->>'percent_complete')::numeric, 0) AS pct
    FROM event_log
    WHERE event_name = 'player.progress_save'
      AND actor_id = $1
      AND course_id = $2
      AND lesson_id IS NOT NULL
    ORDER BY lesson_id, event_ts DESC
),
agg AS (
    SELECT COALESCE(SUM(pct), 0)                 AS sum_pct,
           count(*) FILTER (WHERE pct >= 100)    AS lessons_completed
    FROM latest_pct
),
-- Recency from any player.* behavioral event for this (actor, course).
activity AS (
    SELECT MAX(event_ts) AS last_activity_ts
    FROM event_log
    WHERE event_name LIKE 'player.%'
      AND actor_id = $1
      AND course_id = $2
),
-- Enrolled = holds an active (non-revoked) access grant. If this yields no row
-- the CROSS JOINs below produce nothing, so nothing is inserted/updated.
enrolled AS (
    SELECT 1
    FROM access_grant
    WHERE actor_id = $1 AND course_id = $2 AND revoked_at IS NULL
    LIMIT 1
)
INSERT INTO analytics_student_course
    (actor_id, course_id, lessons_total, lessons_completed, progress_percent, last_activity_ts, computed_at)
SELECT
    $1,
    $2,
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
CROSS JOIN lessons_per_course lc
CROSS JOIN agg a
CROSS JOIN activity act
ON CONFLICT (actor_id, course_id) DO UPDATE SET
    lessons_total     = EXCLUDED.lessons_total,
    lessons_completed = EXCLUDED.lessons_completed,
    progress_percent  = EXCLUDED.progress_percent,
    last_activity_ts  = EXCLUDED.last_activity_ts,
    computed_at       = EXCLUDED.computed_at;
