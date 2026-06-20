package http

import (
	"net/http"

	"github.com/student-learning-portal/backend/internal/domain"
)

// NewRouter creates a new HTTP multiplexer and registers all project routes.
func NewRouter(catalogHandler *CatalogHandler, authHandler *AuthHandler, tokens domain.TokenService) *http.ServeMux {
	mux := http.NewServeMux()

	// Registering the Hello World endpoint
	mux.HandleFunc("/hello", HelloHandler)

	// Database ping endpoint
	mux.HandleFunc("/api/v1/health/db", DBHealthHandler)

	mux.HandleFunc("GET /api/v1/catalog/courses", catalogHandler.GetCourses)

	mux.HandleFunc("POST /api/v1/auth/register", authHandler.Register)
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("GET /api/v1/auth/me", RequireAuth(tokens)(authHandler.Me))

	// TODO: register player, progress and payments endpoints here

	return mux
}
