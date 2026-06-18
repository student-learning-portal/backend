package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/student-learning-portal/backend/internal/usecase"
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

type CatalogHandler struct {
	catalogUseCase *usecase.CatalogUseCase
}

func NewCatalogHandler(uc *usecase.CatalogUseCase) *CatalogHandler {
	return &CatalogHandler{catalogUseCase: uc}
}

// GetCourses handles GET /catalog/courses
func (h *CatalogHandler) GetCourses(w http.ResponseWriter, r *http.Request) {
	// Extract query parameters
	queryParams := r.URL.Query()
	search := queryParams.Get("search")
	category := queryParams.Get("category")

	// Parse pagination values with defaults from openapi.yaml
	page := 1
	if pStr := queryParams.Get("page"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil {
			page = p
		}
	}

	pageSize := 10
	if psStr := queryParams.Get("page_size"); psStr != "" {
		if ps, err := strconv.Atoi(psStr); err == nil {
			pageSize = ps
		}
	}

	// Fetch filtered list from usecase layer
	courses := h.catalogUseCase.ListCourses(search, category, page, pageSize)

	// Respond with JSON
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(courses)
}
