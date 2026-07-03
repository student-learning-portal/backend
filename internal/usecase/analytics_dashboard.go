package usecase

import (
	"context"
	"time"

	"github.com/student-learning-portal/backend/internal/domain"
)

// AnalyticsUseCase serves read queries over the derived analytics layer. It is
// distinct from AnalyticsRecorder, which emits events into the raw log; this
// reads the rollup the loader builds from that log.
type AnalyticsUseCase struct {
	analytics  domain.AnalyticsRepository
	catalog    domain.CatalogRepository
	thresholds domain.RiskThresholds
	now        func() time.Time
}

func NewAnalyticsUseCase(
	analytics domain.AnalyticsRepository,
	catalog domain.CatalogRepository,
	thresholds domain.RiskThresholds,
) *AnalyticsUseCase {
	return &AnalyticsUseCase{
		analytics:  analytics,
		catalog:    catalog,
		thresholds: thresholds,
		now:        time.Now,
	}
}

// DashboardStudent is one learner's row in the teacher dashboard.
type DashboardStudent struct {
	StudentID       string
	FullName        string
	ProgressPercent float64
	Status          string
	DaysInactive    int
}

// TeacherDashboardResult is the aggregated teacher view for a single course.
type TeacherDashboardResult struct {
	AtRiskStudents int
	Students       []DashboardStudent
}

// TeacherDashboard returns the at-risk breakdown for one of the teacher's own
// courses. It returns domain.ErrCourseNotFound for an unknown course and
// domain.ErrForbidden when the course belongs to another teacher.
func (uc *AnalyticsUseCase) TeacherDashboard(ctx context.Context, teacherID, courseID string) (TeacherDashboardResult, error) {
	course, err := uc.catalog.GetByID(ctx, courseID)
	if err != nil {
		return TeacherDashboardResult{}, err
	}
	if course.TeacherID != teacherID {
		return TeacherDashboardResult{}, domain.ErrForbidden
	}

	progress, err := uc.analytics.CourseStudentProgress(ctx, courseID)
	if err != nil {
		return TeacherDashboardResult{}, err
	}

	now := uc.now()
	result := TeacherDashboardResult{Students: make([]DashboardStudent, 0, len(progress))}
	for _, p := range progress {
		status, daysInactive := domain.ClassifyRisk(p, now, uc.thresholds)
		if status == domain.RiskAtRisk {
			result.AtRiskStudents++
		}
		result.Students = append(result.Students, DashboardStudent{
			StudentID:       p.StudentID,
			FullName:        p.FullName,
			ProgressPercent: p.ProgressPercent,
			Status:          status,
			DaysInactive:    daysInactive,
		})
	}
	return result, nil
}

// DashboardCourse is one enrolled course's row in the student's own dashboard.
type DashboardCourse struct {
	CourseID         string
	CourseTitle      string
	ProgressPercent  float64
	LessonsCompleted int
	LessonsTotal     int
	Status           string
	DaysInactive     int
}

// StudentDashboardResult is the aggregated self-service view for one learner
// across every course they are enrolled in.
type StudentDashboardResult struct {
	OverallProgress  float64
	CoursesCompleted int
	Courses          []DashboardCourse
}

// StudentDashboard returns the caller's own rolled-up standing across every
// course they are enrolled in (per analytics_student_course). A learner with no
// rollup rows yet (e.g. just enrolled, before the next loader run) gets a
// zero-value result rather than an error.
func (uc *AnalyticsUseCase) StudentDashboard(ctx context.Context, studentID string) (StudentDashboardResult, error) {
	progress, err := uc.analytics.StudentCourseProgress(ctx, studentID)
	if err != nil {
		return StudentDashboardResult{}, err
	}

	now := uc.now()
	result := StudentDashboardResult{Courses: make([]DashboardCourse, 0, len(progress))}
	var progressSum float64
	for _, p := range progress {
		status, daysInactive := domain.ClassifyRisk(domain.StudentProgress{
			ProgressPercent: p.ProgressPercent,
			LastActivity:    p.LastActivity,
		}, now, uc.thresholds)

		progressSum += p.ProgressPercent
		if p.LessonsTotal > 0 && p.LessonsCompleted >= p.LessonsTotal {
			result.CoursesCompleted++
		}
		result.Courses = append(result.Courses, DashboardCourse{
			CourseID:         p.CourseID,
			CourseTitle:      p.CourseTitle,
			ProgressPercent:  p.ProgressPercent,
			LessonsCompleted: p.LessonsCompleted,
			LessonsTotal:     p.LessonsTotal,
			Status:           status,
			DaysInactive:     daysInactive,
		})
	}
	if len(progress) > 0 {
		result.OverallProgress = progressSum / float64(len(progress))
	}
	return result, nil
}
