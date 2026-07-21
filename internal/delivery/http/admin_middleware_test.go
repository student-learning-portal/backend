package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
)

// These tests drive the gates directly with the claims RequireAuth would have
// injected (authRequestWithClaims, auth_handlers_test.go), so no token is minted.

// --- RequireAdmin ---

func TestRequireAdmin_AllowsAdmin(t *testing.T) {
	next := &nextCapture{}
	w := httptest.NewRecorder()

	RequireAdmin(next.handler)(w, authRequestWithClaims(domain.Claims{UserID: "a1", Role: domain.RoleAdmin}))

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !next.called {
		t.Error("next handler must be called for an administrator")
	}
}

func TestRequireAdmin_RejectsOtherRoles(t *testing.T) {
	for _, role := range []domain.Role{domain.RoleTeacher, domain.RoleStudent} {
		t.Run(string(role), func(t *testing.T) {
			next := &nextCapture{}
			w := httptest.NewRecorder()

			RequireAdmin(next.handler)(w, authRequestWithClaims(domain.Claims{UserID: "u1", Role: role}))

			if w.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", w.Code)
			}
			if next.called {
				t.Errorf("next handler must not be called for %s", role)
			}
		})
	}
}

func TestRequireAdmin_RejectsUnauthenticated(t *testing.T) {
	next := &nextCapture{}
	w := httptest.NewRecorder()

	RequireAdmin(next.handler)(w, httptest.NewRequest(http.MethodGet, "http://x/", nil))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
	if next.called {
		t.Error("next handler must not be called without claims")
	}
}

// --- RequireApprovedTeacher ---

func TestRequireApprovedTeacher_AllowsApprovedTeacher(t *testing.T) {
	repo := &authStubUserRepo{user: domain.User{
		ID: "t1", Role: domain.RoleTeacher, TeacherStatus: domain.TeacherStatusApproved,
	}}
	next := &nextCapture{}
	w := httptest.NewRecorder()

	RequireApprovedTeacher(repo)(next.handler)(w, authRequestWithClaims(domain.Claims{UserID: "t1", Role: domain.RoleTeacher}))

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !next.called {
		t.Error("next handler must be called for an approved teacher")
	}
}

func TestRequireApprovedTeacher_BlocksUnapprovedTeacher(t *testing.T) {
	for _, status := range []domain.TeacherStatus{domain.TeacherStatusPending, domain.TeacherStatusRejected, ""} {
		t.Run(string(status), func(t *testing.T) {
			repo := &authStubUserRepo{user: domain.User{
				ID: "t1", Role: domain.RoleTeacher, TeacherStatus: status,
			}}
			next := &nextCapture{}
			w := httptest.NewRecorder()

			claims := domain.Claims{UserID: "t1", Role: domain.RoleTeacher}
			RequireApprovedTeacher(repo)(next.handler)(w, authRequestWithClaims(claims))

			if w.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", w.Code)
			}
			if next.called {
				t.Error("next handler must not be called for an unapproved teacher")
			}

			var body map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body %q: %v", w.Body.String(), err)
			}
			// The frontend distinguishes "still waiting" from "declined" by this field.
			if body["status"] != string(status) {
				t.Errorf("status field = %q, want %q", body["status"], status)
			}
		})
	}
}

// The token's approval state is never trusted: a teacher approved after their
// token was issued gets in, one rejected afterwards is turned away.
func TestRequireApprovedTeacher_ReadsCurrentStateNotTheToken(t *testing.T) {
	repo := &authStubUserRepo{user: domain.User{
		ID: "t1", Role: domain.RoleTeacher, TeacherStatus: domain.TeacherStatusPending,
	}}
	mw := RequireApprovedTeacher(repo)
	claims := domain.Claims{UserID: "t1", Role: domain.RoleTeacher}

	first := &nextCapture{}
	mw(first.handler)(httptest.NewRecorder(), authRequestWithClaims(claims))
	if first.called {
		t.Fatal("pending teacher must be blocked")
	}

	repo.user.TeacherStatus = domain.TeacherStatusApproved

	second := &nextCapture{}
	mw(second.handler)(httptest.NewRecorder(), authRequestWithClaims(claims))
	if !second.called {
		t.Error("teacher approved mid-session must be let through with the same token")
	}
}

// Non-teachers are none of this middleware's business — the handlers do their
// own role checks and must see the request.
func TestRequireApprovedTeacher_PassesNonTeachersThrough(t *testing.T) {
	repo := &authStubUserRepo{getErr: domain.ErrUserNotFound}
	next := &nextCapture{}
	w := httptest.NewRecorder()

	claims := domain.Claims{UserID: "s1", Role: domain.RoleStudent}
	RequireApprovedTeacher(repo)(next.handler)(w, authRequestWithClaims(claims))

	if !next.called {
		t.Error("a student must reach the handler without an approval lookup")
	}
}

func TestRequireApprovedTeacher_UnknownAccountIsUnauthorized(t *testing.T) {
	repo := &authStubUserRepo{getErr: domain.ErrUserNotFound}
	next := &nextCapture{}
	w := httptest.NewRecorder()

	claims := domain.Claims{UserID: "gone", Role: domain.RoleTeacher}
	RequireApprovedTeacher(repo)(next.handler)(w, authRequestWithClaims(claims))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
	if next.called {
		t.Error("next handler must not be called for a deleted account")
	}
}
