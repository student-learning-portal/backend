package http

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
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

// RequireEntitlement checks that the authenticated user holds an active access grant
// for the course in the {course_id} path parameter, blocking with 403 otherwise.
// Every decision is written to access_check_log. Must be chained after RequireAuth.
func RequireEntitlement(entRepo domain.EntitlementRepository) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			claims, ok := claimsFromContext(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "missing authentication")
				return
			}

			courseID := r.PathValue("course_id")
			lessonID := r.PathValue("lesson_id")

			allowed, err := entRepo.HasActiveGrant(r.Context(), claims.UserID, courseID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "access check failed")
				return
			}

			decision, denyReason := "allow", ""
			if !allowed {
				decision, denyReason = "deny", "no_active_grant"
			}

			_ = entRepo.LogAccessCheck(r.Context(), domain.AccessCheckLog{
				EventID:    uuid.NewString(),
				ActorID:    claims.UserID,
				CourseID:   courseID,
				LessonID:   lessonID,
				Decision:   decision,
				DenyReason: denyReason,
				CheckedAt:  time.Now(),
			})

			if !allowed {
				writeError(w, http.StatusForbidden, "access denied: no active entitlement")
				return
			}

			next(w, r)
		}
	}
}
