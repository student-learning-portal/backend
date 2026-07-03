package usecase

import (
	"context"
	"fmt"
	"math"

	"github.com/student-learning-portal/backend/internal/domain"
)

const percentScale = 100

type ResultsUseCase struct {
	results domain.ResultsRepository
}

func NewResultsUseCase(results domain.ResultsRepository) *ResultsUseCase {
	return &ResultsUseCase{results: results}
}

// MyResults aggregates a learner's progress across their enrolled courses:
// a per-course completion percentage plus an overall percentage (weighted by
// lesson count across all courses) and how many courses are fully complete.
func (uc *ResultsUseCase) MyResults(ctx context.Context, actorID string) (domain.StudentResults, error) {
	courses, err := uc.results.StudentResults(ctx, actorID)
	if err != nil {
		return domain.StudentResults{}, fmt.Errorf("my results: %w", err)
	}

	var totalLessons, totalCompleted, coursesCompleted int
	for i := range courses {
		c := &courses[i]
		if c.LessonsTotal > 0 {
			c.ProgressPercent = percent(c.LessonsCompleted, c.LessonsTotal)
			if c.LessonsCompleted >= c.LessonsTotal {
				coursesCompleted++
			}
		}
		totalLessons += c.LessonsTotal
		totalCompleted += c.LessonsCompleted
	}

	return domain.StudentResults{
		OverallProgressPercent: percent(totalCompleted, totalLessons),
		CoursesEnrolled:        len(courses),
		CoursesCompleted:       coursesCompleted,
		Courses:                courses,
	}, nil
}

// percent returns completed/total as a 0–100 value rounded to two decimals,
// or 0 when there is nothing to complete.
func percent(completed, total int) float64 {
	if total <= 0 {
		return 0
	}
	return math.Round(float64(completed)*percentScale*percentScale/float64(total)) / percentScale
}
