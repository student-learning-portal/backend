package security

import (
	"errors"
	"testing"
	"time"

	"github.com/student-learning-portal/backend/internal/domain"
)

func newTestService(ttl time.Duration) *JWTTokenService {
	return NewJWTTokenService("test-secret-key-for-unit-tests", ttl)
}

func newTestUser() domain.User {
	return domain.User{ID: "user-1", Email: "alice@example.com", Role: domain.RoleStudent}
}

func TestGenerateAndVerify_RoundTrip(t *testing.T) {
	svc := newTestService(time.Hour)
	token, err := svc.Generate(newTestUser())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if token == "" {
		t.Fatal("Generate returned empty token")
	}

	claims, err := svc.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.UserID != "user-1" {
		t.Errorf("UserID = %q, want user-1", claims.UserID)
	}
	if claims.Email != "alice@example.com" {
		t.Errorf("Email = %q, want alice@example.com", claims.Email)
	}
	if claims.Role != domain.RoleStudent {
		t.Errorf("Role = %q, want student", claims.Role)
	}
}

func TestGenerateAndVerify_TeacherRole(t *testing.T) {
	svc := newTestService(time.Hour)
	user := domain.User{ID: "t-1", Email: "bob@example.com", Role: domain.RoleTeacher}
	token, err := svc.Generate(user)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	claims, err := svc.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Role != domain.RoleTeacher {
		t.Errorf("Role = %q, want teacher", claims.Role)
	}
	if claims.UserID != "t-1" {
		t.Errorf("UserID = %q, want t-1", claims.UserID)
	}
}

func TestVerify_ExpiredToken(t *testing.T) {
	svc := newTestService(-time.Second)
	token, err := svc.Generate(newTestUser())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	_, err = svc.Verify(token)
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("err = %v, want ErrInvalidToken", err)
	}
}

func TestVerify_WrongSecret(t *testing.T) {
	svc1 := NewJWTTokenService("secret-a", time.Hour)
	svc2 := NewJWTTokenService("secret-b", time.Hour)

	token, _ := svc1.Generate(newTestUser())
	_, err := svc2.Verify(token)
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("err = %v, want ErrInvalidToken", err)
	}
}

func TestVerify_GarbageToken(t *testing.T) {
	svc := newTestService(time.Hour)
	_, err := svc.Verify("not.a.valid.token")
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("err = %v, want ErrInvalidToken", err)
	}
}

func TestVerify_EmptyToken(t *testing.T) {
	svc := newTestService(time.Hour)
	_, err := svc.Verify("")
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("err = %v, want ErrInvalidToken", err)
	}
}

func TestVerify_TamperedToken(t *testing.T) {
	svc := newTestService(time.Hour)
	token, _ := svc.Generate(newTestUser())
	tampered := token[:len(token)-4] + "xxxx"
	_, err := svc.Verify(tampered)
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("err = %v, want ErrInvalidToken", err)
	}
}
