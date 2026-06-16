package http

import (
	"encoding/json"
	"net/http"
)

// HelloResponse is the format for the hello API.
type HelloResponse struct {
	Message string `json:"message"`
}

// HelloHandler handles GET /hello
func HelloHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(HelloResponse{Message: "Hello, World!"})
}

// DBHealthHandler handles database health checks
func DBHealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Assuming DB is connected based on db.go mock logic
	json.NewEncoder(w).Encode(map[string]string{"status": "connected (simulated)"})
}
