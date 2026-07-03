package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/student-learning-portal/backend/internal/domain"
)

type PostgresResultsRepository struct {
	db *sql.DB
}

func NewPostgresResultsRepository(db *sql.DB) domain.ResultsRepository {
	return &PostgresResultsRepository{db: db}
}

// StudentResults returns one row per course the learner is actively enrolled in
// (holds a non-revoked access grant), with the course's total lesson count and
// how many the learner has completed (progress_state.percent_complete >= 100).
// Percentages are derived by the use case, not here.
//
// The joins mirror the column-type quirks elsewhere in the schema: courses.id is
// UUID while access_grant.course_id and progress_state.course_id are TEXT, so the
// course id is cast to text where it meets those tables.
func (r *PostgresResultsRepository) StudentResults(ctx context.Context, actorID string) ([]domain.CourseResult, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT c.id::text,
		        c.title,
		        COALESCE(lt.lessons_total, 0),
		        COALESCE(pc.lessons_completed, 0),
		        pc.last_activity
		 FROM courses c
		 LEFT JOIN (
		     SELECT course_id, count(*) AS lessons_total
		     FROM lessons GROUP BY course_id
		 ) lt ON lt.course_id = c.id
		 LEFT JOIN (
		     SELECT course_id,
		            count(*) FILTER (WHERE percent_complete >= 100) AS lessons_completed,
		            max(updated_at) AS last_activity
		     FROM progress_state WHERE actor_id = $1 GROUP BY course_id
		 ) pc ON pc.course_id = c.id::text
		 WHERE EXISTS (
		     SELECT 1 FROM access_grant ag
		     WHERE ag.course_id = c.id::text AND ag.actor_id = $1 AND ag.revoked_at IS NULL
		 )
		 ORDER BY c.title`,
		actorID,
	)
	if err != nil {
		return nil, fmt.Errorf("student results: %w", err)
	}
	defer rows.Close()

	results := []domain.CourseResult{}
	for rows.Next() {
		var (
			cr           domain.CourseResult
			lastActivity sql.NullTime
		)
		if err := rows.Scan(&cr.CourseID, &cr.Title, &cr.LessonsTotal, &cr.LessonsCompleted, &lastActivity); err != nil {
			return nil, fmt.Errorf("scan student result: %w", err)
		}
		if lastActivity.Valid {
			t := lastActivity.Time
			cr.LastActivity = &t
		}
		results = append(results, cr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate student results: %w", err)
	}
	return results, nil
}
