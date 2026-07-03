package e2e

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/student-learning-portal/backend/internal/domain"
)

type dashboard struct {
	AtRiskStudents int `json:"at_risk_students"`
	Students       []struct {
		StudentID          string  `json:"student_id"`
		ProgressPercentage float64 `json:"progress_percentage"`
		Status             string  `json:"status"`
		FullName           string  `json:"full_name"`
		DaysInactive       int     `json:"days_inactive"`
	} `json:"students"`
}

func TestAnalytics_RequiresTeacherRole(t *testing.T) {
	e := newTestEnv(t)
	teacher, _ := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	_, studentTok := e.register("student@example.com", "Student", domain.RoleStudent)
	courseID := e.insertCourse(teacher, "Go", "Programming", 10, "published")

	// A student token is rejected with 403 before any course lookup.
	resp := e.do(http.MethodGet, "/api/v1/analytics/teacher/dashboard?course_id="+courseID, studentTok, nil)
	e.requireStatus(resp, http.StatusForbidden)
	if msg := e.errorMessage(resp); msg != "teacher role required" {
		t.Errorf("error = %q, want 'teacher role required'", msg)
	}
}

func TestAnalytics_TeacherCannotViewAnothersCourse(t *testing.T) {
	e := newTestEnv(t)
	owner, _ := e.register("owner@example.com", "Owner", domain.RoleTeacher)
	_, otherTok := e.register("other@example.com", "Other Teacher", domain.RoleTeacher)
	courseID := e.insertCourse(owner, "Go", "Programming", 10, "published")

	resp := e.do(http.MethodGet, "/api/v1/analytics/teacher/dashboard?course_id="+courseID, otherTok, nil)
	e.requireStatus(resp, http.StatusForbidden)
	if msg := e.errorMessage(resp); msg != "you do not own this course" {
		t.Errorf("error = %q, want 'you do not own this course'", msg)
	}
}

func TestAnalytics_MissingCourseIDIsBadRequest(t *testing.T) {
	e := newTestEnv(t)
	_, teacherTok := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	resp := e.do(http.MethodGet, "/api/v1/analytics/teacher/dashboard", teacherTok, nil)
	e.requireStatus(resp, http.StatusBadRequest)
}

func TestAnalytics_UnknownCourseIsNotFound(t *testing.T) {
	e := newTestEnv(t)
	_, teacherTok := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	resp := e.do(http.MethodGet, "/api/v1/analytics/teacher/dashboard?course_id="+uuid.NewString(), teacherTok, nil)
	e.requireStatus(resp, http.StatusNotFound)
}

func TestAnalytics_OwnerWithNoRollupGetsEmptyDashboard(t *testing.T) {
	e := newTestEnv(t)
	teacherID, teacherTok := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	courseID := e.insertCourse(teacherID, "Go", "Programming", 10, "published")

	resp := e.do(http.MethodGet, "/api/v1/analytics/teacher/dashboard?course_id="+courseID, teacherTok, nil)
	e.requireStatus(resp, http.StatusOK)
	var d dashboard
	e.decode(resp, &d)
	if d.AtRiskStudents != 0 || len(d.Students) != 0 {
		t.Fatalf("empty-rollup dashboard = %+v, want 0 at-risk and [] students", d)
	}
}

func TestAnalytics_ClassifiesAndOrdersStudents(t *testing.T) {
	e := newTestEnv(t)
	teacherID, teacherTok := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	courseID := e.insertCourse(teacherID, "Go", "Programming", 10, "published")

	// Two enrolled students with rollup standings; full_name comes from users.
	struggling, _ := e.register("struggle@example.com", "Sam Struggle", domain.RoleStudent)
	onTrack, _ := e.register("ace@example.com", "Ada Ace", domain.RoleStudent)
	now := time.Now()
	e.insertRollup(courseID, struggling, 20.0, 1, 5, &now) // progress < 40 → AT_RISK
	e.insertRollup(courseID, onTrack, 90.0, 9, 10, &now)   // healthy → ON_TRACK

	resp := e.do(http.MethodGet, "/api/v1/analytics/teacher/dashboard?course_id="+courseID, teacherTok, nil)
	e.requireStatus(resp, http.StatusOK)
	var d dashboard
	e.decode(resp, &d)

	if d.AtRiskStudents != 1 {
		t.Errorf("at_risk_students = %d, want 1", d.AtRiskStudents)
	}
	if len(d.Students) != 2 {
		t.Fatalf("students = %d, want 2", len(d.Students))
	}
	// Ordered worst-first (progress_percent ASC): struggling student leads.
	if d.Students[0].StudentID != struggling || d.Students[0].Status != domain.RiskAtRisk {
		t.Errorf("students[0] = %+v, want %s / AT_RISK", d.Students[0], struggling)
	}
	if d.Students[0].FullName != "Sam Struggle" {
		t.Errorf("full_name = %q, want 'Sam Struggle'", d.Students[0].FullName)
	}
	if d.Students[1].StudentID != onTrack || d.Students[1].Status != domain.RiskOnTrack {
		t.Errorf("students[1] = %+v, want %s / ON_TRACK", d.Students[1], onTrack)
	}
}

type studentDashboard struct {
	OverallProgress  float64 `json:"overall_progress"`
	CoursesCompleted int     `json:"courses_completed"`
	Courses          []struct {
		CourseID         string  `json:"course_id"`
		CourseTitle      string  `json:"course_title"`
		ProgressPercent  float64 `json:"progress_percentage"`
		LessonsCompleted int     `json:"lessons_completed"`
		LessonsTotal     int     `json:"lessons_total"`
		Status           string  `json:"status"`
		DaysInactive     int     `json:"days_inactive"`
	} `json:"courses"`
}

