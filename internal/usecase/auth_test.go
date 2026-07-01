package usecase

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/student-learning-portal/backend/internal/domain"
)

// stubAuthUserRepo implements domain.UserRepository for auth use-case tests.
type stubAuthUserRepo struct {
	user      domain.User
	createErr error
	getErr    error
}

func (s *stubAuthUserRepo) Create(u domain.User) (domain.User, error) {
	if s.createErr != nil {
		return domain.User{}, s.createErr
	}
	u.ID = "new-id"
	return u, nil
}

func (s *stubAuthUserRepo) GetByEmail(_ string) (domain.User, error) { return s.user, s.getErr }
func (s *stubAuthUserRepo) GetByID(_ string) (domain.User, error)    { return s.user, s.getErr }

func (s *stubAuthUserRepo) DeductBalance(_ context.Context, _ string, _ float64) (float64, error) {
	return 0, nil
}

func (s *stubAuthUserRepo) CreditBalance(_ context.Context, _ string, _ float64) (float64, error) {
	return 0, nil
}

func (s *stubAuthUserRepo) UpdateEmail(_ context.Context, _, _ string) (domain.User, error) {
	return s.user, s.getErr
}

func (s *stubAuthUserRepo) UpdatePasswordHash(_ context.Context, _, _ string) error {
	return s.getErr
}

func (s *stubAuthUserRepo) UpdateFullName(_ context.Context, _, _ string) (domain.User, error) {
	return s.user, s.getErr
}

func (s *stubAuthUserRepo) UpdateAvatarURL(_ context.Context, _, _ string) (domain.User, error) {
	return s.user, s.getErr
}

// stubAuthTokenService implements domain.TokenService for auth use-case tests.
type stubAuthTokenService struct {
	token    string
	tokenErr error
}

func (s *stubAuthTokenService) Generate(_ domain.User) (string, error) { return s.token, s.tokenErr }

func (s *stubAuthTokenService) Verify(_ string) (domain.Claims, error) { return domain.Claims{}, nil }

func hashForTest(t *testing.T, pw string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	return string(h)
}

// --- Register ---

