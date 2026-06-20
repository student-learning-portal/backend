package internal

import (
	"log"
	"net/http"

	"github.com/student-learning-portal/backend/internal/database"
	delivery "github.com/student-learning-portal/backend/internal/delivery/http"
	"github.com/student-learning-portal/backend/internal/usecase"
)

// Run is the main application assembly point.
// It sets up dependencies, database connections, and starts the HTTP server.
func Run() {
	// Initialize Database
	database.InitDB()

	// Initialize Use Cases backed by the database
	catalogRepo := database.NewPostgresCatalogRepository(database.DB)
	catalogUseCase := usecase.NewCatalogUseCase(catalogRepo)

	// Initialize the HTTP handler and inject use cases
	catalogHandler := delivery.NewCatalogHandler(catalogUseCase)

	// Initialize HTTP Router and inject the handler
	router := delivery.NewRouter(catalogHandler)

	port := ":8080"
	log.Printf("Server listening on port %s", port)
	if err := http.ListenAndServe(port, router); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
