package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
)

type stubCourseRatingRepo struct {
	upserted   domain.CourseRating
	upsertErr  error
	summary    domain.RatingSummary
	summaryErr error
	mine       domain.CourseRating
	mineErr    error
}

func (s *stubCourseRatingRepo) Upsert(_ context.Context, studentID, courseID string, score int) (domain.CourseRating, error) {
	if s.upsertErr != nil {
		return domain.CourseRating{}, s.upsertErr
	}
	return domain.CourseRating{ID: "cr-1", StudentID: studentID, CourseID: courseID, Score: score}, nil
}

func (s *stubCourseRatingRepo) Summary(_ context.Context, _ string) (domain.RatingSummary, error) {
	return s.summary, s.summaryErr
}

func (s *stubCourseRatingRepo) GetByStudent(_ context.Context, _, _ string) (domain.CourseRating, error) {
	return s.mine, s.mineErr
}

type stubTeacherRatingRepo struct {
	upsertErr  error
	summary    domain.RatingSummary
	summaryErr error
	mine       domain.TeacherRating
	mineErr    error
}

func (s *stubTeacherRatingRepo) Upsert(_ context.Context, studentID, teacherID string, score int) (domain.TeacherRating, error) {
	if s.upsertErr != nil {
		return domain.TeacherRating{}, s.upsertErr
	}
	return domain.TeacherRating{ID: "tr-1", StudentID: studentID, TeacherID: teacherID, Score: score}, nil
}

func (s *stubTeacherRatingRepo) Summary(_ context.Context, _ string) (domain.RatingSummary, error) {
	return s.summary, s.summaryErr
}

func (s *stubTeacherRatingRepo) GetByStudent(_ context.Context, _, _ string) (domain.TeacherRating, error) {
	return s.mine, s.mineErr
}

func newRatingUC(
	courseRatings *stubCourseRatingRepo,
	teacherRatings *stubTeacherRatingRepo,
	cat *stubCatalogRepository,
	ent *stubEntitlementRepo,
	usr *stubPaymentUserRepo,
) *RatingUseCase {
	return NewRatingUseCase(courseRatings, teacherRatings, cat, ent, usr)
}

// --- RateCourse ---

func TestRateCourse_EnrolledStoresScore(t *testing.T) {
	uc := newRatingUC(&stubCourseRatingRepo{}, &stubTeacherRatingRepo{}, &stubCatalogRepository{}, &stubEntitlementRepo{hasActiveGrant: true}, &stubPaymentUserRepo{})

	rating, err := uc.RateCourse(context.Background(), "stu-1", "course-1", 8)
	if err != nil {
		t.Fatalf("RateCourse: %v", err)
	}
	if rating.Score != 8 || rating.StudentID != "stu-1" || rating.CourseID != "course-1" {
		t.Errorf("rating = %+v, want score=8 student=stu-1 course=course-1", rating)
	}
}

func TestRateCourse_NotEnrolledForbidden(t *testing.T) {
	uc := newRatingUC(&stubCourseRatingRepo{}, &stubTeacherRatingRepo{}, &stubCatalogRepository{}, &stubEntitlementRepo{hasActiveGrant: false}, &stubPaymentUserRepo{})

	_, err := uc.RateCourse(context.Background(), "stu-1", "course-1", 8)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("err = %v, want ErrForbidden", err)
	}
}

func TestRateCourse_InvalidScoreRejected(t *testing.T) {
	uc := newRatingUC(&stubCourseRatingRepo{}, &stubTeacherRatingRepo{}, &stubCatalogRepository{}, &stubEntitlementRepo{hasActiveGrant: true}, &stubPaymentUserRepo{})

	for _, score := range []int{0, -1, 11, 100} {
		_, err := uc.RateCourse(context.Background(), "stu-1", "course-1", score)
		if !errors.Is(err, domain.ErrInvalidRating) {
			t.Errorf("score=%d: err = %v, want ErrInvalidRating", score, err)
		}
	}
}

func TestCourseRatingSummary_UnknownCourseNotFound(t *testing.T) {
	cat := &stubCatalogRepository{courseErr: domain.ErrCourseNotFound}
	uc := newRatingUC(&stubCourseRatingRepo{}, &stubTeacherRatingRepo{}, cat, &stubEntitlementRepo{}, &stubPaymentUserRepo{})

	_, err := uc.CourseRatingSummary(context.Background(), "missing")
	if !errors.Is(err, domain.ErrCourseNotFound) {
		t.Errorf("err = %v, want ErrCourseNotFound", err)
	}
}

