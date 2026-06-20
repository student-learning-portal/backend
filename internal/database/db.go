package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// DB is the shared connection pool used by the repositories.
var DB *sql.DB

const (
	pingRetries = 10
	pingDelay   = 2 * time.Second
)

// InitDB opens the Postgres connection pool using credentials from the environment
// (see backend/.env / configs/config.env). It retries the initial ping for a while
// since Postgres may still be initializing when this container starts.
func InitDB() {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		envOrDefault("DB_HOST", "localhost"),
		envOrDefault("DB_PORT", "5432"),
		envOrDefault("POSTGRES_USER", "admin"),
		envOrDefault("POSTGRES_PASSWORD", "qwerty"),
		envOrDefault("POSTGRES_DB", "db"),
	)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("failed to open database connection: %v", err)
	}

	var pingErr error
	for attempt := 1; attempt <= pingRetries; attempt++ {
		if pingErr = db.Ping(); pingErr == nil {
			break
		}
		log.Printf("database not ready yet (attempt %d/%d): %v", attempt, pingRetries, pingErr)
		time.Sleep(pingDelay)
	}
	if pingErr != nil {
		log.Fatalf("failed to ping database after %d attempts: %v", pingRetries, pingErr)
	}

	DB = db
	log.Println("Database connection established")
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
