package security

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/student-learning-portal/backend/internal/domain"
)

var ErrInvalidToken = errors.New("invalid or expired token")

type claims struct {
	Email string      `json:"email"`
	Role  domain.Role `json:"role"`
	jwt.RegisteredClaims
}

// JWTTokenService issues and verifies HMAC-signed JWTs.
type JWTTokenService struct {
	secret []byte
	ttl    time.Duration
}

func NewJWTTokenService(secret string, ttl time.Duration) *JWTTokenService {
	return &JWTTokenService{secret: []byte(secret), ttl: ttl}
}

func (s *JWTTokenService) Generate(user domain.User) (string, error) {
	now := time.Now()
	c := claims{
		Email: user.Email,
		Role:  user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	return token.SignedString(s.secret)
}

func (s *JWTTokenService) Verify(tokenString string) (domain.Claims, error) {
	var c claims
	_, err := jwt.ParseWithClaims(tokenString, &c, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Method.Alg())
		}
		return s.secret, nil
	})
	if err != nil {
		return domain.Claims{}, ErrInvalidToken
	}

	return domain.Claims{
		UserID: c.Subject,
		Email:  c.Email,
		Role:   c.Role,
	}, nil
}
