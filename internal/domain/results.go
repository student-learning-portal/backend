package domain

import "context"

// CourseResult is a learner's completion standing in one enrolled course.
// A lesson counts as completed once its saved progress reaches 100% — the same
// threshold the analytics rollup uses — read live from progress_state.
type CourseResult struct {
	CourseID         string  `json:"course_id"`
	Title            string  `json:"title"`
	LessonsTotal     int     `json:"lessons_total"`
	LessonsCompleted int     `json:"lessons_completed"`
	ProgressPercent  float64 `json:"progress_percent"`
}

// StudentResults is a learner's aggregated progress across every course they are
// currently enrolled in (hold an active, non-revoked access grant for).
type StudentResults struct {
	OverallProgressPercent float64        `json:"overall_progress_percent"`
	CoursesEnrolled        int            `json:"courses_enrolled"`
	CoursesCompleted       int            `json:"courses_completed"`
	Courses                []CourseResult `json:"courses"`
}

// ResultsRepository returns the per-course lesson tallies for a learner. It
// leaves ProgressPercent unset; the use case derives the percentages.
type ResultsRepository interface {
	StudentResults(ctx context.Context, actorID string) ([]CourseResult, error)
}
