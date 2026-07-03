package e2e

import (
	"net/http"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
)

// --- response views mirroring the teacher-content handler DTOs ----------

type teacherCourseView struct {
	ID        string  `json:"id"`
	TeacherID string  `json:"teacher_id"`
	Title     string  `json:"title"`
	Price     float64 `json:"price"`
	Status    string  `json:"status"`
}

type teacherLessonView struct {
	LessonID   string `json:"lesson_id"`
	CourseID   string `json:"course_id"`
	Title      string `json:"title"`
	LessonType string `json:"lesson_type"`
	Position   int    `json:"position"`
}

type teacherMediaView struct {
	ID              string `json:"id"`
	URL             string `json:"url"`
	DurationSeconds int    `json:"duration_seconds"`
	MediaType       string `json:"media_type"`
}

type teacherMaterialView struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
	Type  string `json:"type"`
}

func TestTeacherContent_CreateEditPublishFlow(t *testing.T) {
	e := newTestEnv(t)
	_, teacherTok := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)

	// Create a draft course.
	resp := e.do(http.MethodPost, "/api/v1/teacher/courses", teacherTok, map[string]any{
		"title": "Go from Scratch", "description": "d", "subject": "Programming", "price": 25.0,
	})
	e.requireStatus(resp, http.StatusCreated)
	var created teacherCourseView
	e.decode(resp, &created)
	if created.Status != "draft" {
		t.Fatalf("new course status = %q, want draft", created.Status)
	}
	courseID := created.ID

	// Add two lessons; the second appends after the first (position order).
	resp = e.do(http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", teacherTok, map[string]any{
		"title": "Intro", "lesson_type": "video",
	})
	e.requireStatus(resp, http.StatusCreated)
	var lesson1 teacherLessonView
	e.decode(resp, &lesson1)

	resp = e.do(http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", teacherTok, map[string]any{
		"title": "Variables", "lesson_type": "text",
	})
	e.requireStatus(resp, http.StatusCreated)
	var lesson2 teacherLessonView
	e.decode(resp, &lesson2)
	if lesson2.Position <= lesson1.Position {
		t.Fatalf("lesson2.Position = %d, want > lesson1.Position (%d)", lesson2.Position, lesson1.Position)
	}

	// Reorder: put lesson2 first.
	resp = e.do(http.MethodPut, "/api/v1/teacher/courses/"+courseID+"/lessons/order", teacherTok, map[string]any{
		"lesson_ids": []string{lesson2.LessonID, lesson1.LessonID},
	})
	e.requireStatus(resp, http.StatusNoContent)

	// Attach media to lesson1 and a material.
	resp = e.do(http.MethodPut, "/api/v1/teacher/courses/"+courseID+"/lessons/"+lesson1.LessonID+"/media", teacherTok, map[string]any{
		"url": testMediaURL, "duration_seconds": 120, "media_type": "video",
	})
	e.requireStatus(resp, http.StatusOK)
	var media teacherMediaView
	e.decode(resp, &media)
	if media.DurationSeconds != 120 {
		t.Errorf("media.DurationSeconds = %d, want 120", media.DurationSeconds)
	}

	resp = e.do(http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons/"+lesson1.LessonID+"/materials", teacherTok, map[string]any{
		"title": "Slides", "url": "https://cdn.example.com/slides.pdf", "type": "pdf",
	})
	e.requireStatus(resp, http.StatusCreated)
	var material teacherMaterialView
	e.decode(resp, &material)

	// Publish the course.
	resp = e.do(http.MethodPatch, "/api/v1/teacher/courses/"+courseID, teacherTok, map[string]any{
		"title": "Go from Scratch", "description": "d", "subject": "Programming", "price": 25.0, "status": "published",
	})
	e.requireStatus(resp, http.StatusOK)
	var published teacherCourseView
	e.decode(resp, &published)
	if published.Status != "published" {
		t.Fatalf("status after publish = %q, want published", published.Status)
	}

	// The public catalog now lists it, with the reordered lessons and correct
	// lesson_type (regression check for the id/type -> lesson_id/lesson_type DTO fix).
	resp = e.do(http.MethodGet, "/api/v1/catalog/courses/"+courseID+"/lessons", "", nil)
	e.requireStatus(resp, http.StatusOK)
	var lessons []teacherLessonView
	e.decode(resp, &lessons)
	if len(lessons) != 2 || lessons[0].Title != "Variables" || lessons[0].LessonType != "text" {
		t.Fatalf("lessons after reorder = %+v, want [Variables(text), Intro(video)]", lessons)
	}

	// Clean up: remove the material and the second lesson.
	resp = e.do(http.MethodDelete, "/api/v1/teacher/courses/"+courseID+"/lessons/"+lesson1.LessonID+"/materials/"+material.ID, teacherTok, nil)
	e.requireStatus(resp, http.StatusNoContent)

	resp = e.do(http.MethodDelete, "/api/v1/teacher/courses/"+courseID+"/lessons/"+lesson2.LessonID, teacherTok, nil)
	e.requireStatus(resp, http.StatusNoContent)

	if n := e.countRows(`SELECT count(*) FROM lessons WHERE course_id = $1`, courseID); n != 1 {
		t.Errorf("lessons remaining = %d, want 1", n)
	}
}

