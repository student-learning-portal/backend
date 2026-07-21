package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/student-learning-portal/backend/internal/domain"
)

// AdminUseCase implements the administrator's moderation surface: the queue of
// teacher registrations awaiting confirmation, the approve/reject decision on
// each, and the startup bootstrap that guarantees an admin account exists at
// all (there is no way to register one — see domain.Role.Valid).
type AdminUseCase struct {
	approvals domain.TeacherApprovalRepository
}

func NewAdminUseCase(approvals domain.TeacherApprovalRepository) *AdminUseCase {
	return &AdminUseCase{approvals: approvals}
}

// Teachers lists teacher accounts filtered by queue state; an empty status
// returns every teacher, which is what the admin's "all applications" view uses.
func (uc *AdminUseCase) Teachers(ctx context.Context, status domain.TeacherStatus) ([]domain.User, error) {
	if status != "" && !status.Valid() {
		return nil, fmt.Errorf("%w: unknown teacher status %q", ErrValidation, status)
	}
	return uc.approvals.ListTeachersByStatus(ctx, status)
}

// ApproveTeacher confirms the teacher role, opening the authoring endpoints for
// that account.
func (uc *AdminUseCase) ApproveTeacher(ctx context.Context, teacherID, adminID string) (domain.User, error) {
	return uc.decide(ctx, teacherID, adminID, domain.TeacherStatusApproved)
}

// RejectTeacher declines the application. The account is left in place (the
// person can still use the portal as a signed-in user and the decision stays
// reversible via ApproveTeacher) but the teacher endpoints remain closed.
func (uc *AdminUseCase) RejectTeacher(ctx context.Context, teacherID, adminID string) (domain.User, error) {
	return uc.decide(ctx, teacherID, adminID, domain.TeacherStatusRejected)
}

func (uc *AdminUseCase) decide(
	ctx context.Context, teacherID, adminID string, status domain.TeacherStatus,
) (domain.User, error) {
	if strings.TrimSpace(teacherID) == "" {
		return domain.User{}, fmt.Errorf("%w: teacher id is required", ErrValidation)
	}
	return uc.approvals.SetTeacherStatus(ctx, teacherID, status, adminID)
}

// EnsureAdminAccount creates the configured administrator on startup if it is
// missing, and reports whether it had to. The login is stored in the email
// column but is deliberately not validated as an email address: the operator
// configures a plain login like "admin", and only registration (which can never
// produce an admin) enforces the address format.
//
// An existing admin row is never rewritten, so a password changed on purpose
// survives the next restart.
func (uc *AdminUseCase) EnsureAdminAccount(ctx context.Context, login, password, fullName string) (bool, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" || password == "" {
		return false, fmt.Errorf("%w: admin login and password are required", ErrValidation)
	}
	if len(password) < minPasswordLength {
		return false, fmt.Errorf("%w: admin password must be at least %d characters", ErrValidation, minPasswordLength)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return false, fmt.Errorf("hash admin password: %w", err)
	}

	_, created, err := uc.approvals.EnsureAdmin(ctx, login, string(hash), strings.TrimSpace(fullName))
	if err != nil {
		if errors.Is(err, domain.ErrLoginTaken) {
			return false, fmt.Errorf("admin login %q is already used by a non-admin account: %w", login, err)
		}
		return false, err
	}
	return created, nil
}
