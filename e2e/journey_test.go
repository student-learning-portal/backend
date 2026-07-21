// Package e2e holds end-to-end tests that run against a live Postgres instance.
// The test is skipped automatically when the database is not reachable.
//
// Run with:
//
//	go test -tags=e2e -v ./e2e/
package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/student-learning-portal/backend/internal/database"
	delivery "github.com/student-learning-portal/backend/internal/delivery/http"
	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/security"
	"github.com/student-learning-portal/backend/internal/usecase"
)

// Seed UUIDs from scripts/seed.sql.
const (
	seedCourseID = "33333333-3333-4333-8333-000000000001" // Introduction to Go, $49.99
	seedLessonID = "44444444-4444-4444-8444-000000000001" // Go Syntax Basics
	testSecret   = "e2e-test-jwt-secret"
)

// TestCoreJourney walks the primary learner path against a real database and a
// full HTTP stack (no mocks):
//
//  1. Register a new student          -> gets $1 000.00 wallet balance
//  2. Sandbox checkout                -> wallet debited, access grant created
//  3. GET lesson                      -> entitlement middleware allows, content returned
//  4. POST progress at 60 s           -> progress_state upserted
//  5. GET progress                    -> same 60 s returned (persistence confirmed)
func TestCoreJourney(t *testing.T) {
	db := openDB(t)
	defer db.Close()
	applySeed(t, db)

	srv := buildServer(t, db)
	defer srv.Close()
	client := srv.Client()

	// 1. Register a fresh student; new accounts start with 1 000.00
	token, initialBalance := registerStudent(t, client, srv.URL)

	// 2. Buy the course in sandbox - wallet must be debited
	purchase := checkout(t, client, srv.URL, token, seedCourseID)
	if purchase.Status != "succeeded" {
		t.Fatalf("step 2 checkout: status = %q, want succeeded", purchase.Status)
	}
	if purchase.Balance >= initialBalance {
		t.Fatalf("step 2 checkout: balance %.2f should be less than initial %.2f", purchase.Balance, initialBalance)
	}

	// 3. Access the lesson - RequireEntitlement must allow after purchase
	lesson := getLesson(t, client, srv.URL, token, seedCourseID, seedLessonID)
	if lesson.LessonID != seedLessonID {
		t.Fatalf("step 3 get lesson: lesson_id = %q, want %q", lesson.LessonID, seedLessonID)
	}
	if lesson.ContentURL == "" {
		t.Fatal("step 3 get lesson: content_url is empty")
	}

	// 4. Save progress at the 60 second mark
	saved := saveProgress(t, client, srv.URL, token, seedCourseID, seedLessonID, 60)
	if saved.ProgressSeconds != 60 {
		t.Fatalf("step 4 save progress: progress_seconds = %d, want 60", saved.ProgressSeconds)
	}

	// 5. Re-fetch - confirm the row persisted in Postgres
	got := getProgress(t, client, srv.URL, token, seedCourseID, seedLessonID)
	if got.ProgressSeconds != 60 {
		t.Fatalf("step 5 get progress: progress_seconds = %d, want 60 (not persisted)", got.ProgressSeconds)
	}
	if got.UpdatedAt == "" {
		t.Fatal("step 5 get progress: updated_at is empty")
	}
}

// openDB opens a Postgres connection with the same env vars the app reads.
// Skips the test unless e2e is explicitly enabled (SLP_E2E=1) and the database
// is reachable. The opt-in guard matters because these tests TRUNCATE every
// table: without it, a bare `go test ./...` would wipe whatever Postgres the
// default DB_* values point at (e.g. a local dev database on :5433). The
// scripts/integration-test.sh runner sets SLP_E2E=1 and points DB_* at a
// throwaway container.
func openDB(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("SLP_E2E") != "1" {
		t.Skip("e2e disabled: set SLP_E2E=1 and point DB_* at a throwaway database (see scripts/integration-test.sh)")
	}
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getenv("DB_HOST", "localhost"),
		getenv("DB_PORT", "5433"),
		getenv("POSTGRES_USER", "admin"),
		getenv("POSTGRES_PASSWORD", "qwerty"),
		getenv("POSTGRES_DB", "db"),
	)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skipf("e2e skipped: cannot open database: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		t.Skipf("e2e skipped: database not reachable (%v); start Postgres and re-run", err)
	}
	return db
}

