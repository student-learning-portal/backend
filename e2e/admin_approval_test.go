package e2e

import (
	"net/http"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
)

// --- response views mirroring the admin handler DTOs ---------------------

type teacherApplicationView struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Status   string `json:"status"`
}

type teacherApplicationsView struct {
	Pending int                      `json:"pending"`
	Items   []teacherApplicationView `json:"items"`
}

// A freshly registered teacher is created, can sign in, and is reported as
// pending — the account exists, it just isn't a working teacher yet.
func TestAdminApproval_TeacherRegistersAsPending(t *testing.T) {
	e := newTestEnv(t)
	_, tok := e.registerRaw("teacher@example.com", "Tess Teacher", domain.RoleTeacher)

	resp := e.do(http.MethodGet, "/api/v1/auth/me", tok, nil)
	e.requireStatus(resp, http.StatusOK)

	var me struct {
		Role          string `json:"role"`
		TeacherStatus string `json:"teacher_status"`
	}
	e.decode(resp, &me)
	if me.Role != string(domain.RoleTeacher) {
		t.Fatalf("role = %q, want teacher", me.Role)
	}
	if me.TeacherStatus != string(domain.TeacherStatusPending) {
		t.Fatalf("teacher_status = %q, want pending", me.TeacherStatus)
	}
}

// A student registration never enters the queue.
func TestAdminApproval_StudentHasNoApprovalState(t *testing.T) {
	e := newTestEnv(t)
	_, tok := e.registerRaw("student@example.com", "Sam Student", domain.RoleStudent)

	resp := e.do(http.MethodGet, "/api/v1/auth/me", tok, nil)
	e.requireStatus(resp, http.StatusOK)

	var me struct {
		TeacherStatus string `json:"teacher_status"`
	}
	e.decode(resp, &me)
	if me.TeacherStatus != "" {
		t.Fatalf("teacher_status = %q, want empty for a student", me.TeacherStatus)
	}
}

// The gate: a pending teacher is refused by the authoring endpoints, and the
// same request succeeds once the administrator approves them.
func TestAdminApproval_PendingTeacherBlockedUntilApproved(t *testing.T) {
	e := newTestEnv(t)
	teacherID, teacherTok := e.registerRaw("teacher@example.com", "Tess Teacher", domain.RoleTeacher)
	_, adminTok := e.insertAdmin("admin")

	newCourse := map[string]any{"title": "Go from Scratch", "description": "d", "subject": "Programming", "price": 25.0}

	resp := e.do(http.MethodPost, "/api/v1/teacher/courses", teacherTok, newCourse)
	e.requireStatus(resp, http.StatusForbidden)

	resp = e.do(http.MethodGet, "/api/v1/analytics/teacher/dashboard?course_id=x", teacherTok, nil)
	e.requireStatus(resp, http.StatusForbidden)

	resp = e.do(http.MethodPost, "/api/v1/admin/teachers/"+teacherID+"/approve", adminTok, nil)
	e.requireStatus(resp, http.StatusOK)

	var approved teacherApplicationView
	e.decode(resp, &approved)
	if approved.Status != string(domain.TeacherStatusApproved) {
		t.Fatalf("status after approve = %q, want approved", approved.Status)
	}

	// The 24h-old token stays valid: approval is read from the database on
	// every request, so the teacher never has to sign in again.
	resp = e.do(http.MethodPost, "/api/v1/teacher/courses", teacherTok, newCourse)
	e.requireStatus(resp, http.StatusCreated)
}

// Rejecting closes the endpoints again for an account that was approved before.
func TestAdminApproval_RejectRevokesTeacherAccess(t *testing.T) {
	e := newTestEnv(t)
	teacherID, teacherTok := e.register("teacher@example.com", "Tess Teacher", domain.RoleTeacher)
	_, adminTok := e.insertAdmin("admin")

	resp := e.do(http.MethodPost, "/api/v1/admin/teachers/"+teacherID+"/reject", adminTok, nil)
	e.requireStatus(resp, http.StatusOK)

	resp = e.do(http.MethodPost, "/api/v1/teacher/courses", teacherTok, map[string]any{
		"title": "Blocked", "description": "d", "subject": "Programming", "price": 1.0,
	})
	e.requireStatus(resp, http.StatusForbidden)
}

