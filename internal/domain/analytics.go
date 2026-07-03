package domain

import (
	"context"
	"errors"
	"time"
)

// ErrForbidden is returned when an authenticated actor is not permitted to access
// a resource (e.g. a teacher requesting a course they do not own).
var ErrForbidden = errors.New("forbidden")

// Risk status values returned by ClassifyRisk. They match the OpenAPI enum
// (GetAnalyticsTeacherDashboard200JSONResponseBodyStudentsStatus) exactly.
const (
	RiskOnTrack = "ON_TRACK"
	RiskAtRisk  = "AT_RISK"
)

const (
	hoursPerDay               = 24
	defaultMinProgressPercent = 40.0
	defaultMaxInactiveDays    = 7
)

// StudentProgress is a single learner's rolled-up standing in one course, read
// from the analytics_student_course rollup. It is derived from event_log
// (player.* events) plus course structure, never from live OLTP state.
type StudentProgress struct {
	StudentID        string
	FullName         string
	ProgressPercent  float64
	LessonsCompleted int
	LessonsTotal     int
	// LastActivity is the most recent player.* event timestamp for this
	// (student, course). Nil means the learner is enrolled but has no activity.
	LastActivity *time.Time
}

// RiskThresholds parameterise the at-risk classification. A learner is at risk
// when their course progress is below MinProgressPercent OR they have been
// inactive for more than MaxInactiveDays.
type RiskThresholds struct {
	MinProgressPercent float64
	MaxInactiveDays    int
}

// DefaultRiskThresholds are the out-of-the-box detection limits. They can later
// be made environment-configurable without touching call sites.
var DefaultRiskThresholds = RiskThresholds{
	MinProgressPercent: defaultMinProgressPercent,
	MaxInactiveDays:    defaultMaxInactiveDays,
}

// ClassifyRisk decides whether a learner is at risk and reports how many whole
// days they have been inactive. A learner with no recorded activity
// (LastActivity == nil) is treated as maximally inactive and always at risk.
func ClassifyRisk(p StudentProgress, now time.Time, cfg RiskThresholds) (status string, daysInactive int) {
	inactive := true
	if p.LastActivity != nil {
		daysInactive = int(now.Sub(*p.LastActivity).Hours() / hoursPerDay)
		if daysInactive < 0 {
			daysInactive = 0
		}
		inactive = daysInactive > cfg.MaxInactiveDays
	}

	if p.ProgressPercent < cfg.MinProgressPercent || inactive {
		return RiskAtRisk, daysInactive
	}
	return RiskOnTrack, daysInactive
}

// CourseProgress is a single learner's rolled-up standing in one of their own
// enrolled courses, read from the analytics_student_course rollup. It is the
// student-facing counterpart to StudentProgress (which is scoped the other way:
// one course, every learner).
type CourseProgress struct {
	CourseID         string
	CourseTitle      string
	ProgressPercent  float64
	LessonsCompleted int
	LessonsTotal     int
	// LastActivity is the most recent player.* event timestamp for this
	// (student, course). Nil means the learner is enrolled but has no activity.
	LastActivity *time.Time
}

// AnalyticsRepository is the derived analytics layer over event_log. The rollup
// is materialised by RefreshStudentCourseRollup (the loader) and read back per
// course by CourseStudentProgress, or per learner by StudentCourseProgress.
type AnalyticsRepository interface {
	// CourseStudentProgress returns the rolled-up standing of every enrolled
	// learner in a course, ordered by progress ascending (worst first).
	CourseStudentProgress(ctx context.Context, courseID string) ([]StudentProgress, error)
	// StudentCourseProgress returns the rolled-up standing of every course a
	// learner is enrolled in, ordered by most recently active first.
	StudentCourseProgress(ctx context.Context, studentID string) ([]CourseProgress, error)
	// RefreshStudentCourseRollup recomputes the analytics_student_course rollup
	// from event_log (+ normalized tables). Safe to run repeatedly (upsert).
	RefreshStudentCourseRollup(ctx context.Context) error
}
