package e2e

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestMain bootstraps the schema once before the suite runs. If a database is
// reachable but not yet migrated, it applies migrations/*.up.sql so the e2e
// tests work against a blank Postgres (just start the server — no manual
// migrate step). If no database is reachable, it does nothing and every test
// skips via openDB. It never drops or rewrites an already-migrated schema, so
// pointing at an existing dev database won't wipe its structure (the tests
// themselves still truncate row data — always point at a throwaway test DB).
func TestMain(m *testing.M) {
	if db := tryOpen(); db != nil {
		if err := ensureMigrated(db); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: ensure migrated: %v\n", err)
			_ = db.Close()
			os.Exit(1)
		}
		_ = db.Close()
	}
	os.Exit(m.Run())
}

// tryOpen returns a connection using the same env vars openDB uses, or nil when
// e2e is disabled or the database can't be reached (so the suite skips rather
// than fails). It honors the same SLP_E2E opt-in guard as openDB, so TestMain
// never touches a database unless e2e was explicitly enabled.
func tryOpen() *sql.DB {
	if os.Getenv("SLP_E2E") != "1" {
		return nil
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
		return nil
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil
	}
	return db
}

// ensureMigrated applies the migrations only when the schema is absent, detected
// by the presence of the users table. Migration files are plain DDL, so each
// runs as a single multi-statement Exec over pgx's simple protocol.
func ensureMigrated(db *sql.DB) error {
	var reg sql.NullString
	if err := db.QueryRow(`SELECT to_regclass('public.users')`).Scan(&reg); err != nil {
		return fmt.Errorf("probe schema: %w", err)
	}
	if reg.Valid {
		return nil // already migrated
	}

	dir := migrationsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migrations dir %s: %w", dir, err)
	}
	var ups []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			ups = append(ups, e.Name())
		}
	}
	sort.Strings(ups)
	if len(ups) == 0 {
		return fmt.Errorf("no .up.sql files in %s", dir)
	}
	for _, name := range ups {
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if _, err := db.Exec(string(content)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
	}
	return nil
}

// migrationsDir resolves backend/migrations relative to this test file.
func migrationsDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "migrations")
}
