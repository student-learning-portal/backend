package e2e

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/security"
)

const (
	testPassword = "Test1234!"
	testMediaURL = "https://cdn.example.com/intro.mp4"
)

// Typed request bodies keep the JSON field names in one place (struct tags)
// rather than repeating map string keys across every test.
type (
	registerBody struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		FullName string `json:"full_name"`
		Role     string `json:"role"`
	}
	loginBody struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	courseIDBody struct {
		CourseID string `json:"course_id"`
	}
	webhookBody struct {
		TransactionID string `json:"transaction_id"`
		Status        string `json:"status"`
		UserID        string `json:"user_id"`
		CourseID      string `json:"course_id"`
	}
)

// testEnv is a ready-to-drive API for the broader endpoint-level tests: a clean
// database, the full router behind an httptest server, and the token service the
// server verifies with (so tests can also mint tokens directly). It builds on
// the package's openDB/buildServer helpers; openDB skips the test when no
// database is reachable.
type testEnv struct {
	t      *testing.T
	db     *sql.DB
	server *httptest.Server
	tokens *security.JWTTokenService
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	db := openDB(t) // skips when the database is unreachable
	t.Cleanup(func() { _ = db.Close() })
	truncateAll(t, db)

	srv := buildServer(t, db)
	t.Cleanup(srv.Close)

	// Same secret buildServer signs with, so directly-minted tokens verify.
	tokens := security.NewJWTTokenService(getenv("JWT_SECRET", testSecret), 24*time.Hour)
	return &testEnv{t: t, db: db, server: srv, tokens: tokens}
}

// --- HTTP client ---------------------------------------------------------

type apiResp struct {
	status int
	body   []byte
}

// do issues a request to the test server. A non-nil body is JSON-encoded; a
// non-empty token is sent as a bearer token. Unlike the package's call helper,
// it returns the status and raw body so tests can assert error responses too.
func (e *testEnv) do(method, path, token string, body any) apiResp {
	e.t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatalf("marshal body: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.server.URL+path, r)
	if err != nil {
		e.t.Fatalf("new request %s %s: %v", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := e.server.Client().Do(req)
	if err != nil {
		e.t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatalf("read body %s %s: %v", method, path, err)
	}
	return apiResp{status: resp.StatusCode, body: data}
}

func (e *testEnv) decode(r apiResp, v any) {
	e.t.Helper()
	if err := json.Unmarshal(r.body, v); err != nil {
		e.t.Fatalf("decode response %q: %v", string(r.body), err)
	}
}

func (e *testEnv) requireStatus(r apiResp, want int) {
	e.t.Helper()
	if r.status != want {
		e.t.Fatalf("status = %d, want %d; body=%s", r.status, want, string(r.body))
	}
}

func (e *testEnv) errorMessage(r apiResp) string {
	e.t.Helper()
	var out struct {
		Error string `json:"error"`
	}
	e.decode(r, &out)
	return out.Error
}

// --- identities ----------------------------------------------------------

// token mints a bearer token directly (no login) for the given user id + role.
func (e *testEnv) token(userID string, role domain.Role) string {
	e.t.Helper()
	tok, err := e.tokens.Generate(domain.User{ID: userID, Email: userID + "@test.local", Role: role})
	if err != nil {
		e.t.Fatalf("generate token: %v", err)
	}
	return tok
}

// register creates a real account through the API (bcrypt-hashed) and returns
// its id and a freshly issued bearer token.
func (e *testEnv) register(email, fullName string, role domain.Role) (userID, token string) {
	e.t.Helper()
	resp := e.do(http.MethodPost, "/api/v1/auth/register", "", registerBody{
		Email:    email,
		Password: testPassword,
		FullName: fullName,
		Role:     string(role),
	})
	e.requireStatus(resp, http.StatusCreated)
	var out struct {
		Token string `json:"token"`
		User  struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	e.decode(resp, &out)
	if out.User.ID == "" || out.Token == "" {
		e.t.Fatalf("register returned empty id/token: %s", string(resp.body))
	}
	return out.User.ID, out.Token
}

// --- direct DB seeding ---------------------------------------------------

func (e *testEnv) insertCourse(teacherID, title, subject string, price float64, status string) string {
	e.t.Helper()
	var id string
	err := e.db.QueryRow(
		`INSERT INTO courses (teacher_id, title, description, subject, price, currency, status)
		 VALUES ($1, $2, $3, $4, $5, 'USD', $6) RETURNING id`,
		teacherID, title, title+" description", subject, price, status,
	).Scan(&id)
	if err != nil {
		e.t.Fatalf("insert course: %v", err)
	}
	return id
}

func (e *testEnv) insertLesson(courseID, title, lessonType string, position int) string {
	e.t.Helper()
	var id string
	err := e.db.QueryRow(
		`INSERT INTO lessons (course_id, title, lesson_type, position)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		courseID, title, lessonType, position,
	).Scan(&id)
	if err != nil {
		e.t.Fatalf("insert lesson: %v", err)
	}
	return id
}

func (e *testEnv) insertMedia(lessonID string, durationMs int) {
	e.t.Helper()
	if _, err := e.db.Exec(
		`INSERT INTO media (lesson_id, url, duration_ms, media_type) VALUES ($1, $2, $3, 'video')`,
		lessonID, testMediaURL, durationMs,
	); err != nil {
		e.t.Fatalf("insert media: %v", err)
	}
}

func (e *testEnv) insertMaterial(lessonID, title, url, materialType string) {
	e.t.Helper()
	if _, err := e.db.Exec(
		`INSERT INTO materials (lesson_id, title, url, material_type) VALUES ($1, $2, $3, $4)`,
		lessonID, title, url, materialType,
	); err != nil {
		e.t.Fatalf("insert material: %v", err)
	}
}

// insertRollup writes a derived analytics_student_course row, as the rollup
// loader would, so the teacher dashboard has data to classify.
func (e *testEnv) insertRollup(courseID, actorID string, progressPercent float64, completed, total int, lastActivity *time.Time) {
	e.t.Helper()
	if _, err := e.db.Exec(
		`INSERT INTO analytics_student_course
		   (actor_id, course_id, lessons_total, lessons_completed, progress_percent, last_activity_ts)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		actorID, courseID, total, completed, progressPercent, lastActivity,
	); err != nil {
		e.t.Fatalf("insert rollup: %v", err)
	}
}

func (e *testEnv) countRows(query string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.db.QueryRow(query, args...).Scan(&n); err != nil {
		e.t.Fatalf("count query %q: %v", query, err)
	}
	return n
}

func truncateAll(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`TRUNCATE TABLE
		event_log, access_check_log, access_grant, payment, progress_state,
		analytics_student_course, messages, materials, media, lessons, courses, users
		RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}
