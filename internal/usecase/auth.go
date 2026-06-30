package usecase

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/student-learning-portal/backend/internal/domain"
)

const minPasswordLength = 8

var ErrValidation = errors.New("validation error")

type AuthUseCase struct {
	users  domain.UserRepository
	tokens domain.TokenService
}

func NewAuthUseCase(users domain.UserRepository, tokens domain.TokenService) *AuthUseCase {
	return &AuthUseCase{users: users, tokens: tokens}
}

// Register creates a new account and returns a bearer token for it.
func (uc *AuthUseCase) Register(input domain.RegisterInput) (string, domain.User, error) {
	email := normalizeEmail(input.Email)

	if err := validateRegisterInput(email, input.Password, input.FullName, input.Role); err != nil {
		return "", domain.User{}, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return "", domain.User{}, fmt.Errorf("hash password: %w", err)
	}

	created, err := uc.users.Create(domain.User{
		Email:        email,
		PasswordHash: string(hash),
		FullName:     strings.TrimSpace(input.FullName),
		Role:         input.Role,
		AnonymousID:  input.AnonymousID,
	})
	if err != nil {
		return "", domain.User{}, err
	}

	token, err := uc.tokens.Generate(created)
	if err != nil {
		return "", domain.User{}, fmt.Errorf("generate token: %w", err)
	}
	return token, created, nil
}

// Login verifies credentials and returns a bearer token on success.
func (uc *AuthUseCase) Login(input domain.LoginInput) (string, domain.User, error) {
	email := normalizeEmail(input.Email)

	user, err := uc.users.GetByEmail(email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return "", domain.User{}, domain.ErrInvalidLogin
		}
		return "", domain.User{}, err
	}

	if err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)); err != nil {
		return "", domain.User{}, domain.ErrInvalidLogin
	}

	token, err := uc.tokens.Generate(user)
	if err != nil {
		return "", domain.User{}, fmt.Errorf("generate token: %w", err)
	}
	return token, user, nil
}

// CurrentUser resolves the authenticated user from a verified token's claims.
func (uc *AuthUseCase) CurrentUser(claims domain.Claims) (domain.User, error) {
	return uc.users.GetByID(claims.UserID)
}

func validateRegisterInput(email, password, fullName string, role domain.Role) error {
	if _, err := mail.ParseAddress(email); err != nil {
		return fmt.Errorf("%w: invalid email address", ErrValidation)
	}
	if len(password) < minPasswordLength {
		return fmt.Errorf("%w: password must be at least %d characters", ErrValidation, minPasswordLength)
	}
	if strings.TrimSpace(fullName) == "" {
		return fmt.Errorf("%w: full name is required", ErrValidation)
	}
	if !role.Valid() {
		return fmt.Errorf("%w: role must be either %q or %q", ErrValidation, domain.RoleStudent, domain.RoleTeacher)
	}
	return nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