// applySeed resets all domain tables and loads the standard fixture set from
// scripts/seed.sql, giving the test a known, reproducible starting state
func applySeed(t *testing.T, db *sql.DB) {
	t.Helper()
	data, err := os.ReadFile("../scripts/seed.sql")
	if err != nil {
		t.Fatalf("read seed.sql: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), string(data)); err != nil {
		t.Fatalf("apply seed: %v", err)
	}
}

// buildServer wires the full dependency graph (mirroring internal/app.go) and
// returns a test HTTP server backed by real Postgres repos. All handlers are
// wired (catalog, auth, purchase, player, user-courses, profile, analytics) so
// any route can be exercised end to end.
func buildServer(t *testing.T, db *sql.DB) *httptest.Server {
	t.Helper()

	secret := getenv("JWT_SECRET", testSecret)
	tokens := security.NewJWTTokenService(secret, 24*time.Hour)

	catalogRepo := database.NewPostgresCatalogRepository(db)
	userRepo := database.NewPostgresUserRepository(db)
	entitlementRepo := database.NewPostgresEntitlementRepository(db)
	lessonRepo := database.NewPostgresLessonRepository(db)
	progressRepo := database.NewPostgresProgressRepository(db)
	analyticsRepo := database.NewPostgresAnalyticsRepository(db)

	// Postgres event_log + point rollup refresh are wired (so player/purchase
	// e2e tests exercise the real analytics pipeline); the NDJSON sink is left
	// out to keep test stdout quiet. Env must be a real enum value: event_log.env
	// has CHECK (env IN ('dev','staging','prod')), so the Source{} zero value
	// would silently fail every insert (swallowed by the recorder's best-effort
	// error handling) and leave event_log empty.
	analytics := usecase.NewAnalyticsRecorder(domain.Source{Env: "dev"},
		database.NewPostgresEventSink(db),
		usecase.NewRollupRefreshSink(analyticsRepo),
	)

	uploadsDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(uploadsDir, "avatars"), 0o755); err != nil {
		t.Fatalf("create uploads dir: %v", err)
	}

	authUC := usecase.NewAuthUseCase(userRepo, tokens)
	catalogUC := usecase.NewCatalogUseCase(catalogRepo, lessonRepo)
	adminUC := usecase.NewAdminUseCase(userRepo)
	handlers := delivery.Handlers{
		Catalog:        delivery.NewCatalogHandler(catalogUC),
		Auth:           delivery.NewAuthHandler(authUC, analytics),
		Purchase:       delivery.NewPurchaseHandler(usecase.NewPaymentUseCase(entitlementRepo, catalogRepo, userRepo), analytics),
		Player:         delivery.NewPlayerHandler(usecase.NewPlayerUseCase(lessonRepo, progressRepo), analytics),
		UserCourses:    delivery.NewUserCoursesHandler(usecase.NewUserCoursesUseCase(catalogRepo, entitlementRepo)),
		Profile:        delivery.NewProfileHandler(authUC, uploadsDir),
		Analytics:      delivery.NewAnalyticsHandler(usecase.NewAnalyticsUseCase(analyticsRepo, catalogRepo, domain.DefaultRiskThresholds)),
		Results:        delivery.NewResultsHandler(usecase.NewResultsUseCase(database.NewPostgresResultsRepository(db), domain.DefaultRiskThresholds)),
		TeacherContent: delivery.NewTeacherContentHandler(catalogUC, uploadsDir),
		Chat:           delivery.NewChatHandler(usecase.NewChatUseCase(database.NewPostgresChatRepository(db), catalogRepo, entitlementRepo)),
		Admin:          delivery.NewAdminHandler(adminUC, analytics),
	}
	return httptest.NewServer(delivery.NewRouter(handlers, delivery.Deps{
		Tokens:       tokens,
		Entitlements: entitlementRepo,
		Catalog:      catalogRepo,
		Users:        userRepo,
		Analytics:    analytics,
		UploadsDir:   uploadsDir,
	}))
}

