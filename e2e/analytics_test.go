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
