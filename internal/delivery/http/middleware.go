package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/student-learning-portal/backend/internal/domain"
)

type contextKey string

const claimsContextKey contextKey = "claims"

func claimsFromContext(ctx context.Context) (domain.Claims, bool) {
	claims, ok := ctx.Value(claimsContextKey).(domain.Claims)
	return claims, ok
}

// RequireAuth verifies the request's bearer token and injects its claims into
// the request context, rejecting the request with 401 if absent or invalid.
func RequireAuth(tokens domain.TokenService) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			token, ok := strings.CutPrefix(header, "Bearer ")
			if !ok || token == "" {
				writeError(w, http.StatusUnauthorized, "missing bearer token")
				return
			}

			claims, err := tokens.Verify(token)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), claimsContextKey, claims)
			next(w, r.WithContext(ctx))
		}
	}
}
