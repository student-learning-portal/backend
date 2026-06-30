package http

import (
	"net/http"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

// Handlers groups the per-domain HTTP handlers so the router assembly stays a
// single dependency bundle rather than a long positional parameter list.
type Handlers struct {
	Catalog     *CatalogHandler
	Auth        *AuthHandler
	Purchase    *PurchaseHandler
	Player      *PlayerHandler
	UserCourses *UserCoursesHandler
	Profile     *ProfileHandler
	Analytics   *AnalyticsHandler
}

// NewRouter creates a new HTTP multiplexer and registers all project routes.
// The returned handler is wrapped in WithLogContext so every request carries the
// request-scoped logging context the analytics recorder reads.
func NewRouter(
	h Handlers,
	tokens domain.TokenService,
	entitlements domain.EntitlementRepository,
	analytics *usecase.AnalyticsRecorder,
	uploadsDir string,
) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/hello", HelloHandler)
	mux.HandleFunc("/api/v1/health/db", DBHealthHandler)

	mux.HandleFunc("GET /api/v1/catalog/courses", h.Catalog.GetCourses)
	mux.HandleFunc("GET /api/v1/catalog/courses/{course_id}/lessons", h.Catalog.GetCourseLessons)

	mux.HandleFunc("POST /api/v1/auth/register", h.Auth.Register)
	mux.HandleFunc("POST /api/v1/auth/login", h.Auth.Login)
	mux.HandleFunc("GET /api/v1/auth/me", RequireAuth(tokens)(h.Auth.Me))

	mux.HandleFunc("GET /api/v1/teachers/{teacher_id}", h.Auth.GetTeacher)

	auth := RequireAuth(tokens)
	guard := RequireEntitlement(entitlements, analytics)

	mux.HandleFunc("GET /api/v1/users/me/courses", auth(h.UserCourses.MyCourses))

	mux.HandleFunc("PATCH /api/v1/users/me/email", auth(h.Profile.PatchEmail))
	mux.HandleFunc("PATCH /api/v1/users/me/password", auth(h.Profile.PatchPassword))
	mux.HandleFunc("PATCH /api/v1/users/me/name", auth(h.Profile.PatchName))
	mux.HandleFunc("POST /api/v1/users/me/avatar", auth(h.Profile.PostAvatar))

	mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(uploadsDir))))

	mux.HandleFunc("POST /api/v1/purchase/checkout", auth(h.Purchase.Checkout))
	mux.HandleFunc("POST /api/v1/purchase/refund", auth(h.Purchase.Refund))
	mux.HandleFunc("POST /api/v1/purchase/webhook", h.Purchase.Webhook)

	mux.HandleFunc(
		"GET /api/v1/player/courses/{course_id}/lessons/{lesson_id}",
		auth(guard(h.Player.GetLesson)),
	)
	mux.HandleFunc(
		"POST /api/v1/player/courses/{course_id}/lessons/{lesson_id}/progress",
		auth(guard(h.Player.SaveProgress)),
	)
	mux.HandleFunc(
		"GET /api/v1/player/courses/{course_id}/lessons/{lesson_id}/progress",
		auth(guard(h.Player.GetProgress)),
	)

	// Teacher analytics: ownership + role are enforced inside the handler.
	mux.HandleFunc("GET /api/v1/analytics/teacher/dashboard", auth(h.Analytics.TeacherDashboard))

	return WithLogContext(mux)
}
