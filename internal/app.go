package internal

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/student-learning-portal/backend/internal/database"
	delivery "github.com/student-learning-portal/backend/internal/delivery/http"
	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/eventlog"
	"github.com/student-learning-portal/backend/internal/security"
	"github.com/student-learning-portal/backend/internal/usecase"
)

const tokenTTL = 24 * time.Hour

// Run is the main application assembly point.
// It sets up dependencies, database connections, and starts the HTTP server.
func Run() {
	database.InitDB()

	// Analytics event stream (logging-architecture.md): events fan out to the raw
	// NDJSON transport on stdout and to the Postgres event_log load.
	analytics := usecase.NewAnalyticsRecorder(
		analyticsSource(),
		eventlog.NewNDJSONSink(os.Stdout),
		database.NewPostgresEventSink(database.DB),
	)

	catalogRepo := database.NewPostgresCatalogRepository(database.DB)
	catalogUseCase := usecase.NewCatalogUseCase(catalogRepo)

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable must be set")
	}
	tokens := security.NewJWTTokenService(jwtSecret, tokenTTL)

	userRepo := database.NewPostgresUserRepository(database.DB)
	authUseCase := usecase.NewAuthUseCase(userRepo, tokens)

	entitlementRepo := database.NewPostgresEntitlementRepository(database.DB)
	paymentUseCase := usecase.NewPaymentUseCase(entitlementRepo)

	lessonRepo := database.NewPostgresLessonRepository(database.DB)
	progressRepo := database.NewPostgresProgressRepository(database.DB)
	playerUseCase := usecase.NewPlayerUseCase(lessonRepo, progressRepo)

	handlers := delivery.Handlers{
		Catalog:  delivery.NewCatalogHandler(catalogUseCase),
		Auth:     delivery.NewAuthHandler(authUseCase, analytics),
		Purchase: delivery.NewPurchaseHandler(paymentUseCase, analytics),
		Player:   delivery.NewPlayerHandler(playerUseCase, analytics),
	}

	router := delivery.NewRouter(handlers, tokens, entitlementRepo, analytics)

	port := ":8080"
	log.Printf("Server listening on port %s", port)
	if err := http.ListenAndServe(port, router); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// analyticsSource builds the per-instance source block (§2 source) from the
// environment: APP_ENV (dev|staging|prod), HOSTNAME (instance), and RELEASE
// (the deployed git sha).
func analyticsSource() domain.Source {
	return domain.Source{
		Env:      envOrDefault("APP_ENV", "dev"),
		Instance: envOrDefault("HOSTNAME", "local"),
		Release:  envOrDefault("RELEASE", "unknown"),
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