func TestStudentDashboard_RequiresStudentRole(t *testing.T) {
	e := newTestEnv(t)
	_, teacherTok := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)

	resp := e.do(http.MethodGet, "/api/v1/analytics/student/me", teacherTok, nil)
	e.requireStatus(resp, http.StatusForbidden)
	if msg := e.errorMessage(resp); msg != "student role required" {
		t.Errorf("error = %q, want 'student role required'", msg)
	}
}

func TestStudentDashboard_NoEnrollmentsIsEmpty(t *testing.T) {
	e := newTestEnv(t)
	_, studentTok := e.register("student@example.com", "Student", domain.RoleStudent)

	resp := e.do(http.MethodGet, "/api/v1/analytics/student/me", studentTok, nil)
	e.requireStatus(resp, http.StatusOK)
	var d studentDashboard
	e.decode(resp, &d)
	if d.OverallProgress != 0 || d.CoursesCompleted != 0 || len(d.Courses) != 0 {
		t.Fatalf("no-enrollment dashboard = %+v, want zero values", d)
	}
}

func TestStudentDashboard_AggregatesAcrossOwnCoursesOnly(t *testing.T) {
	e := newTestEnv(t)
	teacherID, _ := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	studentID, studentTok := e.register("student@example.com", "Student", domain.RoleStudent)
	otherID, _ := e.register("other@example.com", "Other Student", domain.RoleStudent)

	courseA := e.insertCourse(teacherID, "Go", "Programming", 10, "published")
	courseB := e.insertCourse(teacherID, "SQL", "Databases", 10, "published")
	now := time.Now()
	stale := now.Add(-30 * 24 * time.Hour)

	e.insertRollup(courseA, studentID, 100.0, 5, 5, &now)  // completed, on track
	e.insertRollup(courseB, studentID, 80.0, 4, 5, &stale) // at risk: inactivity
	e.insertRollup(courseA, otherID, 10.0, 1, 5, &now)     // another learner's row, must not leak in

	resp := e.do(http.MethodGet, "/api/v1/analytics/student/me", studentTok, nil)
	e.requireStatus(resp, http.StatusOK)
	var d studentDashboard
	e.decode(resp, &d)

	if d.CoursesCompleted != 1 {
		t.Errorf("courses_completed = %d, want 1", d.CoursesCompleted)
	}
	if d.OverallProgress != 90.0 {
		t.Errorf("overall_progress = %v, want 90", d.OverallProgress)
	}
	if len(d.Courses) != 2 {
		t.Fatalf("courses = %d, want 2", len(d.Courses))
	}
	byID := map[string]string{d.Courses[0].CourseID: d.Courses[0].Status, d.Courses[1].CourseID: d.Courses[1].Status}
	if byID[courseB] != domain.RiskAtRisk {
		t.Errorf("courseB status = %q, want AT_RISK", byID[courseB])
	}
	if byID[courseA] != domain.RiskOnTrack {
		t.Errorf("courseA status = %q, want ON_TRACK", byID[courseA])
	}
}

// TestStudentDashboard_UpdatesImmediatelyAfterProgressSave proves the point
// rollup refresh (usecase.RollupRefreshSink): saving progress through the real
// player endpoint must be visible on /analytics/student/me right away, with no
// analytics-loader run in between.
func TestStudentDashboard_UpdatesImmediatelyAfterProgressSave(t *testing.T) {
	e := newTestEnv(t)
	teacherID, _ := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	_, studentTok := e.register("student@example.com", "Student", domain.RoleStudent)
	courseID := e.insertCourse(teacherID, "Go", "Programming", 10, "published")
	lessonID := e.insertLesson(courseID, "Intro", "video", 1)
	e.insertMedia(lessonID, 200_000) // 200s

	e.grantAccess(studentTok, courseID)

	// Freshly enrolled, no progress yet: no rollup row exists at all (the
	// loader has never run, and no progress event has fired to create one).
	before := e.do(http.MethodGet, "/api/v1/analytics/student/me", studentTok, nil)
	e.requireStatus(before, http.StatusOK)
	var d0 studentDashboard
	e.decode(before, &d0)
	if len(d0.Courses) != 0 {
		t.Fatalf("before progress: courses = %+v, want none", d0.Courses)
	}

	// Save progress at 100s (50% of 200s) — still no loader run.
	save := e.do(http.MethodPost, "/api/v1/player/courses/"+courseID+"/lessons/"+lessonID+"/progress",
		studentTok, map[string]any{"progress_seconds": 100})
	e.requireStatus(save, http.StatusOK)

	after := e.do(http.MethodGet, "/api/v1/analytics/student/me", studentTok, nil)
	e.requireStatus(after, http.StatusOK)
	var d1 studentDashboard
	e.decode(after, &d1)
	if len(d1.Courses) != 1 {
		t.Fatalf("after progress: courses = %d, want 1 (point refresh should have created the row)", len(d1.Courses))
	}
	got := d1.Courses[0]
	if got.CourseID != courseID {
		t.Errorf("course_id = %q, want %q", got.CourseID, courseID)
	}
	if got.ProgressPercent != 50 {
		t.Errorf("progress_percentage = %v, want 50", got.ProgressPercent)
	}
	if got.LessonsTotal != 1 {
		t.Errorf("lessons_total = %d, want 1", got.LessonsTotal)
	}
	if d1.OverallProgress != 50 {
		t.Errorf("overall_progress = %v, want 50", d1.OverallProgress)
	}
}
