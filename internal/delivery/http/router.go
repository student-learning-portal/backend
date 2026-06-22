package http

import (
	"net/http"

	"github.com/student-learning-portal/backend/internal/domain"
)

// NewRouter creates a new HTTP multiplexer and registers all project routes.
func NewRouter(
	catalogHandler *CatalogHandler,
	authHandler *AuthHandler,
	purchaseHandler *PurchaseHandler,
	playerHandler *PlayerHandler,
	tokens domain.TokenService,
	entitlements domain.EntitlementRepository,
) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/hello", HelloHandler)
	mux.HandleFunc("/api/v1/health/db", DBHealthHandler)

	mux.HandleFunc("GET /api/v1/catalog/courses", catalogHandler.GetCourses)

	mux.HandleFunc("POST /api/v1/auth/register", authHandler.Register)
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("GET /api/v1/auth/me", RequireAuth(tokens)(authHandler.Me))

	auth := RequireAuth(tokens)
	guard := RequireEntitlement(entitlements)

	mux.HandleFunc("POST /api/v1/purchase/checkout", auth(purchaseHandler.Checkout))
	mux.HandleFunc("POST /api/v1/purchase/webhook", purchaseHandler.Webhook)

	mux.HandleFunc(
		"GET /api/v1/player/courses/{course_id}/lessons/{lesson_id}",
		auth(guard(playerHandler.GetLesson)),
	)
	mux.HandleFunc(
		"POST /api/v1/player/courses/{course_id}/lessons/{lesson_id}/progress",
		auth(guard(playerHandler.SaveProgress)),
	)
	mux.HandleFunc(
		"GET /api/v1/player/courses/{course_id}/lessons/{lesson_id}/progress",
		auth(guard(playerHandler.GetProgress)),
	)

	return mux
}
