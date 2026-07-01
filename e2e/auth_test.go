package e2e

import (
	"net/http"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
)

func TestAuth_RegisterLoginMeRoundTrip(t *testing.T) {
	e := newTestEnv(t)

	reg := e.do(http.MethodPost, "/api/v1/auth/register", "", registerBody{
		Email: "alice@example.com", Password: testPassword, FullName: "Alice", Role: string(domain.RoleStudent),
	})
	e.requireStatus(reg, http.StatusCreated)
	var regOut struct {
		Token string `json:"token"`
		User  struct {
			ID      string  `json:"id"`
			Email   string  `json:"email"`
			Role    string  `json:"role"`
			Balance float64 `json:"balance"`
		} `json:"user"`
	}
	e.decode(reg, &regOut)
	if regOut.User.Email != "alice@example.com" || regOut.User.Role != "student" {
		t.Fatalf("unexpected register user: %+v", regOut.User)
	}
	if regOut.User.Balance != 1000 {
		t.Errorf("new wallet balance = %v, want 1000", regOut.User.Balance)
	}

	login := e.do(http.MethodPost, "/api/v1/auth/login", "", loginBody{
		Email: "alice@example.com", Password: testPassword,
	})
	e.requireStatus(login, http.StatusOK)
	var loginOut struct {
		Token string `json:"token"`
	}
	e.decode(login, &loginOut)
	if loginOut.Token == "" {
		t.Fatal("login returned empty token")
	}

	me := e.do(http.MethodGet, "/api/v1/auth/me", loginOut.Token, nil)
	e.requireStatus(me, http.StatusOK)
	var meOut struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	e.decode(me, &meOut)
	if meOut.ID != regOut.User.ID || meOut.Email != "alice@example.com" {
		t.Fatalf("/me = %+v, want id=%s email=alice@example.com", meOut, regOut.User.ID)
	}
}

func TestAuth_DuplicateEmailIsConflict_CaseInsensitive(t *testing.T) {
	e := newTestEnv(t)
	e.register("bob@example.com", "Bob", domain.RoleStudent)

	// Same email, different case + whitespace — normalized server-side, so it collides.
	resp := e.do(http.MethodPost, "/api/v1/auth/register", "", registerBody{
		Email: "  BOB@example.com  ", Password: testPassword, FullName: "Bob 2", Role: string(domain.RoleStudent),
	})
	e.requireStatus(resp, http.StatusConflict)
	if msg := e.errorMessage(resp); msg != "email already registered" {
		t.Errorf("error = %q, want 'email already registered'", msg)
	}
}

func TestAuth_LoginWrongPasswordIsUnauthorized(t *testing.T) {
	e := newTestEnv(t)
	e.register("carol@example.com", "Carol", domain.RoleStudent)

	resp := e.do(http.MethodPost, "/api/v1/auth/login", "", loginBody{
		Email: "carol@example.com", Password: "wrong-password",
	})
	e.requireStatus(resp, http.StatusUnauthorized)
	if msg := e.errorMessage(resp); msg != "invalid email or password" {
		t.Errorf("error = %q, want 'invalid email or password'", msg)
	}
}

func TestAuth_RegisterValidationRejectsShortPassword(t *testing.T) {
	e := newTestEnv(t)
	resp := e.do(http.MethodPost, "/api/v1/auth/register", "", registerBody{
		Email: "dan@example.com", Password: "short", FullName: "Dan", Role: string(domain.RoleStudent),
	})
	e.requireStatus(resp, http.StatusBadRequest)
}

func TestAuth_MeRequiresValidToken(t *testing.T) {
	e := newTestEnv(t)

	noTok := e.do(http.MethodGet, "/api/v1/auth/me", "", nil)
	e.requireStatus(noTok, http.StatusUnauthorized)
	if msg := e.errorMessage(noTok); msg != "missing bearer token" {
		t.Errorf("no-token error = %q", msg)
	}

	badTok := e.do(http.MethodGet, "/api/v1/auth/me", "not-a-jwt", nil)
	e.requireStatus(badTok, http.StatusUnauthorized)
	if msg := e.errorMessage(badTok); msg != "invalid or expired token" {
		t.Errorf("bad-token error = %q", msg)
	}
}

func TestAuth_GetTeacherHidesNonTeachers(t *testing.T) {
	e := newTestEnv(t)
	teacherID, _ := e.register("teacher@example.com", "Tess Teacher", domain.RoleTeacher)
	studentID, _ := e.register("student@example.com", "Sam Student", domain.RoleStudent)

	ok := e.do(http.MethodGet, "/api/v1/teachers/"+teacherID, "", nil)
	e.requireStatus(ok, http.StatusOK)
	var teacher struct {
		ID       string `json:"id"`
		FullName string `json:"full_name"`
		Role     string `json:"role"`
	}
	e.decode(ok, &teacher)
	if teacher.ID != teacherID || teacher.FullName != "Tess Teacher" || teacher.Role != "teacher" {
		t.Fatalf("teacher payload = %+v", teacher)
	}

	// A student id is deliberately indistinguishable from "not found" (anti-enumeration).
	notFound := e.do(http.MethodGet, "/api/v1/teachers/"+studentID, "", nil)
	e.requireStatus(notFound, http.StatusNotFound)
}