func TestCourseRatingSummary_ReturnsAggregate(t *testing.T) {
	courseRatings := &stubCourseRatingRepo{summary: domain.RatingSummary{AverageScore: 7.5, RatingsCount: 4}}
	uc := newRatingUC(courseRatings, &stubTeacherRatingRepo{}, &stubCatalogRepository{course: domain.Course{ID: "course-1"}}, &stubEntitlementRepo{}, &stubPaymentUserRepo{})

	summary, err := uc.CourseRatingSummary(context.Background(), "course-1")
	if err != nil {
		t.Fatalf("CourseRatingSummary: %v", err)
	}
	if summary.AverageScore != 7.5 || summary.RatingsCount != 4 {
		t.Errorf("summary = %+v, want {7.5 4}", summary)
	}
}

// --- RateTeacher ---

func TestRateTeacher_EnrolledInTheirCourseStoresScore(t *testing.T) {
	usr := &stubPaymentUserRepo{user: domain.User{ID: "teach-1", Role: domain.RoleTeacher}}
	ent := &stubEntitlementRepo{enrolledCourses: []domain.Course{{ID: "course-1", TeacherID: "teach-1"}}}
	uc := newRatingUC(&stubCourseRatingRepo{}, &stubTeacherRatingRepo{}, &stubCatalogRepository{}, ent, usr)

	rating, err := uc.RateTeacher(context.Background(), "stu-1", "teach-1", 9)
	if err != nil {
		t.Fatalf("RateTeacher: %v", err)
	}
	if rating.Score != 9 || rating.TeacherID != "teach-1" {
		t.Errorf("rating = %+v, want score=9 teacher=teach-1", rating)
	}
}

func TestRateTeacher_NotEnrolledInTheirCoursesForbidden(t *testing.T) {
	usr := &stubPaymentUserRepo{user: domain.User{ID: "teach-1", Role: domain.RoleTeacher}}
	ent := &stubEntitlementRepo{enrolledCourses: []domain.Course{{ID: "course-1", TeacherID: "someone-else"}}}
	uc := newRatingUC(&stubCourseRatingRepo{}, &stubTeacherRatingRepo{}, &stubCatalogRepository{}, ent, usr)

	_, err := uc.RateTeacher(context.Background(), "stu-1", "teach-1", 9)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("err = %v, want ErrForbidden", err)
	}
}

func TestRateTeacher_NonTeacherUserNotFound(t *testing.T) {
	usr := &stubPaymentUserRepo{user: domain.User{ID: "stu-2", Role: domain.RoleStudent}}
	uc := newRatingUC(&stubCourseRatingRepo{}, &stubTeacherRatingRepo{}, &stubCatalogRepository{}, &stubEntitlementRepo{}, usr)

	_, err := uc.RateTeacher(context.Background(), "stu-1", "stu-2", 5)
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("err = %v, want ErrUserNotFound", err)
	}
}

func TestRateTeacher_InvalidScoreRejected(t *testing.T) {
	usr := &stubPaymentUserRepo{user: domain.User{ID: "teach-1", Role: domain.RoleTeacher}}
	uc := newRatingUC(&stubCourseRatingRepo{}, &stubTeacherRatingRepo{}, &stubCatalogRepository{}, &stubEntitlementRepo{}, usr)

	_, err := uc.RateTeacher(context.Background(), "stu-1", "teach-1", 11)
	if !errors.Is(err, domain.ErrInvalidRating) {
		t.Errorf("err = %v, want ErrInvalidRating", err)
	}
}

func TestTeacherRatingSummary_ReturnsAggregate(t *testing.T) {
	usr := &stubPaymentUserRepo{user: domain.User{ID: "teach-1", Role: domain.RoleTeacher}}
	teacherRatings := &stubTeacherRatingRepo{summary: domain.RatingSummary{AverageScore: 6, RatingsCount: 2}}
	uc := newRatingUC(&stubCourseRatingRepo{}, teacherRatings, &stubCatalogRepository{}, &stubEntitlementRepo{}, usr)

	summary, err := uc.TeacherRatingSummary(context.Background(), "teach-1")
	if err != nil {
		t.Fatalf("TeacherRatingSummary: %v", err)
	}
	if summary.AverageScore != 6 || summary.RatingsCount != 2 {
		t.Errorf("summary = %+v, want {6 2}", summary)
	}
}

