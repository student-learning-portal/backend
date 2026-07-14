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
	_ = json.NewEncoder(w).Encode(HelloResponse{Message: "Hello, World!"})
}

// DBHealthHandler handles database health checks
func DBHealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if database.DB == nil || database.DB.Ping() != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "disconnected"})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "connected"})
}

type CatalogHandler struct {
	catalogUseCase *usecase.CatalogUseCase
}

func NewCatalogHandler(uc *usecase.CatalogUseCase) *CatalogHandler {
	return &CatalogHandler{catalogUseCase: uc}
}

// lessonSummaryDTO is one row of a course's lesson list, matching the
// frontend's LessonSummary contract (lesson_id/lesson_type, not the domain
// struct's raw id/type field names).
type lessonSummaryDTO struct {
	LessonID   string `json:"lesson_id"`
	Title      string `json:"title"`
	LessonType string `json:"lesson_type"`
	Position   int    `json:"position"`
}

// GetCourseLessons handles GET /catalog/courses/{course_id}/lessons.
// Public — no auth required. Returns all lessons ordered by position.
func (h *CatalogHandler) GetCourseLessons(w http.ResponseWriter, r *http.Request) {
	courseID := r.PathValue("course_id")
	lessons, err := h.catalogUseCase.GetCourseLessons(r.Context(), courseID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	summaries := make([]lessonSummaryDTO, 0, len(lessons))
	for _, l := range lessons {
		summaries = append(summaries, lessonSummaryDTO{
			LessonID:   l.ID,
			Title:      l.Title,
			LessonType: l.Type,
			Position:   l.Position,
		})
	}
	writeJSON(w, http.StatusOK, summaries)
}

// GetCourses handles GET /catalog/courses
func (h *CatalogHandler) GetCourses(w http.ResponseWriter, r *http.Request) {
	// Extract query parameters
	queryParams := r.URL.Query()
	search := queryParams.Get("search")
	subject := queryParams.Get("subject")
	difficulty := queryParams.Get("difficulty")

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
		Search:     search,
		Subject:    subject,
		Difficulty: difficulty,
		MinPrice:   minPrice,
		MaxPrice:   maxPrice,
		SortBy:     sortBy,
		SortOrder:  sortOrder,
		Page:       page,
		PageSize:   pageSize,
	})

	// Respond with JSON
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(courses)
}
