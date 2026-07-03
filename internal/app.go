package internal

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/student-learning-portal/backend/internal/database"
	delivery "github.com/student-learning-portal/backend/internal/delivery/http"
	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/eventlog"
	"github.com/student-learning-portal/backend/internal/security"
	"github.com/student-learning-portal/backend/internal/usecase"
)

const (
	tokenTTL          = 24 * time.Hour
	serverReadTimeout = 10 * time.Second
	serverIdleTimeout = 120 * time.Second
)

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
	lessonRepo := database.NewPostgresLessonRepository(database.DB)
	catalogUseCase := usecase.NewCatalogUseCase(catalogRepo, lessonRepo)

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable must be set")
	}
	tokens := security.NewJWTTokenService(jwtSecret, tokenTTL)

	userRepo := database.NewPostgresUserRepository(database.DB)
	authUseCase := usecase.NewAuthUseCase(userRepo, tokens)

	entitlementRepo := database.NewPostgresEntitlementRepository(database.DB)
	paymentUseCase := usecase.NewPaymentUseCase(entitlementRepo, catalogRepo, userRepo)

	progressRepo := database.NewPostgresProgressRepository(database.DB)
	playerUseCase := usecase.NewPlayerUseCase(lessonRepo, progressRepo)

	userCoursesUseCase := usecase.NewUserCoursesUseCase(catalogRepo, entitlementRepo)

	resultsRepo := database.NewPostgresResultsRepository(database.DB)
	resultsUseCase := usecase.NewResultsUseCase(resultsRepo)

	uploadsDir := envOrDefault("UPLOADS_DIR", filepath.Join(".", "uploads"))
	//nolint:mnd // 0755 = rwxr-xr-x, standard directory permission
	if err := os.MkdirAll(filepath.Join(uploadsDir, "avatars"), 0o755); err != nil {
		log.Fatalf("failed to create uploads directory: %v", err)
	}

	analyticsRepo := database.NewPostgresAnalyticsRepository(database.DB)
	analyticsUseCase := usecase.NewAnalyticsUseCase(analyticsRepo, catalogRepo, domain.DefaultRiskThresholds)

	handlers := delivery.Handlers{
		Catalog:     delivery.NewCatalogHandler(catalogUseCase),
		Auth:        delivery.NewAuthHandler(authUseCase, analytics),
		Purchase:    delivery.NewPurchaseHandler(paymentUseCase, analytics),
		Player:      delivery.NewPlayerHandler(playerUseCase, analytics),
		UserCourses: delivery.NewUserCoursesHandler(userCoursesUseCase),
		Profile:     delivery.NewProfileHandler(authUseCase, uploadsDir),
		Analytics:   delivery.NewAnalyticsHandler(analyticsUseCase),
		Results:     delivery.NewResultsHandler(resultsUseCase),
	}

	router := delivery.NewRouter(handlers, tokens, entitlementRepo, analytics, uploadsDir)

	port := ":8080"
	log.Printf("Server listening on port %s", port)
	srv := &http.Server{
		Addr:         port,
		Handler:      router,
		ReadTimeout:  serverReadTimeout,
		WriteTimeout: serverReadTimeout,
		IdleTimeout:  serverIdleTimeout,
	}
	if err := srv.ListenAndServe(); err != nil {
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