// - response DTOs (mirror the handler structs) ----------------------------

type authResp struct {
	Token string `json:"token"`
	User  struct {
		Balance float64 `json:"balance"`
	} `json:"user"`
}

type checkoutResp struct {
	Status  string  `json:"status"`
	Balance float64 `json:"balance"`
}

type lessonResp struct {
	LessonID   string `json:"lesson_id"`
	ContentURL string `json:"content_url"`
}

type progressResp struct {
	ProgressSeconds int    `json:"progress_seconds"`
	UpdatedAt       string `json:"updated_at"`
}

// - step helpers ---------------------------------------------------------

func registerStudent(t *testing.T, client *http.Client, base string) (token string, balance float64) {
	t.Helper()
	email := fmt.Sprintf("e2e-%d@test.local", time.Now().UnixNano())
	var resp authResp
	call(t, client, http.MethodPost, base+"/api/v1/auth/register", "", map[string]any{
		"email": email, "password": "Test1234!", "full_name": "E2E Tester", "role": "student",
	}, http.StatusCreated, &resp)
	return resp.Token, resp.User.Balance
}

func checkout(t *testing.T, client *http.Client, base, token, courseID string) checkoutResp {
	t.Helper()
	var resp checkoutResp
	call(t, client, http.MethodPost, base+"/api/v1/purchase/checkout", token,
		map[string]any{"course_id": courseID}, http.StatusOK, &resp)
	return resp
}

func getLesson(t *testing.T, client *http.Client, base, token, courseID, lessonID string) lessonResp {
	t.Helper()
	var resp lessonResp
	call(t, client, http.MethodGet,
		fmt.Sprintf("%s/api/v1/player/courses/%s/lessons/%s", base, courseID, lessonID),
		token, nil, http.StatusOK, &resp)
	return resp
}

func saveProgress(t *testing.T, client *http.Client, base, token, courseID, lessonID string, secs int) progressResp {
	t.Helper()
	var resp progressResp
	call(t, client, http.MethodPost,
		fmt.Sprintf("%s/api/v1/player/courses/%s/lessons/%s/progress", base, courseID, lessonID),
		token, map[string]any{"progress_seconds": secs, "completed": false}, http.StatusOK, &resp)
	return resp
}

func getProgress(t *testing.T, client *http.Client, base, token, courseID, lessonID string) progressResp {
	t.Helper()
	var resp progressResp
	call(t, client, http.MethodGet,
		fmt.Sprintf("%s/api/v1/player/courses/%s/lessons/%s/progress", base, courseID, lessonID),
		token, nil, http.StatusOK, &resp)
	return resp
}

// call makes one HTTP request, asserts the status code, and JSON-decodes the
// response body into dest. A non-matching status is a fatal test error
func call(t *testing.T, client *http.Client, method, url, token string, body any, wantStatus int, dest any) {
	t.Helper()
	var reqBody io.Reader = http.NoBody
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, url, reqBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer res.Body.Close()
	if res.StatusCode != wantStatus {
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("%s %s → %d (want %d): %s", method, url, res.StatusCode, wantStatus, raw)
	}
	if dest != nil {
		if err := json.NewDecoder(res.Body).Decode(dest); err != nil {
			t.Fatalf("decode %s %s response: %v", method, url, err)
		}
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
