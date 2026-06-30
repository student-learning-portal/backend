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
