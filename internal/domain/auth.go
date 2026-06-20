package domain

// RegisterInput carries the fields needed to create a new account.
type RegisterInput struct {
	Email       string
	Password    string
	FullName    string
	Role        Role
	AnonymousID string
}

// LoginInput carries credentials for authenticating an existing account.
type LoginInput struct {
	Email    string
	Password string
}

// Claims is the set of identity facts carried inside an issued token.
type Claims struct {
	UserID string
	Email  string
	Role   Role
}

// TokenService issues and verifies bearer tokens for authenticated users.
type TokenService interface {
	Generate(user User) (string, error)
	Verify(token string) (Claims, error)
}
