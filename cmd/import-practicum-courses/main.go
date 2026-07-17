// Command import-practicum-courses is a one-shot, manually triggered job
// that copies the practicum team's published course catalog into our own
// (see internal/usecase.PracticumImportUseCase for the full explanation and
// internal/practicum for the HTTP client it reads through). It only needs
// the database connection and PRACTICUM_API_URL/PRACTICUM_JWT_SECRET/
// UPLOADS_DIR — no JWT of our own, and no admin HTTP route, since this is
// meant to be run from the deploy host, not exposed over the network.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/student-learning-portal/backend/internal/database"
	"github.com/student-learning-portal/backend/internal/logging"
	"github.com/student-learning-portal/backend/internal/practicum"
	"github.com/student-learning-portal/backend/internal/usecase"
)

const importTimeout = 15 * time.Minute

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	logging.Init("import-practicum-courses")
	log := logging.L()

	database.InitDB()

	catalogRepo := database.NewPostgresCatalogRepository(database.DB)
	lessonRepo := database.NewPostgresLessonRepository(database.DB)

	client := practicum.NewClient(
		envOrDefault("PRACTICUM_API_URL", "http://10.93.27.25:8000/api/v1"),
		os.Getenv("PRACTICUM_JWT_SECRET"),
	)
	remote := practicum.NewImportRepository(client)

	uploadsDir := envOrDefault("UPLOADS_DIR", "./uploads")
	uc := usecase.NewPracticumImportUseCase(remote, catalogRepo, lessonRepo, uploadsDir)

	ctx, cancel := context.WithTimeout(context.Background(), importTimeout)
	defer cancel()

	start := time.Now()
	summary, err := uc.ImportAll(ctx)
	if err != nil {
		log.Error("import-practicum-courses: failed", slog.Any("error", err))
		os.Exit(1)
	}

	for _, e := range summary.Errors {
		log.Error("import-practicum-courses: item failed", slog.String("error", e))
	}
	log.Info(
		"import-practicum-courses: done",
		slog.Duration("duration", time.Since(start)),
		slog.Int("courses_imported", summary.CoursesImported),
		slog.Int("courses_skipped", summary.CoursesSkipped),
		slog.Int("lessons_imported", summary.LessonsImported),
		slog.Int("lessons_skipped", summary.LessonsSkipped),
		slog.Int("errors", len(summary.Errors)),
	)
	if len(summary.Errors) > 0 {
		os.Exit(1)
	}
}
