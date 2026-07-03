// Command analytics-loader recomputes the derived analytics rollup
// (analytics_student_course) from event_log. It is the "loader" stage of the
// pipeline in logging-architecture.md §5.1 and can be run as a one-off job or on
// a schedule (cron). It only needs the database connection — no JWT.
package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/student-learning-portal/backend/internal/database"
)

const refreshTimeout = 5 * time.Minute

func main() {
	full := flag.Bool("full", false, "recompute the rollup from the entire event_log/access_grant history, ignoring the incremental watermark (manual reconciliation)")
	flag.Parse()

	database.InitDB()

	repo := database.NewPostgresAnalyticsRepo(database.DB)

	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel()

	start := time.Now()
	var err error
	if *full {
		err = repo.RefreshStudentCourseRollupFull(ctx)
	} else {
		err = repo.RefreshStudentCourseRollup(ctx)
	}
	if err != nil {
		log.Fatalf("analytics-loader: refresh failed: %v", err)
	}
	log.Printf("analytics-loader: student_course rollup refreshed in %s (full=%v)", time.Since(start), *full)
}
