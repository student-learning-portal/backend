package usecase

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/student-learning-portal/backend/internal/domain"
)

const percentScale = 100

type ResultsUseCase struct {
	results    domain.ResultsRepository
	thresholds domain.RiskThresholds
	now        func() time.Time
}

func NewResultsUseCase(results domain.ResultsRepository, thresholds domain.RiskThresholds) *ResultsUseCase {
	return &ResultsUseCase{results: results, thresholds: thresholds, now: time.Now}
}

// MyResults aggregates a learner's progress across their enrolled courses:
// a per-course completion percentage plus an overall percentage (weighted by
// lesson count across all courses) and how many courses are fully complete.
// Each course also carries the same at-risk classification the analytics
// dashboard uses (domain.ClassifyRisk), so callers get one consistent signal
// regardless of which endpoint they read.
func (uc *ResultsUseCase) MyResults(ctx context.Context, actorID string) (domain.StudentResults, error) {
	courses, err := uc.results.StudentResults(ctx, actorID)
	if err != nil {
		return domain.StudentResults{}, fmt.Errorf("my results: %w", err)
	}

	now := uc.now()
	var totalLessons, totalCompleted, coursesCompleted int
	for i := range courses {
		c := &courses[i]
		if c.LessonsTotal > 0 {
			c.ProgressPercent = percent(c.LessonsCompleted, c.LessonsTotal)
			if c.LessonsCompleted >= c.LessonsTotal {
				coursesCompleted++
			}
		}
		c.Status, c.DaysInactive = domain.ClassifyRisk(domain.StudentProgress{
			ProgressPercent: c.ProgressPercent,
			LastActivity:    c.LastActivity,
		}, now, uc.thresholds)
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
