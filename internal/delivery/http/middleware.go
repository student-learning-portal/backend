package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/logging"
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

// RequireAdmin restricts a route to administrator accounts. Must be chained
// after RequireAuth. The role is read off the verified token: an admin is only
// ever created by the startup bootstrap, so the claim can't be self-issued.
func RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := claimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing authentication")
			return
		}
		if claims.Role != domain.RoleAdmin {
			writeError(w, http.StatusForbidden, "administrator role required")
			return
		}
		next(w, r)
	}
}

// RequireApprovedTeacher blocks the teacher-only endpoints for a teacher whose
// registration an administrator hasn't confirmed yet. Must be chained after
// RequireAuth.
//
// Approval state is read from the database on every call rather than from the
// token: tokens live for 24h, so a teacher approved (or rejected) mid-session
// would otherwise keep the stale verdict until their next login. Callers that
// aren't teachers pass straight through — the individual handlers still do
// their own role check, and this middleware only answers "is this teacher
// allowed to act as one".
func RequireApprovedTeacher(users domain.UserRepository) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			claims, ok := claimsFromContext(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "missing authentication")
				return
			}
			if claims.Role != domain.RoleTeacher {
				next(w, r)
				return
			}

			user, err := users.GetByID(claims.UserID)
			if err != nil {
				if errors.Is(err, domain.ErrUserNotFound) {
					writeError(w, http.StatusUnauthorized, "account no longer exists")
					return
				}
				logging.FromContext(r.Context()).Error("teacher approval check failed",
					slog.String("actor_id", claims.UserID),
					slog.Any("error", err),
				)
				writeError(w, http.StatusInternalServerError, "approval check failed")
				return
			}

			if !user.TeacherApproved() {
				// The status rides along so the frontend can tell "still
				// waiting" apart from "declined" without a second request.
				writeJSON(w, http.StatusForbidden, map[string]string{
					"error":  domain.ErrTeacherNotApproved.Error(),
					"status": string(user.TeacherStatus),
				})
				return
			}

			next(w, r)
		}
	}
}

// RequireEntitlement checks that the authenticated user holds an active access grant
// for the course in the {course_id} path parameter, blocking with 403 otherwise.
// A course's own teacher is always allowed through, without needing a purchase
// grant, so they can preview the content they author.
// Every decision is written to the audit-grade access_check_log and mirrored to the
// analytics event stream as access.check (plus access.denied on refusal). Must be
// chained after RequireAuth.
func RequireEntitlement( //nolint:gocognit // access-control middleware: teacher-bypass + audit + deny logic reads clearer inline
	entRepo domain.EntitlementRepository,
	catalog domain.CatalogRepository,
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

			if claims.Role == domain.RoleTeacher {
				if course, err := catalog.GetByID(r.Context(), courseID); err == nil && course.TeacherID == claims.UserID {
					next(w, r)
					return
				}
			}

			allowed, err := entRepo.HasActiveGrant(r.Context(), claims.UserID, courseID)
			if err != nil {
				logging.FromContext(r.Context()).Error("entitlement check failed",
					slog.String("course_id", courseID),
					slog.String("lesson_id", lessonID),
					slog.Any("error", err),
				)
				writeError(w, http.StatusInternalServerError, "access check failed")
				return
			}

			decision, denyReason := "allow", ""
			if !allowed {
				decision, denyReason = "deny", "no_active_grant"
			}

			if err := entRepo.LogAccessCheck(r.Context(), domain.AccessCheckLog{
				EventID:    uuid.NewString(),
				ActorID:    claims.UserID,
				CourseID:   courseID,
				LessonID:   lessonID,
				Decision:   decision,
				DenyReason: denyReason,
				CheckedAt:  time.Now(),
			}); err != nil {
				logging.FromContext(r.Context()).Error("failed to write access_check_log",
					slog.String("course_id", courseID),
					slog.String("lesson_id", lessonID),
					slog.String("decision", decision),
					slog.Any("error", err),
				)
			}

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
