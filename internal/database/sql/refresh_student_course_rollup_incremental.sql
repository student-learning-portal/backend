-- ============================================================
-- Incremental loader: recompute analytics_student_course only for
-- (actor, course) pairs touched since the last run.
--
-- Same aggregation as refresh_student_course_rollup.sql, but every CTE is
-- narrowed to the "changed" set below instead of scanning the full
-- event_log/access_grant history each run. Called from
-- PostgresAnalyticsRepository.RefreshStudentCourseRollup, which reads and
-- advances the watermark in analytics_rollup_state.
--
-- Params: $1 = watermark (ingest_ts / grant timestamp of the last run)
-- ============================================================

WITH changed AS (
    SELECT DISTINCT actor_id, course_id
    FROM event_log
    WHERE ingest_ts > $1
      AND actor_id IS NOT NULL
      AND course_id IS NOT NULL
    UNION
    SELECT DISTINCT actor_id, course_id
    FROM access_grant
    WHERE granted_at > $1 OR revoked_at > $1
),
lessons_per_course AS (
    SELECT l.course_id::text AS course_id, count(*) AS lessons_total
    FROM lessons l
    WHERE l.course_id::text IN (SELECT DISTINCT course_id FROM changed)
    GROUP BY l.course_id
),
-- Latest saved completion per (actor, course, lesson) from player.progress_save,
-- restricted to the changed pairs.
latest_pct AS (
    SELECT DISTINCT ON (e.actor_id, e.course_id, e.lesson_id)
           e.actor_id,
           e.course_id,
           e.lesson_id,
           COALESCE((e.payload->>'percent_complete')::numeric, 0) AS pct
    FROM event_log e
    JOIN changed ch ON ch.actor_id = e.actor_id AND ch.course_id = e.course_id
    WHERE e.event_name = 'player.progress_save'
      AND e.actor_id IS NOT NULL
      AND e.course_id IS NOT NULL
      AND e.lesson_id IS NOT NULL
    ORDER BY e.actor_id, e.course_id, e.lesson_id, e.event_ts DESC
),
agg AS (
    SELECT actor_id,
           course_id,
           SUM(pct)                              AS sum_pct,
           count(*) FILTER (WHERE pct >= 100)    AS lessons_completed
    FROM latest_pct
    GROUP BY actor_id, course_id
),
-- Recency from any player.* behavioral event, restricted to the changed pairs.
activity AS (
    SELECT e.actor_id, e.course_id, MAX(e.event_ts) AS last_activity_ts
    FROM event_log e
    JOIN changed ch ON ch.actor_id = e.actor_id AND ch.course_id = e.course_id
    WHERE e.event_name LIKE 'player.%'
      AND e.actor_id IS NOT NULL
      AND e.course_id IS NOT NULL
    GROUP BY e.actor_id, e.course_id
),
-- Enrolled = holds an active (non-revoked) access grant, restricted to the
-- changed pairs.
enrolled AS (
    SELECT DISTINCT ag.actor_id, ag.course_id
    FROM access_grant ag
    JOIN changed ch ON ch.actor_id = ag.actor_id AND ch.course_id = ag.course_id
    WHERE ag.revoked_at IS NULL
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
