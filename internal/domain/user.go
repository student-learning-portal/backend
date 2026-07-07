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
)

// Valid reports whether r is one of the known roles; used to validate
// registration input instead of trusting arbitrary client-supplied strings.
func (r Role) Valid() bool {
	return r == RoleStudent || r == RoleTeacher
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
}

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrEmailTaken        = errors.New("email already registered")
	ErrInvalidLogin      = errors.New("invalid email or password")
	ErrInsufficientFunds = errors.New("insufficient wallet balance")
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
