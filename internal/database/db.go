package database

import "log"

// DB is a placeholder for the actual database connection/pool
var DB interface{}

// InitDB sets up the database connection pool using connection string from configs/config.env
func InitDB() {
	log.Println("Database connection established (simulated)")
}
