package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

// ratingStubCourseRepo implements domain.CourseRatingRepository for rating handler tests.
type ratingStubCourseRepo struct {
	summary domain.RatingSummary
	mine    domain.CourseRating
	mineErr error
}

func (s *ratingStubCourseRepo) Upsert(_ context.Context, studentID, courseID string, score int) (domain.CourseRating, error) {
	return domain.CourseRating{ID: "cr-1", StudentID: studentID, CourseID: courseID, Score: score}, nil
}

func (s *ratingStubCourseRepo) Summary(_ context.Context, _ string) (domain.RatingSummary, error) {
	return s.summary, nil
}

func (s *ratingStubCourseRepo) GetByStudent(_ context.Context, _, _ string) (domain.CourseRating, error) {
	return s.mine, s.mineErr
}

// ratingStubTeacherRepo implements domain.TeacherRatingRepository for rating handler tests.
type ratingStubTeacherRepo struct {
	summary domain.RatingSummary
	mine    domain.TeacherRating
	mineErr error
}

func (s *ratingStubTeacherRepo) Upsert(_ context.Context, studentID, teacherID string, score int) (domain.TeacherRating, error) {
	return domain.TeacherRating{ID: "tr-1", StudentID: studentID, TeacherID: teacherID, Score: score}, nil
}

func (s *ratingStubTeacherRepo) Summary(_ context.Context, _ string) (domain.RatingSummary, error) {
	return s.summary, nil
}

func (s *ratingStubTeacherRepo) GetByStudent(_ context.Context, _, _ string) (domain.TeacherRating, error) {
	return s.mine, s.mineErr
}

func newRatingHandler(
	courseRatings *ratingStubCourseRepo,
	teacherRatings *ratingStubTeacherRepo,
	cat *paymentStubCatRepo,
	ent *paymentStubEntRepo,
	usr *paymentStubUserRepo,
) *RatingHandler {
	uc := usecase.NewRatingUseCase(courseRatings, teacherRatings, cat, ent, usr)
	return NewRatingHandler(uc)
}

func ratingRequest(method, body string, claims domain.Claims) *http.Request {
	r := httptest.NewRequest(method, "http://x/", strings.NewReader(body))
	r = r.WithContext(context.WithValue(r.Context(), claimsContextKey, claims))
	r.SetPathValue(keyCourseID, testCourseID)
	r.SetPathValue("teacher_id", testTeacherID)
	return r
}

// --- RateCourse ---

func TestRateCourseHandler_RejectsNonStudent(t *testing.T) {
	h := newRatingHandler(&ratingStubCourseRepo{}, &ratingStubTeacherRepo{}, &paymentStubCatRepo{}, &paymentStubEntRepo{}, &paymentStubUserRepo{})

	w := httptest.NewRecorder()
	r := ratingRequest(http.MethodPost, `{"score":8}`, domain.Claims{UserID: "teach-1", Role: domain.RoleTeacher})
	h.RateCourse(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403; body=%s", w.Code, w.Body.String())
	}
}

func TestRateCourseHandler_NotEnrolledForbidden(t *testing.T) {
	ent := &paymentStubEntRepo{hasActiveGrant: false}
	h := newRatingHandler(&ratingStubCourseRepo{}, &ratingStubTeacherRepo{}, &paymentStubCatRepo{}, ent, &paymentStubUserRepo{})

	w := httptest.NewRecorder()
	r := ratingRequest(http.MethodPost, `{"score":8}`, domain.Claims{UserID: "stu-1", Role: domain.RoleStudent})
	h.RateCourse(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403; body=%s", w.Code, w.Body.String())
	}
}

