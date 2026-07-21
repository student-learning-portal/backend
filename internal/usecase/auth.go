package usecase

import (
	"context"
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

	// A teacher signs up into the administrator's review queue rather than
	// straight into the role: the account is created and can sign in, but
	// RequireApprovedTeacher keeps the authoring endpoints shut until an admin
	// confirms it. Students need no review, so their status stays empty.
	var teacherStatus domain.TeacherStatus
	if input.Role == domain.RoleTeacher {
		teacherStatus = domain.TeacherStatusPending
	}

	created, err := uc.users.Create(domain.User{
		Email:         email,
		PasswordHash:  string(hash),
		FullName:      strings.TrimSpace(input.FullName),
		Role:          input.Role,
		AnonymousID:   input.AnonymousID,
		TeacherStatus: teacherStatus,
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

// ChangeEmail verifies the caller's current password, then updates their email.
func (uc *AuthUseCase) ChangeEmail(ctx context.Context, userID, currentPassword, newEmail string) (domain.User, error) {
	user, err := uc.users.GetByID(userID)
	if err != nil {
		return domain.User{}, err
	}
	if err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		return domain.User{}, domain.ErrInvalidLogin
	}
	email := normalizeEmail(newEmail)
	if _, err = mail.ParseAddress(email); err != nil {
		return domain.User{}, fmt.Errorf("%w: invalid email address", ErrValidation)
	}
	return uc.users.UpdateEmail(ctx, userID, email)
}

// ChangePassword verifies the caller's current password, then replaces it.
func (uc *AuthUseCase) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	user, err := uc.users.GetByID(userID)
	if err != nil {
		return err
	}
	if err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		return domain.ErrInvalidLogin
	}
	if len(newPassword) < minPasswordLength {
		return fmt.Errorf("%w: password must be at least %d characters", ErrValidation, minPasswordLength)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	return uc.users.UpdatePasswordHash(ctx, userID, string(hash))
}

// ChangeName updates the caller's display name.
func (uc *AuthUseCase) ChangeName(ctx context.Context, userID, fullName string) (domain.User, error) {
	if strings.TrimSpace(fullName) == "" {
		return domain.User{}, fmt.Errorf("%w: full name is required", ErrValidation)
	}
	return uc.users.UpdateFullName(ctx, userID, strings.TrimSpace(fullName))
}

// ChangeAvatar stores a new avatar URL for the caller.
func (uc *AuthUseCase) ChangeAvatar(ctx context.Context, userID, avatarURL string) (domain.User, error) {
	return uc.users.UpdateAvatarURL(ctx, userID, avatarURL)
}

// GetTeacherByID fetches a user by id and returns ErrUserNotFound if the
// account does not exist or is not a teacher (prevents role enumeration).
func (uc *AuthUseCase) GetTeacherByID(id string) (domain.User, error) {
	user, err := uc.users.GetByID(id)
	if err != nil {
		return domain.User{}, err
	}
	if user.Role != domain.RoleTeacher {
		return domain.User{}, domain.ErrUserNotFound
	}
	return user, nil
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
