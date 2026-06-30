# Analytics / ML Layer — Derived from the Event Log

> Companion to `logging-architecture.md`. That document specifies the raw event
> log (source of truth). This one specifies the **derived layer**: aggregations
> computed over `event_log` + the normalized tables. The raw log stays
> replayable; everything here can be rebuilt from it at any time.

---

## 1. Principles

- **Log is source of truth.** Derived tables are caches of the log, never the
  other way around. Re-running a loader reproduces them.
- **Raw tables stay raw.** No aggregation columns are added to `event_log` /
  `progress_state`; derivation lives here (`logging-architecture.md` §5.5).
- **Policy in code, metrics in SQL.** The rollup stores raw measurements; the
  at-risk *classification* (thresholds) lives in the application
  (`domain.ClassifyRisk`) so it is testable and tunable in one place.

---

## 2. Rollup: `analytics_student_course`

Per `(student, course)` standing. Migration `000004_analytics_rollup.up.sql`.

| Column | Meaning |
|---|---|
| `actor_id`, `course_id` | Composite key (TEXT, matches log/grant ids). |
| `lessons_total` | Lessons in the course (denominator). |
| `lessons_completed` | Lessons whose latest saved completion ≥ 100. |
| `progress_percent` | Course completion 0–100 = Σ(latest per-lesson %) / `lessons_total`, capped at 100. |
| `last_activity_ts` | Most recent `player.*` event; `NULL` = enrolled, never active. |
| `computed_at` | When the loader last wrote this row. |

### Derivation (loader)
`internal/database/sql/refresh_student_course_rollup.sql` — a single idempotent
`INSERT … ON CONFLICT DO UPDATE`:

- **Progress** from `player.progress_save`: latest `payload.percent_complete`
  per `(actor, course, lesson)` (`DISTINCT ON … ORDER BY event_ts DESC`).
- **Recency** = `MAX(event_ts)` over `event_name LIKE 'player.%'`.
- **Enrollment** = active `access_grant` (`revoked_at IS NULL`).
- **Course size** = lesson count from `lessons`.

Run it two equivalent ways (the Go loader embeds the same `.sql`):

```
go run ./cmd/analytics-loader
# or
psql -f internal/database/sql/refresh_student_course_rollup.sql
```

Schedule it (cron) for periodic refresh; there is currently no auto-refresh on
write.

---

## 3. At-risk detection

`domain.ClassifyRisk(StudentProgress, now, RiskThresholds)` →
`status ∈ {ON_TRACK, AT_RISK}` + `days_inactive`.

A learner is **AT_RISK** when **either**:
- `progress_percent < MinProgressPercent`, **or**
- inactive for more than `MaxInactiveDays` (no activity ⇒ always at risk).

Defaults (`domain.DefaultRiskThresholds`): **40%** progress, **7** days.

Consumed by `GET /api/v1/analytics/teacher/dashboard?course_id=…`
(`AnalyticsUseCase.TeacherDashboard`), which additionally enforces that the
caller is the teacher who owns the course.

---

## 4. Seeding

`scripts/seed_analytics.sql` generates a mock cohort by writing realistic
`player.*` events (same taxonomy the handlers emit) + enrollment, then the loader
builds the rollup from them. See the header of that file for usage.

---

## 5. Future

- Auto-refresh (trigger/queue) instead of manual loader runs.
- Student-facing rollup for `GET /analytics/student/me`.
- `assessment.*` signals (quiz scores) feeding the risk model.
- Columnar store for `event_log` if behavioral volume outgrows Postgres
  (`logging-architecture.md` §5.5).
