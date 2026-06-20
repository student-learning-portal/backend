package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/student-learning-portal/backend/internal/database"
	"github.com/student-learning-portal/backend/internal/domain"
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

	if database.DB == nil || database.DB.Ping() != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "disconnected"})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "connected"})
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
	subject := queryParams.Get("subject")

	var minPrice, maxPrice *float64
	if v := queryParams.Get("min_price"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			minPrice = &f
		}
	}
	if v := queryParams.Get("max_price"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			maxPrice = &f
		}
	}

	sortBy := queryParams.Get("sort_by")
	sortOrder := queryParams.Get("sort_order")

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
	courses := h.catalogUseCase.ListCourses(domain.CourseListParams{
		Search:    search,
		Subject:   subject,
		MinPrice:  minPrice,
		MaxPrice:  maxPrice,
		SortBy:    sortBy,
		SortOrder: sortOrder,
		Page:      page,
		PageSize:  pageSize,
	})

	// Respond with JSON
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(courses)
}
