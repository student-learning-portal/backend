package e2e

import (
	"net/http"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
)

type lessonData struct {
	LessonID            string  `json:"lesson_id"`
	CourseID            string  `json:"course_id"`
	Title               string  `json:"title"`
	ContentURL          string  `json:"content_url"`
	DurationSeconds     int     `json:"duration_seconds"`
	LastProgressSeconds int     `json:"last_progress_seconds"`
	PercentComplete     float64 `json:"percent_complete"`
	Materials           []struct {
		Title string `json:"title"`
		URL   string `json:"url"`
		Type  string `json:"type"`
	} `json:"materials"`
}

type progressData struct {
	LessonID        string  `json:"lesson_id"`
	ProgressSeconds int     `json:"progress_seconds"`
	PercentComplete float64 `json:"percent_complete"`
	Completed       bool    `json:"completed"`
	UpdatedAt       string  `json:"updated_at"`
}

// grantAccess simulates a completed purchase by buying the course through the
// real checkout endpoint, so the entitlement and audit trail are created exactly
// as in production.
func (e *testEnv) grantAccess(token, courseID string) {
	e.t.Helper()
	resp := e.do(http.MethodPost, "/api/v1/purchase/checkout", token, map[string]any{"course_id": courseID})
	e.requireStatus(resp, http.StatusOK)
}

func TestPlayer_UnentitledUserIsForbidden(t *testing.T) {
	e := newTestEnv(t)
	teacher, _ := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	_, studentTok := e.register("student@example.com", "Student", domain.RoleStudent)
	courseID := e.insertCourse(teacher, "Go", "Programming", 49.99, "published")
	lessonID := e.insertLesson(courseID, "Intro", "video", 1)

	resp := e.do(http.MethodGet, "/api/v1/player/courses/"+courseID+"/lessons/"+lessonID, studentTok, nil)
	e.requireStatus(resp, http.StatusForbidden)
	if msg := e.errorMessage(resp); msg != "access denied: no active entitlement" {
		t.Errorf("error = %q", msg)
	}

	// The guard still records an audit row for the denial.
	if n := e.countRows(`SELECT count(*) FROM access_check_log WHERE decision='deny' AND course_id=$1`, courseID); n != 1 {
		t.Errorf("deny audit rows = %d, want 1", n)
	}
}

func TestPlayer_EntitledUserGetsContentAndAuditAllow(t *testing.T) {
	e := newTestEnv(t)
	teacher, _ := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	_, studentTok := e.register("student@example.com", "Student", domain.RoleStudent)
	courseID := e.insertCourse(teacher, "Go", "Programming", 49.99, "published")
	lessonID := e.insertLesson(courseID, "Intro", "video", 1)
	e.insertMedia(lessonID, "https://cdn.example.com/intro.mp4", 120000)
	e.insertMaterial(lessonID, "Slides", "https://cdn.example.com/intro.pdf", "pdf")

	e.grantAccess(studentTok, courseID)

	resp := e.do(http.MethodGet, "/api/v1/player/courses/"+courseID+"/lessons/"+lessonID, studentTok, nil)
	e.requireStatus(resp, http.StatusOK)
	var ld lessonData
	e.decode(resp, &ld)
	if ld.ContentURL != "https://cdn.example.com/intro.mp4" {
		t.Errorf("content_url = %q", ld.ContentURL)
	}
	if ld.DurationSeconds != 120 {
		t.Errorf("duration_seconds = %d, want 120", ld.DurationSeconds)
	}
	if len(ld.Materials) != 1 || ld.Materials[0].Title != "Slides" {
		t.Errorf("materials = %+v", ld.Materials)
	}
	if ld.LastProgressSeconds != 0 {
		t.Errorf("fresh lesson last_progress_seconds = %d, want 0", ld.LastProgressSeconds)
	}

	if n := e.countRows(`SELECT count(*) FROM access_check_log WHERE decision='allow' AND course_id=$1`, courseID); n != 1 {
		t.Errorf("allow audit rows = %d, want 1", n)
	}
}

