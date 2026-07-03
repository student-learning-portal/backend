package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

type stubResultsRepo struct {
	courses []domain.CourseResult
	err     error
}

func (s *stubResultsRepo) StudentResults(_ context.Context, _ string) ([]domain.CourseResult, error) {
	return s.courses, s.err
}

func newResultsHandler(repo domain.ResultsRepository) *ResultsHandler {
	return NewResultsHandler(usecase.NewResultsUseCase(repo))
}

func resultsRequestWithClaims(userID string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	return r.WithContext(context.WithValue(r.Context(), claimsContextKey, domain.Claims{UserID: userID}))
}

func TestMyResultsHandler_Success(t *testing.T) {
	h := newResultsHandler(&stubResultsRepo{courses: []domain.CourseResult{
		{CourseID: "a", Title: "A", LessonsTotal: 4, LessonsCompleted: 1},
	}})
	w := httptest.NewRecorder()
	h.MyResults(w, resultsRequestWithClaims("u1"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp domain.StudentResults
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.CoursesEnrolled != 1 {
		t.Errorf("courses_enrolled = %d, want 1", resp.CoursesEnrolled)
	}
	if len(resp.Courses) != 1 || resp.Courses[0].ProgressPercent != 25 {
		t.Errorf("resp = %+v, want one course at 25%%", resp)
	}
}

func TestMyResultsHandler_MissingAuth(t *testing.T) {
	h := newResultsHandler(&stubResultsRepo{})
	w := httptest.NewRecorder()
	h.MyResults(w, httptest.NewRequest(http.MethodGet, "http://x/", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestMyResultsHandler_RepoError(t *testing.T) {
	h := newResultsHandler(&stubResultsRepo{err: errors.New("boom")})
	w := httptest.NewRecorder()
	h.MyResults(w, resultsRequestWithClaims("u1"))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
