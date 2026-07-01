package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

// authStubUserRepo implements domain.UserRepository for auth handler tests.
type authStubUserRepo struct {
	user      domain.User
	createErr error
	getErr    error
}

func (s *authStubUserRepo) Create(u domain.User) (domain.User, error) {
	if s.createErr != nil {
		return domain.User{}, s.createErr
	}
	u.ID = "new-id"
	return u, nil
}

func (s *authStubUserRepo) GetByEmail(_ string) (domain.User, error) { return s.user, s.getErr }
func (s *authStubUserRepo) GetByID(_ string) (domain.User, error)    { return s.user, s.getErr }

func (s *authStubUserRepo) DeductBalance(_ context.Context, _ string, _ float64) (float64, error) {
	return 0, nil
}

func (s *authStubUserRepo) CreditBalance(_ context.Context, _ string, _ float64) (float64, error) {
	return 0, nil
}

func (s *authStubUserRepo) UpdateEmail(_ context.Context, _, _ string) (domain.User, error) {
	return s.user, s.getErr
}

func (s *authStubUserRepo) UpdatePasswordHash(_ context.Context, _, _ string) error {
	return s.getErr
}

func (s *authStubUserRepo) UpdateFullName(_ context.Context, _, _ string) (domain.User, error) {
	return s.user, s.getErr
}

func (s *authStubUserRepo) UpdateAvatarURL(_ context.Context, _, _ string) (domain.User, error) {
	return s.user, s.getErr
}

// authStubTokenService implements domain.TokenService for auth handler tests.
type authStubTokenService struct {
	token    string
	tokenErr error
}

func (s *authStubTokenService) Generate(_ domain.User) (string, error) { return s.token, s.tokenErr }
func (s *authStubTokenService) Verify(_ string) (domain.Claims, error) { return domain.Claims{}, nil }

func newAuthHandler(repo *authStubUserRepo, tokens *authStubTokenService) *AuthHandler {
	uc := usecase.NewAuthUseCase(repo, tokens)
	return NewAuthHandler(uc, noopRecorder())
}

func authPostRequest(body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "http://x/", strings.NewReader(body))
}

func authRequestWithClaims(claims domain.Claims) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	return r.WithContext(context.WithValue(r.Context(), claimsContextKey, claims))
}

// --- Register ---

func TestRegisterHandler_Success(t *testing.T) {
	h := newAuthHandler(&authStubUserRepo{}, &authStubTokenService{token: "tok"})
	w := httptest.NewRecorder()
	body := `{"email":"alice@example.com","password":"password1","full_name":"Alice","role":"student"}`
	h.Register(w, authPostRequest(body))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	var resp authResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Token != "tok" {
		t.Errorf("token = %q, want tok", resp.Token)
	}
}

func TestRegisterHandler_InvalidBody(t *testing.T) {
	h := newAuthHandler(&authStubUserRepo{}, &authStubTokenService{})
	w := httptest.NewRecorder()
	h.Register(w, authPostRequest("{bad json"))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestRegisterHandler_ValidationError(t *testing.T) {
	h := newAuthHandler(&authStubUserRepo{}, &authStubTokenService{})
	w := httptest.NewRecorder()
	body := `{"email":"not-an-email","password":"password1","full_name":"Alice","role":"student"}`
	h.Register(w, authPostRequest(body))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestRegisterHandler_EmailTaken(t *testing.T) {
	h := newAuthHandler(&authStubUserRepo{createErr: domain.ErrEmailTaken}, &authStubTokenService{})
	w := httptest.NewRecorder()
	body := `{"email":"alice@example.com","password":"password1","full_name":"Alice","role":"student"}`
	h.Register(w, authPostRequest(body))
	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", w.Code)
	}
}

// --- Login ---

func TestLoginHandler_Success(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("password1"), bcrypt.MinCost)
	user := domain.User{ID: "u1", Email: "alice@example.com", PasswordHash: string(hash)}
	h := newAuthHandler(&authStubUserRepo{user: user}, &authStubTokenService{token: "tok"})

	w := httptest.NewRecorder()
	h.Login(w, authPostRequest(`{"email":"alice@example.com","password":"password1"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp authResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Token != "tok" {
		t.Errorf("token = %q, want tok", resp.Token)
	}
}

func TestLoginHandler_InvalidBody(t *testing.T) {
	h := newAuthHandler(&authStubUserRepo{}, &authStubTokenService{})
	w := httptest.NewRecorder()
	h.Login(w, authPostRequest("{bad json"))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestLoginHandler_WrongCredentials(t *testing.T) {
	h := newAuthHandler(&authStubUserRepo{getErr: domain.ErrUserNotFound}, &authStubTokenService{})
	w := httptest.NewRecorder()
	h.Login(w, authPostRequest(`{"email":"nobody@example.com","password":"password1"}`))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

// --- Me ---

func TestMeHandler_Success(t *testing.T) {
	user := domain.User{ID: "u1", Email: "alice@example.com", Role: domain.RoleStudent}
	h := newAuthHandler(&authStubUserRepo{user: user}, &authStubTokenService{})

	w := httptest.NewRecorder()
	h.Me(w, authRequestWithClaims(domain.Claims{UserID: "u1"}))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp userPayload
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Email != "alice@example.com" {
		t.Errorf("email = %q, want alice@example.com", resp.Email)
	}
}

func TestMeHandler_MissingAuth(t *testing.T) {
	h := newAuthHandler(&authStubUserRepo{}, &authStubTokenService{})
	w := httptest.NewRecorder()
	h.Me(w, httptest.NewRequest(http.MethodGet, "http://x/", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestMeHandler_UserNotFound(t *testing.T) {
	h := newAuthHandler(&authStubUserRepo{getErr: domain.ErrUserNotFound}, &authStubTokenService{})
	w := httptest.NewRecorder()
	h.Me(w, authRequestWithClaims(domain.Claims{UserID: "ghost"}))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}
