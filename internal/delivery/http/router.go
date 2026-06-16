package http

import (
	"net/http"
)

// NewRouter creates a new HTTP multiplexer and registers all project routes.
func NewRouter() *http.ServeMux {
	mux := http.NewServeMux()

	// Registering the Hello World endpoint
	mux.HandleFunc("/hello", HelloHandler)

	// Database ping endpoint
	mux.HandleFunc("/api/v1/health/db", DBHealthHandler)

	// TODO: register course catalog, player, progress and payments endpoints here

	return mux
}
