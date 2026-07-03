package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

const testTeacherID = "teacher-1"

// teacherRequest builds a request carrying auth claims and the teacher-content
// path values, mimicking what RequireAuth + ServeMux supply at runtime.
func teacherRequest(method, body string, claims domain.Claims) *http.Request {
	r := httptest.NewRequest(method, "http://x/", strings.NewReader(body))
	r = r.WithContext(context.WithValue(r.Context(), claimsContextKey, claims))
	r.SetPathValue(keyCourseID, testCourseID)
	r.SetPathValue(keyLessonID, testLessonID)
	r.SetPathValue("material_id", "material-1")
	return r
}

func newTeacherContentHandler(catalog domain.CatalogRepository, lessons domain.LessonRepository) *TeacherContentHandler {
	return NewTeacherContentHandler(usecase.NewCatalogUseCase(catalog, lessons))
}

func TestCreateCourse_RejectsNonTeacher(t *testing.T) {
	h := newTeacherContentHandler(&paymentStubCatRepo{}, &stubLessonRepo{})

	w := httptest.NewRecorder()
	r := teacherRequest(http.MethodPost, `{"title":"Go basics","price":10}`, domain.Claims{UserID: "u1", Role: domain.RoleStudent})
	h.CreateCourse(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403; body=%s", w.Code, w.Body.String())
	}
}

func TestCreateCourse_Success(t *testing.T) {
	h := newTeacherContentHandler(&paymentStubCatRepo{}, &stubLessonRepo{})

	w := httptest.NewRecorder()
	r := teacherRequest(http.MethodPost, `{"title":"Go basics","price":10}`, domain.Claims{UserID: testTeacherID, Role: domain.RoleTeacher})
	h.CreateCourse(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	var resp domain.Course
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Title != "Go basics" || resp.TeacherID != testTeacherID {
		t.Errorf("resp = %+v, want title=Go basics teacher_id=%s", resp, testTeacherID)
	}
}

func TestCreateCourse_ValidationError(t *testing.T) {
	h := newTeacherContentHandler(&paymentStubCatRepo{}, &stubLessonRepo{})

	w := httptest.NewRecorder()
	r := teacherRequest(http.MethodPost, `{"title":""}`, domain.Claims{UserID: testTeacherID, Role: domain.RoleTeacher})
	h.CreateCourse(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestUpdateCourse_RejectsNonOwner(t *testing.T) {
	catalog := &paymentStubCatRepo{course: domain.Course{ID: testCourseID, TeacherID: testTeacherID}}
	h := newTeacherContentHandler(catalog, &stubLessonRepo{})

	w := httptest.NewRecorder()
	r := teacherRequest(http.MethodPatch, `{"title":"New title","status":"draft"}`, domain.Claims{UserID: "someone-else", Role: domain.RoleTeacher})
	h.UpdateCourse(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403; body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteCourse_RejectsWhenNotDraft(t *testing.T) {
	catalog := &paymentStubCatRepo{course: domain.Course{ID: testCourseID, TeacherID: testTeacherID, Status: "published"}}
	h := newTeacherContentHandler(catalog, &stubLessonRepo{})

	w := httptest.NewRecorder()
	r := teacherRequest(http.MethodDelete, "", domain.Claims{UserID: testTeacherID, Role: domain.RoleTeacher})
	h.DeleteCourse(w, r)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409; body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteCourse_DraftSucceeds(t *testing.T) {
	catalog := &paymentStubCatRepo{course: domain.Course{ID: testCourseID, TeacherID: testTeacherID, Status: "draft"}}
	h := newTeacherContentHandler(catalog, &stubLessonRepo{})

	w := httptest.NewRecorder()
	r := teacherRequest(http.MethodDelete, "", domain.Claims{UserID: testTeacherID, Role: domain.RoleTeacher})
	h.DeleteCourse(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204; body=%s", w.Code, w.Body.String())
	}
}

func TestCreateLesson_Success(t *testing.T) {
	catalog := &paymentStubCatRepo{course: domain.Course{ID: testCourseID, TeacherID: testTeacherID}}
	h := newTeacherContentHandler(catalog, &stubLessonRepo{})

	w := httptest.NewRecorder()
	r := teacherRequest(http.MethodPost, `{"title":"Lesson 1","lesson_type":"video"}`, domain.Claims{UserID: testTeacherID, Role: domain.RoleTeacher})
	h.CreateLesson(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	var resp teacherLessonDTO
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Title != "Lesson 1" || resp.LessonType != "video" {
		t.Errorf("resp = %+v, want title=Lesson 1 lesson_type=video", resp)
	}
}

func TestCreateLesson_RejectsUnknownType(t *testing.T) {
	catalog := &paymentStubCatRepo{course: domain.Course{ID: testCourseID, TeacherID: testTeacherID}}
	h := newTeacherContentHandler(catalog, &stubLessonRepo{})

	w := httptest.NewRecorder()
	r := teacherRequest(http.MethodPost, `{"title":"Lesson 1","lesson_type":"podcast"}`, domain.Claims{UserID: testTeacherID, Role: domain.RoleTeacher})
	h.CreateLesson(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestReorderLessons_RejectsEmptyList(t *testing.T) {
	catalog := &paymentStubCatRepo{course: domain.Course{ID: testCourseID, TeacherID: testTeacherID}}
	h := newTeacherContentHandler(catalog, &stubLessonRepo{})

	w := httptest.NewRecorder()
	r := teacherRequest(http.MethodPut, `{"lesson_ids":[]}`, domain.Claims{UserID: testTeacherID, Role: domain.RoleTeacher})
	h.ReorderLessons(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestSetLessonMedia_Success(t *testing.T) {
	catalog := &paymentStubCatRepo{course: domain.Course{ID: testCourseID, TeacherID: testTeacherID}}
	lessons := &stubLessonRepo{lesson: domain.Lesson{ID: testLessonID, CourseID: testCourseID}}
	h := newTeacherContentHandler(catalog, lessons)

	w := httptest.NewRecorder()
	r := teacherRequest(http.MethodPut, `{"url":"https://cdn/x.mp4","duration_seconds":90,"media_type":"video"}`, domain.Claims{UserID: testTeacherID, Role: domain.RoleTeacher})
	h.SetLessonMedia(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp mediaDTO
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.DurationSeconds != 90 || resp.MediaType != "video" {
		t.Errorf("resp = %+v, want duration_seconds=90 media_type=video", resp)
	}
}

func TestAddMaterial_RejectsMissingFields(t *testing.T) {
	catalog := &paymentStubCatRepo{course: domain.Course{ID: testCourseID, TeacherID: testTeacherID}}
	lessons := &stubLessonRepo{lesson: domain.Lesson{ID: testLessonID, CourseID: testCourseID}}
	h := newTeacherContentHandler(catalog, lessons)

	w := httptest.NewRecorder()
	r := teacherRequest(http.MethodPost, `{"title":"","url":""}`, domain.Claims{UserID: testTeacherID, Role: domain.RoleTeacher})
	h.AddMaterial(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteMaterial_Success(t *testing.T) {
	catalog := &paymentStubCatRepo{course: domain.Course{ID: testCourseID, TeacherID: testTeacherID}}
	lessons := &stubLessonRepo{lesson: domain.Lesson{ID: testLessonID, CourseID: testCourseID}}
	h := newTeacherContentHandler(catalog, lessons)

	w := httptest.NewRecorder()
	r := teacherRequest(http.MethodDelete, "", domain.Claims{UserID: testTeacherID, Role: domain.RoleTeacher})
	h.DeleteMaterial(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204; body=%s", w.Code, w.Body.String())
	}
}