func TestTeacherContent_ForbiddenCrossTeacherAccess(t *testing.T) {
	e := newTestEnv(t)
	teacherA, _ := e.register("teacher-a@example.com", "Teacher A", domain.RoleTeacher)
	_, tokB := e.register("teacher-b@example.com", "Teacher B", domain.RoleTeacher)
	courseID := e.insertCourse(teacherA, "A's Course", "Programming", 10, "draft")

	resp := e.do(http.MethodPatch, "/api/v1/teacher/courses/"+courseID, tokB, map[string]any{
		"title": "Hijacked", "status": "draft",
	})
	e.requireStatus(resp, http.StatusForbidden)

	resp = e.do(http.MethodPost, "/api/v1/teacher/courses/"+courseID+"/lessons", tokB, map[string]any{
		"title": "Sneaky lesson", "lesson_type": "video",
	})
	e.requireStatus(resp, http.StatusForbidden)

	resp = e.do(http.MethodDelete, "/api/v1/teacher/courses/"+courseID, tokB, nil)
	e.requireStatus(resp, http.StatusForbidden)
}

func TestTeacherContent_DeleteRequiresDraft(t *testing.T) {
	e := newTestEnv(t)
	teacherID, tok := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	courseID := e.insertCourse(teacherID, "Published Course", "Programming", 10, "published")

	resp := e.do(http.MethodDelete, "/api/v1/teacher/courses/"+courseID, tok, nil)
	e.requireStatus(resp, http.StatusConflict)

	// Archive it back to draft, then deletion is allowed.
	resp = e.do(http.MethodPatch, "/api/v1/teacher/courses/"+courseID, tok, map[string]any{
		"title": "Published Course", "subject": "Programming", "price": 10.0, "status": "draft",
	})
	e.requireStatus(resp, http.StatusOK)

	resp = e.do(http.MethodDelete, "/api/v1/teacher/courses/"+courseID, tok, nil)
	e.requireStatus(resp, http.StatusNoContent)

	if n := e.countRows(`SELECT count(*) FROM courses WHERE id = $1`, courseID); n != 0 {
		t.Errorf("course rows remaining = %d, want 0", n)
	}
}

func TestTeacherContent_StudentCannotAuthor(t *testing.T) {
	e := newTestEnv(t)
	_, studentTok := e.register("student@example.com", "Student", domain.RoleStudent)

	resp := e.do(http.MethodPost, "/api/v1/teacher/courses", studentTok, map[string]any{
		"title": "Not allowed", "price": 1.0,
	})
	e.requireStatus(resp, http.StatusForbidden)
}

// TestRequireEntitlement_TeacherPreviewsOwnUnpublishedCourse exercises the
// middleware fix: a teacher can open their own course's lesson through the
// player even with no purchase grant and the course still in draft, while a
// second teacher (who doesn't own it) is still denied.
func TestRequireEntitlement_TeacherPreviewsOwnUnpublishedCourse(t *testing.T) {
	e := newTestEnv(t)
	teacherA, tokA := e.register("teacher-a@example.com", "Teacher A", domain.RoleTeacher)
	_, tokB := e.register("teacher-b@example.com", "Teacher B", domain.RoleTeacher)

	courseID := e.insertCourse(teacherA, "Draft Course", "Programming", 10, "draft")
	lessonID := e.insertLesson(courseID, "Intro", "video", 0)
	e.insertMedia(lessonID, 60_000)

	resp := e.do(http.MethodGet, "/api/v1/player/courses/"+courseID+"/lessons/"+lessonID, tokA, nil)
	e.requireStatus(resp, http.StatusOK)

	resp = e.do(http.MethodGet, "/api/v1/player/courses/"+courseID+"/lessons/"+lessonID, tokB, nil)
	e.requireStatus(resp, http.StatusForbidden)
}
