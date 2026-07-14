package e2e

import (
	"net/http"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
)

type chatMessage struct {
	ID         string  `json:"id"`
	CourseID   string  `json:"course_id"`
	StudentID  string  `json:"student_id"`
	LessonID   *string `json:"lesson_id"`
	SenderRole string  `json:"sender_role"`
	SenderID   string  `json:"sender_id"`
	Body       string  `json:"body"`
	CreatedAt  string  `json:"created_at"`
}

type chatThread struct {
	StudentID    string `json:"student_id"`
	StudentName  string `json:"student_name"`
	MessageCount int    `json:"message_count"`
	LastMessage  string `json:"last_message"`
	LastAt       string `json:"last_at"`
}

// TestChat_StudentTeacherConversation walks the whole feature: an enrolled
// student messages the teacher, the teacher sees the thread and replies, and the
// student sees the reply — all through the real endpoints against Postgres.
func TestChat_StudentTeacherConversation(t *testing.T) {
	e := newTestEnv(t)
	teacherID, teacherTok := e.register("teacher@example.com", "Terry Teacher", domain.RoleTeacher)
	studentID, studentTok := e.register("student@example.com", "Sam Student", domain.RoleStudent)
	courseID := e.insertCourse(teacherID, "Go", "Programming", 10, "published")
	e.grantAccess(studentTok, courseID) // enroll the student

	studentPath := "/api/v1/courses/" + courseID + "/messages"
	teacherThreads := "/api/v1/teacher/courses/" + courseID + "/threads"
	teacherThread := teacherThreads + "/" + studentID + "/messages"

	// 1. Student sends a question.
	send := e.do(http.MethodPost, studentPath, studentTok, map[string]any{"body": "Why did lesson 2 fail?"})
	e.requireStatus(send, http.StatusCreated)
	var sent chatMessage
	e.decode(send, &sent)
	if sent.SenderRole != "student" || sent.SenderID != studentID || sent.Body != "Why did lesson 2 fail?" {
		t.Fatalf("sent message wrong: %+v", sent)
	}

	// 2. Teacher's inbox shows one thread with that student.
	inbox := e.do(http.MethodGet, teacherThreads, teacherTok, nil)
	e.requireStatus(inbox, http.StatusOK)
	var threads []chatThread
	e.decode(inbox, &threads)
	if len(threads) != 1 || threads[0].StudentID != studentID ||
		threads[0].MessageCount != 1 || threads[0].LastMessage != "Why did lesson 2 fail?" {
		t.Fatalf("teacher inbox wrong: %+v", threads)
	}
	if threads[0].StudentName != "Sam Student" {
		t.Errorf("student_name = %q, want 'Sam Student'", threads[0].StudentName)
	}

	// 3. Teacher reads and replies to that student's thread.
	e.requireStatus(e.do(http.MethodGet, teacherThread, teacherTok, nil), http.StatusOK)
	reply := e.do(http.MethodPost, teacherThread, teacherTok, map[string]any{"body": "Check your imports."})
	e.requireStatus(reply, http.StatusCreated)
	var replied chatMessage
	e.decode(reply, &replied)
	if replied.SenderRole != "teacher" || replied.SenderID != teacherID {
		t.Fatalf("reply identity wrong: %+v", replied)
	}

	// 4. Student now sees both messages, oldest first.
	view := e.do(http.MethodGet, studentPath, studentTok, nil)
	e.requireStatus(view, http.StatusOK)
	var msgs []chatMessage
	e.decode(view, &msgs)
	if len(msgs) != 2 {
		t.Fatalf("student thread has %d messages, want 2", len(msgs))
	}
	if msgs[0].SenderRole != "student" || msgs[1].SenderRole != "teacher" {
		t.Errorf("order wrong: %s then %s, want student then teacher", msgs[0].SenderRole, msgs[1].SenderRole)
	}
}

func TestChat_UnenrolledStudentForbidden(t *testing.T) {
	e := newTestEnv(t)
	teacherID, _ := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	_, outsiderTok := e.register("outsider@example.com", "Outsider", domain.RoleStudent)
	courseID := e.insertCourse(teacherID, "Go", "Programming", 10, "published")

	path := "/api/v1/courses/" + courseID + "/messages"
	e.requireStatus(e.do(http.MethodGet, path, outsiderTok, nil), http.StatusForbidden)
	e.requireStatus(e.do(http.MethodPost, path, outsiderTok, map[string]any{"body": "hi"}), http.StatusForbidden)
}

func TestChat_TeacherCannotAccessAnothersCourse(t *testing.T) {
	e := newTestEnv(t)
	ownerID, _ := e.register("owner@example.com", "Owner", domain.RoleTeacher)
	_, otherTok := e.register("other@example.com", "Other Teacher", domain.RoleTeacher)
	courseID := e.insertCourse(ownerID, "Go", "Programming", 10, "published")

	resp := e.do(http.MethodGet, "/api/v1/teacher/courses/"+courseID+"/threads", otherTok, nil)
	e.requireStatus(resp, http.StatusForbidden)
}

func TestChat_RoleAndValidationGuards(t *testing.T) {
	e := newTestEnv(t)
	teacherID, teacherTok := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	_, studentTok := e.register("student@example.com", "Student", domain.RoleStudent)
	courseID := e.insertCourse(teacherID, "Go", "Programming", 10, "published")
	e.grantAccess(studentTok, courseID)

	// A teacher hitting the student route is rejected on role.
	e.requireStatus(e.do(http.MethodGet, "/api/v1/courses/"+courseID+"/messages", teacherTok, nil), http.StatusForbidden)
	// A student hitting the teacher route is rejected on role.
	e.requireStatus(e.do(http.MethodGet, "/api/v1/teacher/courses/"+courseID+"/threads", studentTok, nil), http.StatusForbidden)
	// Empty message body is a 400.
	e.requireStatus(e.do(http.MethodPost, "/api/v1/courses/"+courseID+"/messages", studentTok, map[string]any{"body": "   "}), http.StatusBadRequest)
	// No token is a 401.
	e.requireStatus(e.do(http.MethodGet, "/api/v1/courses/"+courseID+"/messages", "", nil), http.StatusUnauthorized)
}
