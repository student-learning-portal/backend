package database

import (
	"context"
	"database/sql"
	_ "embed" // enables //go:embed of the rollup refresh SQL
	"fmt"
	"time"

	"github.com/student-learning-portal/backend/internal/domain"
)

// refreshStudentCourseRollupSQL is the full-table loader aggregation, kept in a
// .sql file so the exact same statement can be run via `psql -f` for a manual
// full reconciliation pass (e.g. after a backfill/replay into event_log).
//
//go:embed sql/refresh_student_course_rollup.sql
var refreshStudentCourseRollupSQL string

// refreshStudentCourseRollupIncrementalSQL is the same aggregation narrowed to
// only the (actor, course) pairs touched since a watermark. RefreshStudentCourseRollup
// uses this by default so routine runs don't rescan the entire event_log.
//
//go:embed sql/refresh_student_course_rollup_incremental.sql
var refreshStudentCourseRollupIncrementalSQL string

// refreshStudentCourseRollupOneSQL is the same aggregation scoped to a single
// (actor, course) pair, cheap enough to run inline on the request path.
//
//go:embed sql/refresh_student_course_rollup_one.sql
var refreshStudentCourseRollupOneSQL string

type PostgresAnalyticsRepository struct {
	db *sql.DB
}

func NewPostgresAnalyticsRepository(db *sql.DB) domain.AnalyticsRepository {
	return NewPostgresAnalyticsRepo(db)
}

// NewPostgresAnalyticsRepo returns the concrete repository type (rather than the
// domain.AnalyticsRepository interface), for callers that need access to
// RefreshStudentCourseRollupFull, which isn't part of the interface.
func NewPostgresAnalyticsRepo(db *sql.DB) *PostgresAnalyticsRepository {
	return &PostgresAnalyticsRepository{db: db}
}

// RefreshStudentCourseRollup recomputes the analytics_student_course rollup from
// event_log. It only rescans (actor, course) pairs touched since the last run
// (tracked in analytics_rollup_state), rather than the full event_log history,
// so routine runs stay cheap as the log grows. The statement upserts, so
// re-running is idempotent, and a run that finds nothing new is a no-op.
func (r *PostgresAnalyticsRepository) RefreshStudentCourseRollup(ctx context.Context) error {
	runAt := time.Now().UTC()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("refresh student-course rollup: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	var watermark time.Time
	if err := tx.QueryRowContext(ctx,
		`SELECT last_ingest_ts FROM analytics_rollup_state WHERE id = 1 FOR UPDATE`,
	).Scan(&watermark); err != nil {
		return fmt.Errorf("refresh student-course rollup: read watermark: %w", err)
	}

	if _, err := tx.ExecContext(ctx, refreshStudentCourseRollupIncrementalSQL, watermark); err != nil {
		return fmt.Errorf("refresh student-course rollup: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE analytics_rollup_state SET last_ingest_ts = $1 WHERE id = 1`, runAt,
	); err != nil {
		return fmt.Errorf("refresh student-course rollup: advance watermark: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("refresh student-course rollup: commit: %w", err)
	}
	return nil
}

// RefreshStudentCourseRollupFull recomputes the analytics_student_course rollup
// from the entire event_log/access_grant history, ignoring the incremental
// watermark. Use for manual reconciliation (e.g. after a backfill or replay
// that inserted rows with old ingest_ts values the incremental path would
// otherwise skip).
func (r *PostgresAnalyticsRepository) RefreshStudentCourseRollupFull(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, refreshStudentCourseRollupSQL); err != nil {
		return fmt.Errorf("refresh student-course rollup (full): %w", err)
	}
	if _, err := r.db.ExecContext(ctx,
		`UPDATE analytics_rollup_state SET last_ingest_ts = $1 WHERE id = 1`, time.Now().UTC(),
	); err != nil {
		return fmt.Errorf("refresh student-course rollup (full): advance watermark: %w", err)
	}
	return nil
}

// RefreshStudentCourseRow recomputes a single (actor, course) rollup row from
// event_log. Safe to run repeatedly (upsert); inserts nothing if the actor
// holds no active grant for the course.
func (r *PostgresAnalyticsRepository) RefreshStudentCourseRow(ctx context.Context, actorID, courseID string) error {
	if _, err := r.db.ExecContext(ctx, refreshStudentCourseRollupOneSQL, actorID, courseID); err != nil {
		return fmt.Errorf("refresh student course row: %w", err)
	}
	return nil
}

// CourseStudentProgress reads every enrolled learner's rolled-up standing for a
// course, worst progress first, joining the full name for display.
func (r *PostgresAnalyticsRepository) CourseStudentProgress(ctx context.Context, courseID string) ([]domain.StudentProgress, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT a.actor_id, COALESCE(u.full_name, ''), a.progress_percent,
		        a.lessons_completed, a.lessons_total, a.last_activity_ts
		 FROM analytics_student_course a
		 LEFT JOIN users u ON u.id::text = a.actor_id
		 WHERE a.course_id = $1
		 ORDER BY a.progress_percent ASC, a.actor_id ASC`,
		courseID,
	)
	if err != nil {
		return nil, fmt.Errorf("query course student progress: %w", err)
	}
	defer rows.Close()

	out := []domain.StudentProgress{}
	for rows.Next() {
		var (
			p            domain.StudentProgress
			lastActivity sql.NullTime
		)
		if err := rows.Scan(
			&p.StudentID, &p.FullName, &p.ProgressPercent,
			&p.LessonsCompleted, &p.LessonsTotal, &lastActivity,
		); err != nil {
			return nil, fmt.Errorf("scan student progress: %w", err)
		}
		if lastActivity.Valid {
			t := lastActivity.Time
			p.LastActivity = &t
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate student progress: %w", err)
	}
	return out, nil
}

// StudentCourseProgress reads a learner's rolled-up standing across every
// course they are enrolled in, joining the course title for display and
// ordering most recently active first (never-active courses last).
func (r *PostgresAnalyticsRepository) StudentCourseProgress(ctx context.Context, studentID string) ([]domain.CourseProgress, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT a.course_id, COALESCE(c.title, ''), a.progress_percent,
		        a.lessons_completed, a.lessons_total, a.last_activity_ts
		 FROM analytics_student_course a
		 LEFT JOIN courses c ON c.id::text = a.course_id
		 WHERE a.actor_id = $1
		 ORDER BY a.last_activity_ts DESC NULLS LAST, a.course_id ASC`,
		studentID,
	)
	if err != nil {
		return nil, fmt.Errorf("query student course progress: %w", err)
	}
	defer rows.Close()

	out := []domain.CourseProgress{}
	for rows.Next() {
		var (
			p            domain.CourseProgress
			lastActivity sql.NullTime
		)
		if err := rows.Scan(
			&p.CourseID, &p.CourseTitle, &p.ProgressPercent,
			&p.LessonsCompleted, &p.LessonsTotal, &lastActivity,
		); err != nil {
			return nil, fmt.Errorf("scan student course progress: %w", err)
		}
		if lastActivity.Valid {
			t := lastActivity.Time
			p.LastActivity = &t
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate student course progress: %w", err)
	}
	return out, nil
}
