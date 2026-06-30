package database

import (
	"context"
	"database/sql"
	_ "embed" // enables //go:embed of the rollup refresh SQL
	"fmt"

	"github.com/student-learning-portal/backend/internal/domain"
)

// refreshStudentCourseRollupSQL is the loader aggregation, kept in a .sql file so
// the exact same statement can be run via `psql -f` for manual refreshes/debugging.
//
//go:embed sql/refresh_student_course_rollup.sql
var refreshStudentCourseRollupSQL string

type PostgresAnalyticsRepository struct {
	db *sql.DB
}

func NewPostgresAnalyticsRepository(db *sql.DB) domain.AnalyticsRepository {
	return &PostgresAnalyticsRepository{db: db}
}

// RefreshStudentCourseRollup recomputes the analytics_student_course rollup from
// event_log. The statement upserts, so re-running is idempotent.
func (r *PostgresAnalyticsRepository) RefreshStudentCourseRollup(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, refreshStudentCourseRollupSQL); err != nil {
		return fmt.Errorf("refresh student-course rollup: %w", err)
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
