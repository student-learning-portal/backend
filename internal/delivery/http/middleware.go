package http

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

type contextKey string

const claimsContextKey contextKey = "claims"

// Analytics event payload keys shared across all handler files in this package.
const (
	keyCourseID = "course_id"
	keyLessonID = "lesson_id"
	keyTxnID    = "txn_id"
)

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
			// Attribute downstream analytics events to the authenticated user.
			ctx = domain.ContextWithActor(ctx, domain.Actor{
				ActorID:   claims.UserID,
				Role:      claims.Role,
				AuthState: domain.AuthStateAuthenticated,
			})
			next(w, r.WithContext(ctx))
		}
	}
}

// RequireEntitlement checks that the authenticated user holds an active access grant
// for the course in the {course_id} path parameter, blocking with 403 otherwise.
// Every decision is written to the audit-grade access_check_log and mirrored to the
// analytics event stream as access.check (plus access.denied on refusal). Must be
// chained after RequireAuth.
func RequireEntitlement(
	entRepo domain.EntitlementRepository,
	analytics *usecase.AnalyticsRecorder,
) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			claims, ok := claimsFromContext(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "missing authentication")
				return
			}

			courseID := r.PathValue(keyCourseID)
			lessonID := r.PathValue(keyLessonID)

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

			analytics.Record(r.Context(), domain.EventAccessCheck, domain.PIINone, map[string]any{
				keyCourseID:   courseID,
				keyLessonID:   lessonID,
				"decision":    decision,
				"deny_reason": denyReason,
			})

			if !allowed {
				analytics.Record(r.Context(), domain.EventAccessDenied, domain.PIINone, map[string]any{
					keyCourseID:     courseID,
					keyLessonID:     lessonID,
					"deny_reason":   denyReason,
					"attempted_via": "player",
				})
				writeError(w, http.StatusForbidden, "access denied: no active entitlement")
				return
			}

			next(w, r)
		}
	}
}
