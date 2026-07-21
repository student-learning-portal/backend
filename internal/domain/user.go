package domain

import (
	"context"
	"errors"
	"time"
)

// Role is an account's permission level; it gates registration input and
// which endpoints/actions (e.g. teacher content authoring) a caller may use.
type Role string

const (
	RoleStudent Role = "student"
	RoleTeacher Role = "teacher"
	// RoleAdmin moderates the portal (it reviews teacher registrations). It is
	// deliberately absent from Valid below: an admin account is never created
	// through the public registration endpoint, only by the startup bootstrap
	// in usecase.AdminUseCase.EnsureAdminAccount.
	RoleAdmin Role = "admin"
)

// Valid reports whether r is a role a client may pick when registering; used to
// validate registration input instead of trusting arbitrary client-supplied
// strings. RoleAdmin is excluded on purpose — accepting it here would let
// anyone mint themselves an administrator through POST /auth/register.
func (r Role) Valid() bool {
	return r == RoleStudent || r == RoleTeacher
}

// TeacherStatus is where a teacher account sits in the administrator's approval
// queue. It is empty for students and admins, which have nothing to review.
type TeacherStatus string

const (
	// TeacherStatusPending is the state every freshly registered teacher starts
	// in: the account exists and can sign in, but the teacher-only endpoints
	// stay closed until an administrator confirms the role.
	TeacherStatusPending  TeacherStatus = "pending"
	TeacherStatusApproved TeacherStatus = "approved"
	TeacherStatusRejected TeacherStatus = "rejected"
)

// Valid reports whether s is a decision an administrator may record.
func (s TeacherStatus) Valid() bool {
	return s == TeacherStatusPending || s == TeacherStatusApproved || s == TeacherStatusRejected
}

// User represents an account in the system.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	FullName     string
	Role         Role
	// AnonymousID links this account to its pre-auth anonymous_id from event_log,
	// captured at signup. May be empty if the client didn't provide one.
	AnonymousID string
	CreatedAt   time.Time
	UpdatedAt   time.Time

	// Balance is the user's sandbox wallet balance (virtual money, no real
	// funds). New accounts start with a default grant so they can exercise
	// the purchase flow.
	Balance float64

	// AvatarURL is the server-relative path to the user's profile picture,
	// e.g. "/uploads/avatars/{id}.jpg". Empty when no avatar has been set.
	AvatarURL string

	// TeacherStatus is the account's position in the approval queue; only ever
	// set for Role == RoleTeacher (see TeacherStatus).
	TeacherStatus TeacherStatus
	// TeacherStatusUpdatedAt is when an administrator last decided on this
	// account, or the signup time for a still-pending one. Zero for non-teachers.
	TeacherStatusUpdatedAt time.Time
}

// TeacherApproved reports whether u may use the teacher-only endpoints: they
// must both hold the teacher role and have been confirmed by an administrator.
func (u User) TeacherApproved() bool {
	return u.Role == RoleTeacher && u.TeacherStatus == TeacherStatusApproved
}

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrEmailTaken        = errors.New("email already registered")
	ErrInvalidLogin      = errors.New("invalid email or password")
	ErrInsufficientFunds = errors.New("insufficient wallet balance")
	// ErrTeacherNotApproved is returned when a teacher whose registration is
	// still pending (or was rejected) tries to use a teacher-only endpoint.
	ErrTeacherNotApproved = errors.New("teacher account is awaiting administrator approval")
	// ErrNotTeacher is returned when an approval decision targets an account
	// that does not hold the teacher role.
	ErrNotTeacher = errors.New("account is not a teacher")
	// ErrLoginTaken is returned by the admin bootstrap when the configured
	// administrator login already belongs to a non-admin account.
	ErrLoginTaken = errors.New("login already belongs to a non-admin account")
)

// UserRepository persists and retrieves user accounts.
type UserRepository interface {
	Create(user User) (User, error)
	GetByEmail(email string) (User, error)
	GetByID(id string) (User, error)

	// DeductBalance atomically subtracts amount from the user's wallet and
	// returns the resulting balance. It returns ErrInsufficientFunds if the
	// balance would go negative.
	DeductBalance(ctx context.Context, userID string, amount float64) (float64, error)
	// CreditBalance atomically adds amount to the user's wallet (e.g. on
	// refund) and returns the resulting balance.
	CreditBalance(ctx context.Context, userID string, amount float64) (float64, error)

	UpdateEmail(ctx context.Context, userID, newEmail string) (User, error)
	UpdatePasswordHash(ctx context.Context, userID, newHash string) error
	UpdateFullName(ctx context.Context, userID, fullName string) (User, error)
	UpdateAvatarURL(ctx context.Context, userID, avatarURL string) (User, error)
}

// TeacherApprovalRepository is the administrator-facing slice of the users
// table: the teacher approval queue plus the bootstrap that guarantees an admin
// account exists. Kept separate from UserRepository so the request-path use
// cases (and their test doubles) don't have to carry moderation methods.
type TeacherApprovalRepository interface {
	// ListTeachersByStatus returns teacher accounts in the given queue state,
	// newest registration first. An empty status returns every teacher.
	ListTeachersByStatus(ctx context.Context, status TeacherStatus) ([]User, error)
	// SetTeacherStatus records an administrator's decision on a teacher
	// account, returning ErrUserNotFound for an unknown id and ErrNotTeacher
	// when the target holds a different role.
	SetTeacherStatus(ctx context.Context, userID string, status TeacherStatus, reviewerID string) (User, error)
	// EnsureAdmin creates the administrator account if it is missing and
	// reports whether it did. An existing admin is returned untouched — the
	// bootstrap must never silently reset a password that was changed on
	// purpose — and a non-admin account on the same login is ErrLoginTaken.
	EnsureAdmin(ctx context.Context, login, passwordHash, fullName string) (User, bool, error)
}
