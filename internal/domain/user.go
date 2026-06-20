package domain

import (
	"errors"
	"time"
)

type Role string

const (
	RoleStudent Role = "student"
	RoleTeacher Role = "teacher"
)

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
}

var (
	ErrUserNotFound = errors.New("user not found")
	ErrEmailTaken   = errors.New("email already registered")
	ErrInvalidLogin = errors.New("invalid email or password")
)

// UserRepository persists and retrieves user accounts.
type UserRepository interface {
	Create(user User) (User, error)
	GetByEmail(email string) (User, error)
	GetByID(id string) (User, error)
}
