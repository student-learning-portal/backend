package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

// middlewareTokenService implements domain.TokenService for middleware tests.
type middlewareTokenService struct {
	claims    domain.Claims
	verifyErr error
}

func (s *middlewareTokenService) Generate(_ domain.User) (string, error) { return "tok", nil }

func (s *middlewareTokenService) Verify(_ string) (domain.Claims, error) {
	return s.claims, s.verifyErr
}

// middlewareEntRepo implements domain.EntitlementRepository for middleware tests.
type middlewareEntRepo struct {
	hasGrant bool
	grantErr error
}

func (s *middlewareEntRepo) CreatePayment(_ context.Context, _ domain.Payment) error { return nil }

func (s *middlewareEntRepo) GetPayment(_ context.Context, _ string) (domain.Payment, error) {
	return domain.Payment{}, nil
}

func (s *middlewareEntRepo) UpdatePaymentStatus(_ context.Context, _, _ string) error  { return nil }
func (s *middlewareEntRepo) CreateGrant(_ context.Context, _ domain.AccessGrant) error { return nil }
func (s *middlewareEntRepo) RevokeGrant(_ context.Context, _, _ string) error          { return nil }

func (s *middlewareEntRepo) HasActiveGrant(_ context.Context, _, _ string) (bool, error) {
	return s.hasGrant, s.grantErr
}

func (s *middlewareEntRepo) GetActiveGrant(_ context.Context, _, _ string) (domain.AccessGrant, error) {
	return domain.AccessGrant{}, nil
}

func (s *middlewareEntRepo) LogAccessCheck(_ context.Context, _ domain.AccessCheckLog) error {
	return nil
}

func (s *middlewareEntRepo) GetEnrolledCourses(_ context.Context, _ string) ([]domain.Course, error) {
	return nil, nil
}

// nextCapture records whether the next handler was called.
type nextCapture struct{ called bool }

func (n *nextCapture) handler(w http.ResponseWriter, _ *http.Request) {
	n.called = true
	w.WriteHeader(http.StatusOK)
}

// --- RequireAuth ---

func TestRequireAuth_MissingBearerToken(t *testing.T) {
	svc := &middlewareTokenService{claims: domain.Claims{UserID: "u1"}}
	mw := RequireAuth(svc)
	next := &nextCapture{}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	mw(next.handler)(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
	if next.called {
		t.Error("next handler must not be called without token")
	}
}

func TestRequireAuth_EmptyBearerValue(t *testing.T) {
	svc := &middlewareTokenService{claims: domain.Claims{UserID: "u1"}}
	mw := RequireAuth(svc)
	next := &nextCapture{}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	r.Header.Set("Authorization", "Bearer ")
	mw(next.handler)(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestRequireAuth_InvalidToken(t *testing.T) {
	svc := &middlewareTokenService{verifyErr: domain.ErrForbidden}
	mw := RequireAuth(svc)
	next := &nextCapture{}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	r.Header.Set("Authorization", "Bearer bad.token.here")
	mw(next.handler)(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
	if next.called {
		t.Error("next handler must not be called with invalid token")
	}
}

func TestRequireAuth_ValidToken_InjectsClaimsAndCallsNext(t *testing.T) {
	claims := domain.Claims{UserID: "u1", Email: "alice@example.com", Role: domain.RoleStudent}
	svc := &middlewareTokenService{claims: claims}
	mw := RequireAuth(svc)

	var captured domain.Claims
	handler := func(_ http.ResponseWriter, r *http.Request) {
		captured, _ = claimsFromContext(r.Context())
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	r.Header.Set("Authorization", "Bearer valid.token.here")
	mw(handler)(w, r)

	if captured.UserID != "u1" {
		t.Errorf("UserID = %q, want u1", captured.UserID)
	}
	if captured.Email != "alice@example.com" {
		t.Errorf("Email = %q, want alice@example.com", captured.Email)
	}
}

func TestRequireAuth_ValidToken_SetsActorInContext(t *testing.T) {
	claims := domain.Claims{UserID: "u1", Role: domain.RoleTeacher}
	svc := &middlewareTokenService{claims: claims}
	mw := RequireAuth(svc)

	var actor domain.Actor
	var ok bool
	handler := func(_ http.ResponseWriter, r *http.Request) {
		actor, ok = domain.ActorFromContext(r.Context())
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	r.Header.Set("Authorization", "Bearer valid.token")
	mw(handler)(w, r)

	if !ok {
		t.Fatal("actor not set in context")
	}
	if actor.ActorID != "u1" {
		t.Errorf("actor.ActorID = %q, want u1", actor.ActorID)
	}
	if actor.AuthState != domain.AuthStateAuthenticated {
		t.Errorf("actor.AuthState = %q, want authenticated", actor.AuthState)
	}
}

// --- RequireEntitlement ---

func entitlementRequest(courseID, lessonID string, claims domain.Claims) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	r = r.WithContext(context.WithValue(r.Context(), claimsContextKey, claims))
	r.SetPathValue(keyCourseID, courseID)
	r.SetPathValue(keyLessonID, lessonID)
	return r
}

func TestRequireEntitlement_MissingClaims(t *testing.T) {
	mw := RequireEntitlement(&middlewareEntRepo{hasGrant: true}, noopRecorder())
	next := &nextCapture{}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	mw(next.handler)(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
	if next.called {
		t.Error("next must not be called without claims")
	}
}

func TestRequireEntitlement_AccessGranted_CallsNext(t *testing.T) {
	mw := RequireEntitlement(&middlewareEntRepo{hasGrant: true}, noopRecorder())
	next := &nextCapture{}

	w := httptest.NewRecorder()
	r := entitlementRequest("c1", "l1", domain.Claims{UserID: "u1"})
	mw(next.handler)(w, r)

	if !next.called {
		t.Error("next handler must be called when grant exists")
	}
}

func TestRequireEntitlement_NoGrant_Returns403(t *testing.T) {
	mw := RequireEntitlement(&middlewareEntRepo{hasGrant: false}, noopRecorder())
	next := &nextCapture{}

	w := httptest.NewRecorder()
	r := entitlementRequest("c1", "l1", domain.Claims{UserID: "u1"})
	mw(next.handler)(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
	if next.called {
		t.Error("next must not be called when grant is denied")
	}
}

func TestRequireEntitlement_GrantCheckError_Returns500(t *testing.T) {
	mw := RequireEntitlement(&middlewareEntRepo{grantErr: domain.ErrForbidden}, usecase.NewAnalyticsRecorder(domain.Source{}))
	next := &nextCapture{}

	w := httptest.NewRecorder()
	r := entitlementRequest("c1", "l1", domain.Claims{UserID: "u1"})
	mw(next.handler)(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