func TestRateCourseHandler_InvalidScoreRejected(t *testing.T) {
	ent := &paymentStubEntRepo{hasActiveGrant: true}
	h := newRatingHandler(&ratingStubCourseRepo{}, &ratingStubTeacherRepo{}, &paymentStubCatRepo{}, ent, &paymentStubUserRepo{})

	w := httptest.NewRecorder()
	r := ratingRequest(http.MethodPost, `{"score":11}`, domain.Claims{UserID: "stu-1", Role: domain.RoleStudent})
	h.RateCourse(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestRateCourseHandler_Success(t *testing.T) {
	ent := &paymentStubEntRepo{hasActiveGrant: true}
	h := newRatingHandler(&ratingStubCourseRepo{}, &ratingStubTeacherRepo{}, &paymentStubCatRepo{}, ent, &paymentStubUserRepo{})

	w := httptest.NewRecorder()
	r := ratingRequest(http.MethodPost, `{"score":8}`, domain.Claims{UserID: "stu-1", Role: domain.RoleStudent})
	h.RateCourse(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	var resp courseRatingRecordResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Score != 8 || resp.CourseID != testCourseID || resp.StudentID != "stu-1" {
		t.Errorf("resp = %+v, want score=8 course_id=%s student_id=stu-1", resp, testCourseID)
	}
}

func TestCourseRatingSummaryHandler_UnknownCourseNotFound(t *testing.T) {
	cat := &paymentStubCatRepo{courseErr: domain.ErrCourseNotFound}
	h := newRatingHandler(&ratingStubCourseRepo{}, &ratingStubTeacherRepo{}, cat, &paymentStubEntRepo{}, &paymentStubUserRepo{})

	w := httptest.NewRecorder()
	r := ratingRequest(http.MethodGet, "", domain.Claims{})
	h.CourseRatingSummary(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestCourseRatingSummaryHandler_ReturnsAggregate(t *testing.T) {
	cat := &paymentStubCatRepo{course: domain.Course{ID: testCourseID}}
	courseRatings := &ratingStubCourseRepo{summary: domain.RatingSummary{AverageScore: 7.5, RatingsCount: 4}}
	h := newRatingHandler(courseRatings, &ratingStubTeacherRepo{}, cat, &paymentStubEntRepo{}, &paymentStubUserRepo{})

	w := httptest.NewRecorder()
	r := ratingRequest(http.MethodGet, "", domain.Claims{})
	h.CourseRatingSummary(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp ratingSummaryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.AverageScore != 7.5 || resp.RatingsCount != 4 {
		t.Errorf("resp = %+v, want {7.5 4}", resp)
	}
}

// --- RateTeacher ---

func TestRateTeacherHandler_RejectsNonStudent(t *testing.T) {
	h := newRatingHandler(&ratingStubCourseRepo{}, &ratingStubTeacherRepo{}, &paymentStubCatRepo{}, &paymentStubEntRepo{}, &paymentStubUserRepo{})

	w := httptest.NewRecorder()
	r := ratingRequest(http.MethodPost, `{"score":8}`, domain.Claims{UserID: "teach-1", Role: domain.RoleTeacher})
	h.RateTeacher(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403; body=%s", w.Code, w.Body.String())
	}
}

func TestRateTeacherHandler_NotEnrolledForbidden(t *testing.T) {
	usr := &paymentStubUserRepo{user: domain.User{ID: testTeacherID, Role: domain.RoleTeacher}}
	ent := &paymentStubEntRepo{enrolledCourses: []domain.Course{{ID: testCourseID, TeacherID: "someone-else"}}}
	h := newRatingHandler(&ratingStubCourseRepo{}, &ratingStubTeacherRepo{}, &paymentStubCatRepo{}, ent, usr)

	w := httptest.NewRecorder()
	r := ratingRequest(http.MethodPost, `{"score":8}`, domain.Claims{UserID: "stu-1", Role: domain.RoleStudent})
	h.RateTeacher(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403; body=%s", w.Code, w.Body.String())
	}
}

func TestRateTeacherHandler_NonTeacherNotFound(t *testing.T) {
	usr := &paymentStubUserRepo{user: domain.User{ID: testTeacherID, Role: domain.RoleStudent}}
	h := newRatingHandler(&ratingStubCourseRepo{}, &ratingStubTeacherRepo{}, &paymentStubCatRepo{}, &paymentStubEntRepo{}, usr)

	w := httptest.NewRecorder()
	r := ratingRequest(http.MethodPost, `{"score":8}`, domain.Claims{UserID: "stu-1", Role: domain.RoleStudent})
	h.RateTeacher(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestRateTeacherHandler_Success(t *testing.T) {
	usr := &paymentStubUserRepo{user: domain.User{ID: testTeacherID, Role: domain.RoleTeacher}}
	ent := &paymentStubEntRepo{enrolledCourses: []domain.Course{{ID: testCourseID, TeacherID: testTeacherID}}}
	h := newRatingHandler(&ratingStubCourseRepo{}, &ratingStubTeacherRepo{}, &paymentStubCatRepo{}, ent, usr)

	w := httptest.NewRecorder()
	r := ratingRequest(http.MethodPost, `{"score":9}`, domain.Claims{UserID: "stu-1", Role: domain.RoleStudent})
	h.RateTeacher(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	var resp teacherRatingResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Score != 9 || resp.TeacherID != testTeacherID {
		t.Errorf("resp = %+v, want score=9 teacher_id=%s", resp, testTeacherID)
	}
}

func TestTeacherRatingSummaryHandler_ReturnsAggregate(t *testing.T) {
	usr := &paymentStubUserRepo{user: domain.User{ID: testTeacherID, Role: domain.RoleTeacher}}
	teacherRatings := &ratingStubTeacherRepo{summary: domain.RatingSummary{AverageScore: 6, RatingsCount: 2}}
	h := newRatingHandler(&ratingStubCourseRepo{}, teacherRatings, &paymentStubCatRepo{}, &paymentStubEntRepo{}, usr)

	w := httptest.NewRecorder()
	r := ratingRequest(http.MethodGet, "", domain.Claims{})
	h.TeacherRatingSummary(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp ratingSummaryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.AverageScore != 6 || resp.RatingsCount != 2 {
		t.Errorf("resp = %+v, want {6 2}", resp)
	}
}

// --- MyCourseRating / MyTeacherRating ---

func TestMyCourseRatingHandler_RejectsNonStudent(t *testing.T) {
	h := newRatingHandler(&ratingStubCourseRepo{}, &ratingStubTeacherRepo{}, &paymentStubCatRepo{}, &paymentStubEntRepo{}, &paymentStubUserRepo{})

	w := httptest.NewRecorder()
	r := ratingRequest(http.MethodGet, "", domain.Claims{UserID: "teach-1", Role: domain.RoleTeacher})
	h.MyCourseRating(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403; body=%s", w.Code, w.Body.String())
	}
}

func TestMyCourseRatingHandler_NotRatedYet(t *testing.T) {
	cat := &paymentStubCatRepo{course: domain.Course{ID: testCourseID}}
	courseRatings := &ratingStubCourseRepo{mineErr: domain.ErrRatingNotFound}
	h := newRatingHandler(courseRatings, &ratingStubTeacherRepo{}, cat, &paymentStubEntRepo{}, &paymentStubUserRepo{})

	w := httptest.NewRecorder()
	r := ratingRequest(http.MethodGet, "", domain.Claims{UserID: "stu-1", Role: domain.RoleStudent})
	h.MyCourseRating(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestMyCourseRatingHandler_ReturnsOwnRating(t *testing.T) {
	cat := &paymentStubCatRepo{course: domain.Course{ID: testCourseID}}
	courseRatings := &ratingStubCourseRepo{mine: domain.CourseRating{ID: "cr-1", StudentID: "stu-1", CourseID: testCourseID, Score: 7}}
	h := newRatingHandler(courseRatings, &ratingStubTeacherRepo{}, cat, &paymentStubEntRepo{}, &paymentStubUserRepo{})

	w := httptest.NewRecorder()
	r := ratingRequest(http.MethodGet, "", domain.Claims{UserID: "stu-1", Role: domain.RoleStudent})
	h.MyCourseRating(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp courseRatingRecordResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Score != 7 {
		t.Errorf("resp = %+v, want score=7", resp)
	}
}

func TestMyTeacherRatingHandler_RejectsNonStudent(t *testing.T) {
	h := newRatingHandler(&ratingStubCourseRepo{}, &ratingStubTeacherRepo{}, &paymentStubCatRepo{}, &paymentStubEntRepo{}, &paymentStubUserRepo{})

	w := httptest.NewRecorder()
	r := ratingRequest(http.MethodGet, "", domain.Claims{UserID: "teach-1", Role: domain.RoleTeacher})
	h.MyTeacherRating(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403; body=%s", w.Code, w.Body.String())
	}
}

func TestMyTeacherRatingHandler_NotRatedYet(t *testing.T) {
	usr := &paymentStubUserRepo{user: domain.User{ID: testTeacherID, Role: domain.RoleTeacher}}
	teacherRatings := &ratingStubTeacherRepo{mineErr: domain.ErrRatingNotFound}
	h := newRatingHandler(&ratingStubCourseRepo{}, teacherRatings, &paymentStubCatRepo{}, &paymentStubEntRepo{}, usr)

	w := httptest.NewRecorder()
	r := ratingRequest(http.MethodGet, "", domain.Claims{UserID: "stu-1", Role: domain.RoleStudent})
	h.MyTeacherRating(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestMyTeacherRatingHandler_ReturnsOwnRating(t *testing.T) {
	usr := &paymentStubUserRepo{user: domain.User{ID: testTeacherID, Role: domain.RoleTeacher}}
	teacherRatings := &ratingStubTeacherRepo{mine: domain.TeacherRating{ID: "tr-1", StudentID: "stu-1", TeacherID: testTeacherID, Score: 9}}
	h := newRatingHandler(&ratingStubCourseRepo{}, teacherRatings, &paymentStubCatRepo{}, &paymentStubEntRepo{}, usr)

	w := httptest.NewRecorder()
	r := ratingRequest(http.MethodGet, "", domain.Claims{UserID: "stu-1", Role: domain.RoleStudent})
	h.MyTeacherRating(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp teacherRatingResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Score != 9 {
		t.Errorf("resp = %+v, want score=9", resp)
	}
}