func TestRegister_Success(t *testing.T) {
	uc := NewAuthUseCase(&stubAuthUserRepo{}, &stubAuthTokenService{token: "tok"})
	token, user, err := uc.Register(domain.RegisterInput{
		Email:    "alice@example.com",
		Password: "password1",
		FullName: "Alice Smith",
		Role:     domain.RoleStudent,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if token != "tok" {
		t.Errorf("token = %q, want tok", token)
	}
	if user.Email != "alice@example.com" {
		t.Errorf("email = %q, want alice@example.com", user.Email)
	}
}

func TestRegister_NormalizesEmail(t *testing.T) {
	uc := NewAuthUseCase(&stubAuthUserRepo{}, &stubAuthTokenService{token: "tok"})
	_, user, err := uc.Register(domain.RegisterInput{
		Email:    "  ALICE@EXAMPLE.COM  ",
		Password: "password1",
		FullName: "Alice Smith",
		Role:     domain.RoleStudent,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if user.Email != "alice@example.com" {
		t.Errorf("email = %q, want alice@example.com", user.Email)
	}
}

func TestRegister_InvalidEmail(t *testing.T) {
	uc := NewAuthUseCase(&stubAuthUserRepo{}, &stubAuthTokenService{})
	_, _, err := uc.Register(domain.RegisterInput{
		Email: "not-an-email", Password: "password1", FullName: "Alice", Role: domain.RoleStudent,
	})
	if !errors.Is(err, ErrValidation) {
		t.Errorf("err = %v, want ErrValidation", err)
	}
}

func TestRegister_ShortPassword(t *testing.T) {
	uc := NewAuthUseCase(&stubAuthUserRepo{}, &stubAuthTokenService{})
	_, _, err := uc.Register(domain.RegisterInput{
		Email: "alice@example.com", Password: "short", FullName: "Alice", Role: domain.RoleStudent,
	})
	if !errors.Is(err, ErrValidation) {
		t.Errorf("err = %v, want ErrValidation", err)
	}
}

func TestRegister_PasswordExactlyMinLength(t *testing.T) {
	uc := NewAuthUseCase(&stubAuthUserRepo{}, &stubAuthTokenService{token: "tok"})
	_, _, err := uc.Register(domain.RegisterInput{
		Email: "alice@example.com", Password: "12345678", FullName: "Alice", Role: domain.RoleStudent,
	})
	if err != nil {
		t.Errorf("8-char password should pass: %v", err)
	}
}

func TestRegister_EmptyFullName(t *testing.T) {
	uc := NewAuthUseCase(&stubAuthUserRepo{}, &stubAuthTokenService{})
	_, _, err := uc.Register(domain.RegisterInput{
		Email: "alice@example.com", Password: "password1", FullName: "   ", Role: domain.RoleStudent,
	})
	if !errors.Is(err, ErrValidation) {
		t.Errorf("err = %v, want ErrValidation", err)
	}
}

func TestRegister_InvalidRole(t *testing.T) {
	uc := NewAuthUseCase(&stubAuthUserRepo{}, &stubAuthTokenService{})
	_, _, err := uc.Register(domain.RegisterInput{
		Email: "alice@example.com", Password: "password1", FullName: "Alice", Role: "admin",
	})
	if !errors.Is(err, ErrValidation) {
		t.Errorf("err = %v, want ErrValidation", err)
	}
}

func TestRegister_TeacherRoleValid(t *testing.T) {
	uc := NewAuthUseCase(&stubAuthUserRepo{}, &stubAuthTokenService{token: "tok"})
	_, _, err := uc.Register(domain.RegisterInput{
		Email: "bob@example.com", Password: "password1", FullName: "Bob", Role: domain.RoleTeacher,
	})
	if err != nil {
		t.Errorf("teacher role should be valid: %v", err)
	}
}

func TestRegister_EmailTaken(t *testing.T) {
	uc := NewAuthUseCase(&stubAuthUserRepo{createErr: domain.ErrEmailTaken}, &stubAuthTokenService{})
	_, _, err := uc.Register(domain.RegisterInput{
		Email: "alice@example.com", Password: "password1", FullName: "Alice", Role: domain.RoleStudent,
	})
	if !errors.Is(err, domain.ErrEmailTaken) {
		t.Errorf("err = %v, want ErrEmailTaken", err)
	}
}

// --- Login ---

func TestLogin_Success(t *testing.T) {
	user := domain.User{
		ID:           "u1",
		Email:        "alice@example.com",
		PasswordHash: hashForTest(t, "password1"),
		Role:         domain.RoleStudent,
	}
	uc := NewAuthUseCase(&stubAuthUserRepo{user: user}, &stubAuthTokenService{token: "tok"})

	token, got, err := uc.Login(domain.LoginInput{Email: "alice@example.com", Password: "password1"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if token != "tok" {
		t.Errorf("token = %q, want tok", token)
	}
	if got.ID != "u1" {
		t.Errorf("user.ID = %q, want u1", got.ID)
	}
}

func TestLogin_NormalizesEmail(t *testing.T) {
	user := domain.User{
		Email:        "alice@example.com",
		PasswordHash: hashForTest(t, "password1"),
	}
	uc := NewAuthUseCase(&stubAuthUserRepo{user: user}, &stubAuthTokenService{token: "tok"})
	_, _, err := uc.Login(domain.LoginInput{Email: "  ALICE@EXAMPLE.COM  ", Password: "password1"})
	if err != nil {
		t.Fatalf("Login with upper-case email: %v", err)
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	uc := NewAuthUseCase(&stubAuthUserRepo{getErr: domain.ErrUserNotFound}, &stubAuthTokenService{})
	_, _, err := uc.Login(domain.LoginInput{Email: "nobody@example.com", Password: "password1"})
	if !errors.Is(err, domain.ErrInvalidLogin) {
		t.Errorf("err = %v, want ErrInvalidLogin", err)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	user := domain.User{
		Email:        "alice@example.com",
		PasswordHash: hashForTest(t, "correct-password"),
	}
	uc := NewAuthUseCase(&stubAuthUserRepo{user: user}, &stubAuthTokenService{})
	_, _, err := uc.Login(domain.LoginInput{Email: "alice@example.com", Password: "wrong-password"})
	if !errors.Is(err, domain.ErrInvalidLogin) {
		t.Errorf("err = %v, want ErrInvalidLogin", err)
	}
}

// --- CurrentUser ---

func TestCurrentUser_LooksUpByID(t *testing.T) {
	user := domain.User{ID: "u1", Email: "alice@example.com"}
	uc := NewAuthUseCase(&stubAuthUserRepo{user: user}, &stubAuthTokenService{})
	got, err := uc.CurrentUser(domain.Claims{UserID: "u1"})
	if err != nil {
		t.Fatalf("CurrentUser: %v", err)
	}
	if got.ID != "u1" {
		t.Errorf("ID = %q, want u1", got.ID)
	}
}

func TestCurrentUser_NotFound(t *testing.T) {
	uc := NewAuthUseCase(&stubAuthUserRepo{getErr: domain.ErrUserNotFound}, &stubAuthTokenService{})
	_, err := uc.CurrentUser(domain.Claims{UserID: "ghost"})
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("err = %v, want ErrUserNotFound", err)
	}
}
