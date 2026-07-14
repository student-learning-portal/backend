package internal

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/student-learning-portal/backend/internal/database"
	delivery "github.com/student-learning-portal/backend/internal/delivery/http"
	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/eventlog"
	"github.com/student-learning-portal/backend/internal/logging"
	"github.com/student-learning-portal/backend/internal/practicum"
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
	logging.Init("portal")
	logging.L().Info("starting portal backend")

	database.InitDB()

	analyticsRepo := database.NewPostgresAnalyticsRepository(database.DB)

	// Analytics event stream (logging-architecture.md): events fan out to the raw
	// NDJSON transport on stdout, the Postgres event_log load, and a point rollup
	// refresh that keeps a learner's own analytics_student_course row close to
	// real-time instead of waiting for the periodic batch loader
	// (analytics-ml-layer.md §2; cmd/analytics-loader remains the reconciliation pass).
	analytics := usecase.NewAnalyticsRecorder(
		analyticsSource(),
		eventlog.NewNDJSONSink(os.Stdout),
		database.NewPostgresEventSink(database.DB),
		usecase.NewRollupRefreshSink(analyticsRepo),
	)

	catalogRepo := database.NewPostgresCatalogRepository(database.DB)
	lessonRepo := database.NewPostgresLessonRepository(database.DB)
	catalogUseCase := usecase.NewCatalogUseCase(catalogRepo, lessonRepo)

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		logging.L().Error("JWT_SECRET environment variable must be set")
		os.Exit(1)
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
	resultsUseCase := usecase.NewResultsUseCase(resultsRepo, domain.DefaultRiskThresholds)

	chatRepo := database.NewPostgresChatRepository(database.DB)
	chatUseCase := usecase.NewChatUseCase(chatRepo, catalogRepo, entitlementRepo)

	uploadsDir := envOrDefault("UPLOADS_DIR", filepath.Join(".", "uploads"))
	//nolint:mnd // 0755 = rwxr-xr-x, standard directory permission
	if err := os.MkdirAll(filepath.Join(uploadsDir, "avatars"), 0o755); err != nil {
		logging.L().Error("failed to create uploads directory",
			slog.String("uploads_dir", uploadsDir),
			slog.Any("error", err),
		)
		os.Exit(1)
	}

	analyticsUseCase := usecase.NewAnalyticsUseCase(analyticsRepo, catalogRepo, domain.DefaultRiskThresholds)

	// Course ratings/comments proxy to the practicum-team service rather than
	// reimplementing their enrollment/progress-gated review logic locally —
	// see internal/practicum. PRACTICUM_JWT_SECRET must equal *their*
	// JWT_SECRET (a trust relationship between two independently deployed
	// services, not a copy of our own auth secret) and
	// PRACTICUM_INTEGRATION_TEACHER_ID must be a teacher account already
	// registered on their side (their POST /auth/register_teacher, done once).
	practicumClient := practicum.NewClient(
		envOrDefault("PRACTICUM_API_URL", "http://10.93.27.25:8000/api/v1"),
		os.Getenv("PRACTICUM_JWT_SECRET"),
	)
	reviewRepo := practicum.NewReviewRepository(practicumClient, catalogRepo, os.Getenv("PRACTICUM_INTEGRATION_TEACHER_ID"))
	reviewUseCase := usecase.NewReviewUseCase(reviewRepo)

	// Local 1-10 rating system for courses and teachers, stored in our own
	// database — separate from the practicum-proxied review above, which has
	// no notion of teachers at all (see internal/practicum).
	courseRatingRepo := database.NewPostgresCourseRatingRepository(database.DB)
	teacherRatingRepo := database.NewPostgresTeacherRatingRepository(database.DB)
	ratingUseCase := usecase.NewRatingUseCase(courseRatingRepo, teacherRatingRepo, catalogRepo, entitlementRepo, userRepo)

	handlers := delivery.Handlers{
		Catalog:        delivery.NewCatalogHandler(catalogUseCase),
		Auth:           delivery.NewAuthHandler(authUseCase, analytics),
		Purchase:       delivery.NewPurchaseHandler(paymentUseCase, analytics),
		Player:         delivery.NewPlayerHandler(playerUseCase, analytics),
		UserCourses:    delivery.NewUserCoursesHandler(userCoursesUseCase),
		Profile:        delivery.NewProfileHandler(authUseCase, uploadsDir),
		Analytics:      delivery.NewAnalyticsHandler(analyticsUseCase),
		Results:        delivery.NewResultsHandler(resultsUseCase),
		TeacherContent: delivery.NewTeacherContentHandler(catalogUseCase),
		Chat:           delivery.NewChatHandler(chatUseCase),
		Review:         delivery.NewReviewHandler(reviewUseCase),
		Rating:         delivery.NewRatingHandler(ratingUseCase),
	}

	router := delivery.NewRouter(handlers, tokens, entitlementRepo, catalogRepo, analytics, uploadsDir)

	port := ":8080"
	logging.L().Info("server listening", slog.String("port", port))
	srv := &http.Server{
		Addr:         port,
		Handler:      router,
		ReadTimeout:  serverReadTimeout,
		WriteTimeout: serverReadTimeout,
		IdleTimeout:  serverIdleTimeout,
	}
	if err := srv.ListenAndServe(); err != nil {
		logging.L().Error("server failed", slog.Any("error", err))
		os.Exit(1)
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