func TestPlayer_GrantForOneCourseCannotReadAnothersLesson(t *testing.T) {
	e := newTestEnv(t)
	teacher, _ := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	_, studentTok := e.register("student@example.com", "Student", domain.RoleStudent)

	courseA := e.insertCourse(teacher, "Course A", "X", 10, "published")
	courseB := e.insertCourse(teacher, "Course B", "X", 10, "published")
	lessonB := e.insertLesson(courseB, "B-Lesson", "video", 1)

	e.grantAccess(studentTok, courseA) // entitled to A only

	// Use A's id in the path (guard passes) but B's lesson id: the lesson is scoped
	// to the course, so it resolves to nothing → 404, not a content leak.
	resp := e.do(http.MethodGet, "/api/v1/player/courses/"+courseA+"/lessons/"+lessonB, studentTok, nil)
	e.requireStatus(resp, http.StatusNotFound)
	if msg := e.errorMessage(resp); msg != "lesson not found" {
		t.Errorf("error = %q, want 'lesson not found'", msg)
	}
}

func TestPlayer_ProgressIsScopedPerUser(t *testing.T) {
	e := newTestEnv(t)
	teacher, _ := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	_, aliceTok := e.register("alice@example.com", "Alice", domain.RoleStudent)
	_, bobTok := e.register("bob@example.com", "Bob", domain.RoleStudent)
	courseID := e.insertCourse(teacher, "Go", "Programming", 10, "published")
	lessonID := e.insertLesson(courseID, "Intro", "video", 1)
	e.insertMedia(lessonID, "https://cdn.example.com/intro.mp4", 200000) // 200s

	e.grantAccess(aliceTok, courseID)
	e.grantAccess(bobTok, courseID)

	// Alice saves progress at 50s.
	save := e.do(http.MethodPost, "/api/v1/player/courses/"+courseID+"/lessons/"+lessonID+"/progress",
		aliceTok, map[string]any{"progress_seconds": 50})
	e.requireStatus(save, http.StatusOK)
	var saved progressData
	e.decode(save, &saved)
	if saved.ProgressSeconds != 50 {
		t.Errorf("saved progress_seconds = %d, want 50", saved.ProgressSeconds)
	}
	if saved.PercentComplete != 25 { // 50s / 200s
		t.Errorf("percent_complete = %v, want 25", saved.PercentComplete)
	}

	// Alice resumes: she sees her own progress.
	aliceGet := e.do(http.MethodGet, "/api/v1/player/courses/"+courseID+"/lessons/"+lessonID+"/progress", aliceTok, nil)
	e.requireStatus(aliceGet, http.StatusOK)
	var aliceProg progressData
	e.decode(aliceGet, &aliceProg)
	if aliceProg.ProgressSeconds != 50 {
		t.Errorf("alice resume = %d, want 50", aliceProg.ProgressSeconds)
	}

	// Bob, equally entitled, has no progress of his own — Alice's is not visible.
	bobGet := e.do(http.MethodGet, "/api/v1/player/courses/"+courseID+"/lessons/"+lessonID+"/progress", bobTok, nil)
	e.requireStatus(bobGet, http.StatusNotFound)
}

func TestPlayer_SaveProgressValidation(t *testing.T) {
	e := newTestEnv(t)
	teacher, _ := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	_, studentTok := e.register("student@example.com", "Student", domain.RoleStudent)
	courseID := e.insertCourse(teacher, "Go", "Programming", 10, "published")
	lessonID := e.insertLesson(courseID, "Intro", "video", 1)
	e.grantAccess(studentTok, courseID)

	base := "/api/v1/player/courses/" + courseID + "/lessons/" + lessonID + "/progress"

	// progress_seconds is required (pointer): omitting it is a 400, not a save of 0.
	missing := e.do(http.MethodPost, base, studentTok, map[string]any{"completed": true})
	e.requireStatus(missing, http.StatusBadRequest)

	// Negative offset rejected.
	negative := e.do(http.MethodPost, base, studentTok, map[string]any{"progress_seconds": -5})
	e.requireStatus(negative, http.StatusBadRequest)
}
