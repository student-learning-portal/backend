package usecase

import (
	"context"
	"fmt"

	"github.com/student-learning-portal/backend/internal/domain"
)

// RatingUseCase gates and stores 1-10 course/teacher ratings. Only students
// may rate (enforced by the HTTP handler's role check, same as
// ReviewHandler), and only for a course they're enrolled in or a teacher
// whose course they're enrolled in.
type RatingUseCase struct {
	courseRatings  domain.CourseRatingRepository
	teacherRatings domain.TeacherRatingRepository
	catalog        domain.CatalogRepository
	entitlements   domain.EntitlementRepository
	users          domain.UserRepository
}

func NewRatingUseCase(
	courseRatings domain.CourseRatingRepository,
	teacherRatings domain.TeacherRatingRepository,
	catalog domain.CatalogRepository,
	entitlements domain.EntitlementRepository,
	users domain.UserRepository,
) *RatingUseCase {
	return &RatingUseCase{
		courseRatings:  courseRatings,
		teacherRatings: teacherRatings,
		catalog:        catalog,
		entitlements:   entitlements,
		users:          users,
	}
}

func validateScore(score int) error {
	if score < 1 || score > 10 {
		return domain.ErrInvalidRating
	}
	return nil
}

// RateCourse records studentID's 1-10 score for courseID. studentID must
// hold an active access grant for the course.
func (uc *RatingUseCase) RateCourse(ctx context.Context, studentID, courseID string, score int) (domain.CourseRating, error) {
	if err := validateScore(score); err != nil {
		return domain.CourseRating{}, err
	}
	enrolled, err := uc.entitlements.HasActiveGrant(ctx, studentID, courseID)
	if err != nil {
		return domain.CourseRating{}, fmt.Errorf("check enrollment: %w", err)
	}
	if !enrolled {
		return domain.CourseRating{}, domain.ErrForbidden
	}
	rating, err := uc.courseRatings.Upsert(ctx, studentID, courseID, score)
	if err != nil {
		return domain.CourseRating{}, fmt.Errorf("rate course: %w", err)
	}
	return rating, nil
}

// CourseRatingSummary returns the aggregate 1-10 rating for a course.
func (uc *RatingUseCase) CourseRatingSummary(ctx context.Context, courseID string) (domain.RatingSummary, error) {
	if _, err := uc.catalog.GetByID(ctx, courseID); err != nil {
		return domain.RatingSummary{}, err
	}
	return uc.courseRatings.Summary(ctx, courseID)
}

// MyCourseRating returns studentID's own rating for courseID, or
// domain.ErrRatingNotFound if they haven't rated it yet.
func (uc *RatingUseCase) MyCourseRating(ctx context.Context, studentID, courseID string) (domain.CourseRating, error) {
	if _, err := uc.catalog.GetByID(ctx, courseID); err != nil {
		return domain.CourseRating{}, err
	}
	return uc.courseRatings.GetByStudent(ctx, studentID, courseID)
}

// requireTeacher fails with domain.ErrUserNotFound unless teacherID belongs
// to a teacher account (prevents rating an arbitrary/non-teacher user id).
func (uc *RatingUseCase) requireTeacher(teacherID string) error {
	user, err := uc.users.GetByID(teacherID)
	if err != nil {
		return err
	}
	if user.Role != domain.RoleTeacher {
		return domain.ErrUserNotFound
	}
	return nil
}

// requireEnrolledWithTeacher fails with domain.ErrForbidden unless studentID
// is enrolled in at least one course owned by teacherID.
func (uc *RatingUseCase) requireEnrolledWithTeacher(ctx context.Context, studentID, teacherID string) error {
	courses, err := uc.entitlements.GetEnrolledCourses(ctx, studentID)
	if err != nil {
		return fmt.Errorf("list enrolled courses: %w", err)
	}
	for _, c := range courses {
		if c.TeacherID == teacherID {
			return nil
		}
	}
	return domain.ErrForbidden
}

// RateTeacher records studentID's 1-10 score for teacherID. studentID must
// be enrolled in at least one course taught by teacherID.
func (uc *RatingUseCase) RateTeacher(ctx context.Context, studentID, teacherID string, score int) (domain.TeacherRating, error) {
	if err := validateScore(score); err != nil {
		return domain.TeacherRating{}, err
	}
	if err := uc.requireTeacher(teacherID); err != nil {
		return domain.TeacherRating{}, err
	}
	if err := uc.requireEnrolledWithTeacher(ctx, studentID, teacherID); err != nil {
		return domain.TeacherRating{}, err
	}
	rating, err := uc.teacherRatings.Upsert(ctx, studentID, teacherID, score)
	if err != nil {
		return domain.TeacherRating{}, fmt.Errorf("rate teacher: %w", err)
	}
	return rating, nil
}

// TeacherRatingSummary returns the aggregate 1-10 rating for a teacher.
func (uc *RatingUseCase) TeacherRatingSummary(ctx context.Context, teacherID string) (domain.RatingSummary, error) {
	if err := uc.requireTeacher(teacherID); err != nil {
		return domain.RatingSummary{}, err
	}
	return uc.teacherRatings.Summary(ctx, teacherID)
}

// MyTeacherRating returns studentID's own rating for teacherID, or
// domain.ErrRatingNotFound if they haven't rated them yet.
func (uc *RatingUseCase) MyTeacherRating(ctx context.Context, studentID, teacherID string) (domain.TeacherRating, error) {
	if err := uc.requireTeacher(teacherID); err != nil {
		return domain.TeacherRating{}, err
	}
	return uc.teacherRatings.GetByStudent(ctx, studentID, teacherID)
}
