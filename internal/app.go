package internal

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/student-learning-portal/backend/internal/database"
	delivery "github.com/student-learning-portal/backend/internal/delivery/http"
	"github.com/student-learning-portal/backend/internal/security"
	"github.com/student-learning-portal/backend/internal/usecase"
)

const tokenTTL = 24 * time.Hour

// Run is the main application assembly point.
// It sets up dependencies, database connections, and starts the HTTP server.
func Run() {
	// Initialize Database
	database.InitDB()

	// Initialize Use Cases backed by the database
	catalogRepo := database.NewPostgresCatalogRepository(database.DB)
	catalogUseCase := usecase.NewCatalogUseCase(catalogRepo)

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable must be set")
	}
	tokens := security.NewJWTTokenService(jwtSecret, tokenTTL)

	userRepo := database.NewPostgresUserRepository(database.DB)
	authUseCase := usecase.NewAuthUseCase(userRepo, tokens)

	// Initialize the HTTP handlers and inject use cases
	catalogHandler := delivery.NewCatalogHandler(catalogUseCase)
	authHandler := delivery.NewAuthHandler(authUseCase)

	// Initialize HTTP Router and inject the handlers
	router := delivery.NewRouter(catalogHandler, authHandler, tokens)

	port := ":8080"
	log.Printf("Server listening on port %s", port)
	if err := http.ListenAndServe(port, router); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
