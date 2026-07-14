package http

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
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
	return NewTeacherContentHandler(usecase.NewCatalogUseCase(catalog, lessons), "")
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

// multipartUploadRequest builds a POST request with a single "file" form part
// (plus any extra string fields) and the teacher-content path values, mirroring
// teacherRequest but for multipart/form-data instead of JSON.
func multipartUploadRequest(t *testing.T, claims domain.Claims, content []byte, filename string, extraFields map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	for k, v := range extraFields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("write field %q: %v", k, err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	r := httptest.NewRequest(http.MethodPost, "http://x/", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r = r.WithContext(context.WithValue(r.Context(), claimsContextKey, claims))
	r.SetPathValue(keyCourseID, testCourseID)
	// A real UUID, unlike the plain testLessonID string elsewhere in this
	// file — the upload handlers validate lesson_id as a UUID before using
	// it in a filesystem path (path traversal guard).
	r.SetPathValue(keyLessonID, testLessonUUID)
	return r
}

const testLessonUUID = "11111111-1111-1111-1111-111111111111"

// mp4Content is a minimal ISO base media file "ftyp" box that
// http.DetectContentType recognizes as "video/mp4" — just enough to exercise
// the MIME sniff, not a playable file.
var mp4Content = []byte{
	0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 0, 0, 0, 'i', 's', 'o', 'm', 'm', 'p', '4', '2',
}

func TestUploadLessonMedia_Success(t *testing.T) {
	catalog := &paymentStubCatRepo{course: domain.Course{ID: testCourseID, TeacherID: testTeacherID}}
	lessons := &stubLessonRepo{lesson: domain.Lesson{ID: testLessonUUID, CourseID: testCourseID}}
	h := NewTeacherContentHandler(usecase.NewCatalogUseCase(catalog, lessons), t.TempDir())

	w := httptest.NewRecorder()
	r := multipartUploadRequest(t, domain.Claims{UserID: testTeacherID, Role: domain.RoleTeacher},
		mp4Content, "lecture.mp4", map[string]string{"duration_seconds": "120"})
	h.UploadLessonMedia(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp mediaDTO
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.MediaType != "video" || resp.DurationSeconds != 120 {
		t.Errorf("resp = %+v, want media_type=video duration_seconds=120", resp)
	}
	if !strings.HasPrefix(resp.URL, "/uploads/lessons/"+testLessonUUID+"/") {
		t.Errorf("URL = %q, want prefix /uploads/lessons/%s/", resp.URL, testLessonUUID)
	}
}

func TestUploadLessonMedia_RejectsUnsupportedType(t *testing.T) {
	catalog := &paymentStubCatRepo{course: domain.Course{ID: testCourseID, TeacherID: testTeacherID}}
	lessons := &stubLessonRepo{lesson: domain.Lesson{ID: testLessonUUID, CourseID: testCourseID}}
	h := NewTeacherContentHandler(usecase.NewCatalogUseCase(catalog, lessons), t.TempDir())

	w := httptest.NewRecorder()
	r := multipartUploadRequest(t, domain.Claims{UserID: testTeacherID, Role: domain.RoleTeacher},
		[]byte("plain text, not a media file"), "notes.txt", nil)
	h.UploadLessonMedia(w, r)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want 415; body=%s", w.Code, w.Body.String())
	}
}

func TestUploadMaterial_Success(t *testing.T) {
	catalog := &paymentStubCatRepo{course: domain.Course{ID: testCourseID, TeacherID: testTeacherID}}
	lessons := &stubLessonRepo{lesson: domain.Lesson{ID: testLessonUUID, CourseID: testCourseID}}
	h := NewTeacherContentHandler(usecase.NewCatalogUseCase(catalog, lessons), t.TempDir())

	pdfContent := []byte("%PDF-1.4 minimal fake pdf content for MIME sniffing")
	w := httptest.NewRecorder()
	r := multipartUploadRequest(t, domain.Claims{UserID: testTeacherID, Role: domain.RoleTeacher},
		pdfContent, "slides.pdf", map[string]string{"title": "Lecture Slides"})
	h.UploadMaterial(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	var resp materialResponseDTO
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Title != "Lecture Slides" || resp.Type != "pdf" {
		t.Errorf("resp = %+v, want title=%q type=pdf", resp, "Lecture Slides")
	}
}

func TestUploadMaterial_DefaultsTitleToFilename(t *testing.T) {
	catalog := &paymentStubCatRepo{course: domain.Course{ID: testCourseID, TeacherID: testTeacherID}}
	lessons := &stubLessonRepo{lesson: domain.Lesson{ID: testLessonUUID, CourseID: testCourseID}}
	h := NewTeacherContentHandler(usecase.NewCatalogUseCase(catalog, lessons), t.TempDir())

	pdfContent := []byte("%PDF-1.4 minimal fake pdf content for MIME sniffing")
	w := httptest.NewRecorder()
	r := multipartUploadRequest(t, domain.Claims{UserID: testTeacherID, Role: domain.RoleTeacher},
		pdfContent, "handout.pdf", nil)
	h.UploadMaterial(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	var resp materialResponseDTO
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Title != "handout.pdf" {
		t.Errorf("Title = %q, want handout.pdf (fallback to filename)", resp.Title)
	}
}
