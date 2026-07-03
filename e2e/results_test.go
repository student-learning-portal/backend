package e2e

import (
	"net/http"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
)

type studentResults struct {
	OverallProgressPercent float64 `json:"overall_progress_percent"`
	CoursesEnrolled        int     `json:"courses_enrolled"`
	CoursesCompleted       int     `json:"courses_completed"`
	Courses                []struct {
		CourseID         string  `json:"course_id"`
		Title            string  `json:"title"`
		LessonsTotal     int     `json:"lessons_total"`
		LessonsCompleted int     `json:"lessons_completed"`
		ProgressPercent  float64 `json:"progress_percent"`
	} `json:"courses"`
}

// completeLesson marks a lesson finished for the token's owner (percent 100).
func (e *testEnv) completeLesson(token, courseID, lessonID string) {
	e.t.Helper()
	resp := e.do(http.MethodPost,
		"/api/v1/player/courses/"+courseID+"/lessons/"+lessonID+"/progress",
		token, map[string]any{"progress_seconds": 60, "completed": true})
	e.requireStatus(resp, http.StatusOK)
}

func TestResults_AggregatesProgressAcrossEnrolledCourses(t *testing.T) {
	e := newTestEnv(t)
	teacher, _ := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	_, studentTok := e.register("student@example.com", "Student", domain.RoleStudent)

	// Alpha: 2 lessons, student completes 1 -> 50%.
	alpha := e.insertCourse(teacher, "Alpha", "Programming", 10, "published")
	a1 := e.insertLesson(alpha, "A1", "video", 1)
	e.insertLesson(alpha, "A2", "video", 2)
	e.insertMedia(a1, 60000)

	// Beta: 1 lesson, student completes it -> 100%.
	beta := e.insertCourse(teacher, "Beta", "Programming", 10, "published")
	b1 := e.insertLesson(beta, "B1", "video", 1)
	e.insertMedia(b1, 60000)

	e.grantAccess(studentTok, alpha)
	e.grantAccess(studentTok, beta)
	e.completeLesson(studentTok, alpha, a1)
	e.completeLesson(studentTok, beta, b1)

	resp := e.do(http.MethodGet, "/api/v1/users/me/results", studentTok, nil)
	e.requireStatus(resp, http.StatusOK)
	var res studentResults
	e.decode(resp, &res)

	if res.CoursesEnrolled != 2 {
		t.Fatalf("courses_enrolled = %d, want 2", res.CoursesEnrolled)
	}
	if res.CoursesCompleted != 1 { // only Beta is fully done
		t.Errorf("courses_completed = %d, want 1", res.CoursesCompleted)
	}
	// Ordered by title: Alpha then Beta.
	if len(res.Courses) != 2 {
		t.Fatalf("courses len = %d, want 2", len(res.Courses))
	}
	if res.Courses[0].Title != "Alpha" || res.Courses[0].LessonsTotal != 2 ||
		res.Courses[0].LessonsCompleted != 1 || res.Courses[0].ProgressPercent != 50 {
		t.Errorf("Alpha = %+v, want total 2 / completed 1 / 50%%", res.Courses[0])
	}
	if res.Courses[1].Title != "Beta" || res.Courses[1].ProgressPercent != 100 {
		t.Errorf("Beta = %+v, want 100%%", res.Courses[1])
	}
	// Overall is lesson-weighted: (1 + 1) / (2 + 1) = 66.67%.
	if res.OverallProgressPercent != 66.67 {
		t.Errorf("overall = %v, want 66.67", res.OverallProgressPercent)
	}
}

func TestResults_NoEnrollmentsIsEmpty(t *testing.T) {
	e := newTestEnv(t)
	_, studentTok := e.register("student@example.com", "Student", domain.RoleStudent)

	resp := e.do(http.MethodGet, "/api/v1/users/me/results", studentTok, nil)
	e.requireStatus(resp, http.StatusOK)
	var res studentResults
	e.decode(resp, &res)

	if res.CoursesEnrolled != 0 || res.CoursesCompleted != 0 || res.OverallProgressPercent != 0 {
		t.Errorf("empty results = %+v, want all zero", res)
	}
	if len(res.Courses) != 0 {
		t.Errorf("courses = %d, want 0", len(res.Courses))
	}
}

func TestResults_ExcludesRefundedCourse(t *testing.T) {
	e := newTestEnv(t)
	teacher, _ := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	_, studentTok := e.register("student@example.com", "Student", domain.RoleStudent)
	courseID := e.insertCourse(teacher, "Go", "Programming", 10, "published")
	e.insertLesson(courseID, "Intro", "video", 1)

	e.grantAccess(studentTok, courseID)
	// Refund revokes the grant, so the course drops out of results.
	e.requireStatus(
		e.do(http.MethodPost, "/api/v1/purchase/refund", studentTok, courseIDBody{CourseID: courseID}),
		http.StatusOK,
	)

	resp := e.do(http.MethodGet, "/api/v1/users/me/results", studentTok, nil)
	e.requireStatus(resp, http.StatusOK)
	var res studentResults
	e.decode(resp, &res)
	if res.CoursesEnrolled != 0 {
		t.Errorf("courses_enrolled = %d, want 0 (grant revoked)", res.CoursesEnrolled)
	}
}

func TestResults_RequiresAuth(t *testing.T) {
	e := newTestEnv(t)
	resp := e.do(http.MethodGet, "/api/v1/users/me/results", "", nil)
	e.requireStatus(resp, http.StatusUnauthorized)
}