// The queue lists exactly the teachers awaiting review, and drops them once
// they are decided on.
func TestAdminApproval_QueueListsPendingTeachers(t *testing.T) {
	e := newTestEnv(t)
	pendingID, _ := e.registerRaw("pending@example.com", "Paula Pending", domain.RoleTeacher)
	e.register("approved@example.com", "Adam Approved", domain.RoleTeacher)
	e.registerRaw("student@example.com", "Sam Student", domain.RoleStudent)
	_, adminTok := e.insertAdmin("admin")

	resp := e.do(http.MethodGet, "/api/v1/admin/teachers", adminTok, nil)
	e.requireStatus(resp, http.StatusOK)

	var queue teacherApplicationsView
	e.decode(resp, &queue)
	if len(queue.Items) != 1 || queue.Items[0].ID != pendingID {
		t.Fatalf("pending queue = %+v, want only %s", queue.Items, pendingID)
	}
	if queue.Pending != 1 {
		t.Fatalf("pending count = %d, want 1", queue.Pending)
	}

	e.requireStatus(e.do(http.MethodPost, "/api/v1/admin/teachers/"+pendingID+"/approve", adminTok, nil), http.StatusOK)

	resp = e.do(http.MethodGet, "/api/v1/admin/teachers", adminTok, nil)
	e.requireStatus(resp, http.StatusOK)
	e.decode(resp, &queue)
	if len(queue.Items) != 0 {
		t.Fatalf("pending queue after approval = %+v, want empty", queue.Items)
	}

	// status=all keeps the decided applications visible.
	resp = e.do(http.MethodGet, "/api/v1/admin/teachers?status=all", adminTok, nil)
	e.requireStatus(resp, http.StatusOK)
	e.decode(resp, &queue)
	if len(queue.Items) != 2 {
		t.Fatalf("all applications = %+v, want 2 teachers", queue.Items)
	}
}

// Only administrators reach the queue — in particular a teacher must not be
// able to approve themselves.
func TestAdminApproval_QueueIsAdminOnly(t *testing.T) {
	e := newTestEnv(t)
	teacherID, teacherTok := e.registerRaw("teacher@example.com", "Tess Teacher", domain.RoleTeacher)
	_, studentTok := e.registerRaw("student@example.com", "Sam Student", domain.RoleStudent)

	for _, tc := range []struct {
		name, method, path, token string
	}{
		{"teacher lists queue", http.MethodGet, "/api/v1/admin/teachers", teacherTok},
		{"student lists queue", http.MethodGet, "/api/v1/admin/teachers", studentTok},
		{"teacher self-approves", http.MethodPost, "/api/v1/admin/teachers/" + teacherID + "/approve", teacherTok},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e.requireStatus(e.do(tc.method, tc.path, tc.token, nil), http.StatusForbidden)
		})
	}

	e.requireStatus(e.do(http.MethodGet, "/api/v1/admin/teachers", "", nil), http.StatusUnauthorized)
}

// A decision aimed at a non-teacher is a conflict, not a silent role change.
func TestAdminApproval_CannotApproveNonTeacher(t *testing.T) {
	e := newTestEnv(t)
	studentID, _ := e.registerRaw("student@example.com", "Sam Student", domain.RoleStudent)
	_, adminTok := e.insertAdmin("admin")

	resp := e.do(http.MethodPost, "/api/v1/admin/teachers/"+studentID+"/approve", adminTok, nil)
	e.requireStatus(resp, http.StatusConflict)

	resp = e.do(http.MethodPost, "/api/v1/admin/teachers/00000000-0000-4000-8000-000000000001/approve", adminTok, nil)
	e.requireStatus(resp, http.StatusNotFound)
}

// The public registration endpoint must never mint an administrator.
func TestAdminApproval_CannotRegisterAsAdmin(t *testing.T) {
	e := newTestEnv(t)
	resp := e.do(http.MethodPost, "/api/v1/auth/register", "", registerBody{
		Email: "wannabe@example.com", Password: testPassword, FullName: "Wannabe Admin", Role: "admin",
	})
	e.requireStatus(resp, http.StatusBadRequest)
}