// --- MyCourseRating / MyTeacherRating ---

func TestMyCourseRating_UnknownCourseNotFound(t *testing.T) {
	cat := &stubCatalogRepository{courseErr: domain.ErrCourseNotFound}
	uc := newRatingUC(&stubCourseRatingRepo{}, &stubTeacherRatingRepo{}, cat, &stubEntitlementRepo{}, &stubPaymentUserRepo{})

	_, err := uc.MyCourseRating(context.Background(), "stu-1", "missing")
	if !errors.Is(err, domain.ErrCourseNotFound) {
		t.Errorf("err = %v, want ErrCourseNotFound", err)
	}
}

func TestMyCourseRating_NotRatedYet(t *testing.T) {
	courseRatings := &stubCourseRatingRepo{mineErr: domain.ErrRatingNotFound}
	cat := &stubCatalogRepository{course: domain.Course{ID: "course-1"}}
	uc := newRatingUC(courseRatings, &stubTeacherRatingRepo{}, cat, &stubEntitlementRepo{}, &stubPaymentUserRepo{})

	_, err := uc.MyCourseRating(context.Background(), "stu-1", "course-1")
	if !errors.Is(err, domain.ErrRatingNotFound) {
		t.Errorf("err = %v, want ErrRatingNotFound", err)
	}
}

func TestMyCourseRating_ReturnsOwnRating(t *testing.T) {
	courseRatings := &stubCourseRatingRepo{mine: domain.CourseRating{ID: "cr-1", StudentID: "stu-1", CourseID: "course-1", Score: 7}}
	cat := &stubCatalogRepository{course: domain.Course{ID: "course-1"}}
	uc := newRatingUC(courseRatings, &stubTeacherRatingRepo{}, cat, &stubEntitlementRepo{}, &stubPaymentUserRepo{})

	rating, err := uc.MyCourseRating(context.Background(), "stu-1", "course-1")
	if err != nil {
		t.Fatalf("MyCourseRating: %v", err)
	}
	if rating.Score != 7 {
		t.Errorf("rating = %+v, want score=7", rating)
	}
}

func TestMyTeacherRating_NonTeacherUserNotFound(t *testing.T) {
	usr := &stubPaymentUserRepo{user: domain.User{ID: "stu-2", Role: domain.RoleStudent}}
	uc := newRatingUC(&stubCourseRatingRepo{}, &stubTeacherRatingRepo{}, &stubCatalogRepository{}, &stubEntitlementRepo{}, usr)

	_, err := uc.MyTeacherRating(context.Background(), "stu-1", "stu-2")
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("err = %v, want ErrUserNotFound", err)
	}
}

func TestMyTeacherRating_NotRatedYet(t *testing.T) {
	usr := &stubPaymentUserRepo{user: domain.User{ID: "teach-1", Role: domain.RoleTeacher}}
	teacherRatings := &stubTeacherRatingRepo{mineErr: domain.ErrRatingNotFound}
	uc := newRatingUC(&stubCourseRatingRepo{}, teacherRatings, &stubCatalogRepository{}, &stubEntitlementRepo{}, usr)

	_, err := uc.MyTeacherRating(context.Background(), "stu-1", "teach-1")
	if !errors.Is(err, domain.ErrRatingNotFound) {
		t.Errorf("err = %v, want ErrRatingNotFound", err)
	}
}

func TestMyTeacherRating_ReturnsOwnRating(t *testing.T) {
	usr := &stubPaymentUserRepo{user: domain.User{ID: "teach-1", Role: domain.RoleTeacher}}
	teacherRatings := &stubTeacherRatingRepo{mine: domain.TeacherRating{ID: "tr-1", StudentID: "stu-1", TeacherID: "teach-1", Score: 9}}
	uc := newRatingUC(&stubCourseRatingRepo{}, teacherRatings, &stubCatalogRepository{}, &stubEntitlementRepo{}, usr)

	rating, err := uc.MyTeacherRating(context.Background(), "stu-1", "teach-1")
	if err != nil {
		t.Fatalf("MyTeacherRating: %v", err)
	}
	if rating.Score != 9 {
		t.Errorf("rating = %+v, want score=9", rating)
	}
}
