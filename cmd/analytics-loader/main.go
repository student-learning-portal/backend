// Command analytics-loader recomputes the derived analytics rollup
// (analytics_student_course) from event_log. It is the "loader" stage of the
// pipeline in logging-architecture.md §5.1 and can be run as a one-off job or on
// a schedule (cron). It only needs the database connection — no JWT.
package main

import (
	"context"
	"log"
	"time"

	"github.com/student-learning-portal/backend/internal/database"
)

const refreshTimeout = 5 * time.Minute

func main() {
	database.InitDB()

	repo := database.NewPostgresAnalyticsRepository(database.DB)

	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel()

	start := time.Now()
	if err := repo.RefreshStudentCourseRollup(ctx); err != nil {
		log.Fatalf("analytics-loader: refresh failed: %v", err)
	}
	log.Printf("analytics-loader: student_course rollup refreshed in %s", time.Since(start))
}
