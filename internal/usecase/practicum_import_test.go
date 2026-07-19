package usecase

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
)

// fakeImportRepo implements domain.CourseImportRepository for import tests.
type fakeImportRepo struct {
	courses        []domain.RemoteCourse
	lessons        map[string][]domain.RemoteLesson // keyed by remote course id
	files          map[string][]domain.RemoteFile   // keyed by remote lesson id
	fileBytes      map[string][]byte                // keyed by remote file id
	primaryMediaID map[string]string                // keyed by remote lesson id
}

func (f *fakeImportRepo) ListRemoteCourses(context.Context) ([]domain.RemoteCourse, error) {
	return f.courses, nil
}

func (f *fakeImportRepo) ListRemoteLessons(_ context.Context, remoteCourseID string) ([]domain.RemoteLesson, error) {
	return f.lessons[remoteCourseID], nil
}

func (f *fakeImportRepo) ListRemoteLessonFiles(_ context.Context, remoteLessonID string) ([]domain.RemoteFile, error) {
	return f.files[remoteLessonID], nil
}

func (f *fakeImportRepo) DownloadRemoteFile(_ context.Context, remoteFileID string) ([]byte, error) {
	if data, ok := f.fileBytes[remoteFileID]; ok {
		return data, nil
	}
	return []byte("data"), nil
}

func (f *fakeImportRepo) GetRemotePrimaryMediaFileID(_ context.Context, remoteLessonID string) (string, bool, error) {
	id, ok := f.primaryMediaID[remoteLessonID]
	return id, ok, nil
}

// importCatalogRepo is a minimal in-memory domain.CatalogRepository for
// import tests — only the methods PracticumImportUseCase actually calls are
// meaningfully implemented.
type importCatalogRepo struct {
	byExternalID map[string]domain.Course
	created      []domain.Course
	nextID       int
}

func newImportCatalogRepo() *importCatalogRepo {
	return &importCatalogRepo{byExternalID: map[string]domain.Course{}}
}

func (r *importCatalogRepo) GetCourses(domain.CourseListParams) ([]domain.Course, int, error) {
	return nil, 0, nil
}

func (r *importCatalogRepo) GetByID(context.Context, string) (domain.Course, error) {
	return domain.Course{}, nil
}

func (r *importCatalogRepo) GetByTeacherID(context.Context, string) ([]domain.Course, error) {
	return nil, nil
}

func (r *importCatalogRepo) Create(_ context.Context, c domain.Course) (domain.Course, error) {
	r.nextID++
	c.ID = "local-" + string(rune('0'+r.nextID))
	r.created = append(r.created, c)
	return c, nil
}

func (r *importCatalogRepo) Update(_ context.Context, c domain.Course) (domain.Course, error) {
	return c, nil
}
func (r *importCatalogRepo) Delete(context.Context, string) error { return nil }

func (r *importCatalogRepo) GetExternalCourseID(context.Context, string) (string, bool, error) {
	return "", false, nil
}

func (r *importCatalogRepo) SetExternalCourseID(_ context.Context, courseID, externalID string) error {
	c := domain.Course{ID: courseID}
	for _, created := range r.created {
		if created.ID == courseID {
			c = created
			break
		}
	}
	r.byExternalID[externalID] = c
	return nil
}

func (r *importCatalogRepo) FindByExternalCourseID(_ context.Context, externalID string) (domain.Course, bool, error) {
	c, ok := r.byExternalID[externalID]
	return c, ok, nil
}

// importLessonRepo is a minimal in-memory domain.LessonRepository for import tests.
type importLessonRepo struct {
	byExternalID map[string]domain.Lesson
	created      []domain.Lesson
	media        map[string]domain.Media
	materials    map[string][]domain.Material
	nextID       int

	createLessonErr error
}

func newImportLessonRepo() *importLessonRepo {
	return &importLessonRepo{
		byExternalID: map[string]domain.Lesson{},
		media:        map[string]domain.Media{},
		materials:    map[string][]domain.Material{},
	}
}

func (r *importLessonRepo) GetLessonsByCourseID(context.Context, string) ([]domain.Lesson, error) {
	return nil, nil
}

func (r *importLessonRepo) GetLesson(context.Context, string, string) (domain.Lesson, error) {
	return domain.Lesson{}, nil
}

func (r *importLessonRepo) GetLessonMedia(context.Context, string) ([]domain.Media, error) {
	return nil, nil
}

func (r *importLessonRepo) GetLessonMaterials(context.Context, string) ([]domain.Material, error) {
	return nil, nil
}

func (r *importLessonRepo) CreateLesson(_ context.Context, courseID, title, lessonType string) (domain.Lesson, error) {
	if r.createLessonErr != nil {
		return domain.Lesson{}, r.createLessonErr
	}
	r.nextID++
	l := domain.Lesson{ID: "lesson-" + string(rune('0'+r.nextID)), CourseID: courseID, Title: title, Type: lessonType}
	r.created = append(r.created, l)
	return l, nil
}

func (r *importLessonRepo) UpdateLesson(context.Context, string, string, string, string) (domain.Lesson, error) {
	return domain.Lesson{}, nil
}
func (r *importLessonRepo) ReorderLessons(context.Context, string, []string) error { return nil }
func (r *importLessonRepo) DeleteLesson(context.Context, string, string) error     { return nil }

func (r *importLessonRepo) SetLessonMedia(_ context.Context, lessonID, url string, durationMs int, mediaType string) (domain.Media, error) {
	m := domain.Media{LessonID: lessonID, URL: url, DurationMs: durationMs, Type: mediaType}
	r.media[lessonID] = m
	return m, nil
}
func (r *importLessonRepo) DeleteLessonMedia(context.Context, string) error { return nil }

func (r *importLessonRepo) AddMaterial(_ context.Context, lessonID, title, url, materialType string) (domain.Material, error) {
	m := domain.Material{LessonID: lessonID, Title: title, URL: url, Type: materialType}
	r.materials[lessonID] = append(r.materials[lessonID], m)
	return m, nil
}
func (r *importLessonRepo) DeleteMaterial(context.Context, string, string) error { return nil }

func (r *importLessonRepo) FindByExternalID(_ context.Context, externalLessonID string) (domain.Lesson, bool, error) {
	l, ok := r.byExternalID[externalLessonID]
	return l, ok, nil
}

func (r *importLessonRepo) SetExternalLessonID(_ context.Context, lessonID, externalID string) error {
	l := domain.Lesson{ID: lessonID}
	for _, created := range r.created {
		if created.ID == lessonID {
			l = created
			break
		}
	}
	r.byExternalID[externalID] = l
	return nil
}

func TestPracticumImportUseCase_ImportAll_CreatesCourseAndLessonsWithMedia(t *testing.T) {
	remote := &fakeImportRepo{
		courses: []domain.RemoteCourse{
			{ID: "rc1", Title: "Go Basics", Subject: "programming", Price: 500, Difficulty: domain.DifficultyBeginner, DurationMinutes: 60},
		},
		lessons: map[string][]domain.RemoteLesson{
			"rc1": {{ID: "rl1", Title: "Intro"}, {ID: "rl2", Title: "Quiz only"}},
		},
		files: map[string][]domain.RemoteFile{
			"rl1": {
				{ID: "f1", OriginalFilename: "video.mp4", MimeType: "video/mp4"},
				{ID: "f2", OriginalFilename: "slides.pdf", MimeType: "application/pdf"},
			},
			// rl2 has no files at all — simulates a quiz-only lesson.
		},
	}
	catalog := newImportCatalogRepo()
	lessons := newImportLessonRepo()

	uc := NewPracticumImportUseCase(remote, catalog, lessons, t.TempDir())
	summary, err := uc.ImportAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(summary.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", summary.Errors)
	}
	if summary.CoursesImported != 1 || summary.CoursesSkipped != 0 {
		t.Errorf("courses: imported=%d skipped=%d", summary.CoursesImported, summary.CoursesSkipped)
	}
	if summary.LessonsImported != 2 || summary.LessonsSkipped != 0 {
		t.Errorf("lessons: imported=%d skipped=%d", summary.LessonsImported, summary.LessonsSkipped)
	}

	if len(catalog.created) != 1 {
		t.Fatalf("expected 1 course created, got %d", len(catalog.created))
	}
	created := catalog.created[0]
	if created.Title != "Go Basics" || created.TeacherID != importOwnerTeacherID {
		t.Errorf("course = %+v", created)
	}
	if _, ok := catalog.byExternalID["rc1"]; !ok {
		t.Error("expected course linked by external id rc1")
	}

	if len(lessons.created) != 2 {
		t.Fatalf("expected 2 lessons created, got %d", len(lessons.created))
	}
	introLesson := lessons.created[0]
	if introLesson.Type != videoType {
		t.Errorf("intro lesson type = %q, want video", introLesson.Type)
	}
	quizLesson := lessons.created[1]
	if quizLesson.Type != "text" {
		t.Errorf("quiz-only lesson type = %q, want text", quizLesson.Type)
	}

	media, ok := lessons.media[introLesson.ID]
	if !ok || media.Type != videoType || media.URL == "" {
		t.Errorf("expected video media on intro lesson, got %+v (ok=%v)", media, ok)
	}
	materials := lessons.materials[introLesson.ID]
	if len(materials) != 1 || materials[0].Type != materialTypePDF {
		t.Errorf("expected 1 pdf material on intro lesson, got %+v", materials)
	}
	if len(lessons.materials[quizLesson.ID]) != 0 {
		t.Errorf("expected no materials on quiz-only lesson, got %+v", lessons.materials[quizLesson.ID])
	}
}

func TestPracticumImportUseCase_ImportAll_PrefersContentMediaFileIDOverFirstFile(t *testing.T) {
	remote := &fakeImportRepo{
		courses: []domain.RemoteCourse{{ID: "rc1", Title: "Go Basics"}},
		lessons: map[string][]domain.RemoteLesson{
			"rc1": {{ID: "rl1", Title: "Intro"}},
		},
		files: map[string][]domain.RemoteFile{
			// f1 (video) comes first in the list, but the lesson's content
			// names f2 as the primary — f2's bytes must be the ones saved.
			"rl1": {
				{ID: "f1", OriginalFilename: "old-take.mp4", MimeType: "video/mp4"},
				{ID: "f2", OriginalFilename: "final-take.mp4", MimeType: "video/mp4"},
			},
		},
		fileBytes: map[string][]byte{
			"f1": []byte("old take bytes"),
			"f2": []byte("final take bytes"),
		},
		primaryMediaID: map[string]string{"rl1": "f2"},
	}
	catalog := newImportCatalogRepo()
	lessons := newImportLessonRepo()
	uploadsDir := t.TempDir()

	uc := NewPracticumImportUseCase(remote, catalog, lessons, uploadsDir)
	summary, err := uc.ImportAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(summary.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", summary.Errors)
	}

	introLesson := lessons.created[0]
	media, ok := lessons.media[introLesson.ID]
	if !ok {
		t.Fatalf("expected media on intro lesson")
	}

	savedPath := filepath.Join(uploadsDir, strings.TrimPrefix(media.URL, "/uploads/"))
	got, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatalf("read saved media file: %v", err)
	}
	if string(got) != "final take bytes" {
		t.Errorf("saved media content = %q, want the content-preferred file (f2)'s bytes", got)
	}
}

func TestPracticumImportUseCase_ImportAll_SkipsAlreadyImportedCourse(t *testing.T) {
	remote := &fakeImportRepo{
		courses: []domain.RemoteCourse{{ID: "rc1", Title: "Go Basics"}},
		lessons: map[string][]domain.RemoteLesson{"rc1": {{ID: "rl1", Title: "Intro"}}},
	}
	catalog := newImportCatalogRepo()
	catalog.byExternalID["rc1"] = domain.Course{ID: "existing-course"}
	lessons := newImportLessonRepo()
	lessons.byExternalID["rl1"] = domain.Lesson{ID: "existing-lesson"}

	uc := NewPracticumImportUseCase(remote, catalog, lessons, t.TempDir())
	summary, err := uc.ImportAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.CoursesImported != 0 || summary.CoursesSkipped != 1 {
		t.Errorf("courses: imported=%d skipped=%d", summary.CoursesImported, summary.CoursesSkipped)
	}
	if summary.LessonsImported != 0 || summary.LessonsSkipped != 1 {
		t.Errorf("lessons: imported=%d skipped=%d", summary.LessonsImported, summary.LessonsSkipped)
	}
	if len(catalog.created) != 0 {
		t.Errorf("expected no new course created, got %d", len(catalog.created))
	}
}

func TestPracticumImportUseCase_ImportAll_RecordsPerCourseErrorAndContinues(t *testing.T) {
	remote := &fakeImportRepo{
		courses: []domain.RemoteCourse{
			{ID: "rc1", Title: "Broken"},
			{ID: "rc2", Title: "Fine"},
		},
		lessons: map[string][]domain.RemoteLesson{
			"rc2": {{ID: "rl1", Title: "Intro"}},
		},
	}
	catalog := newImportCatalogRepo()
	lessons := newImportLessonRepo()
	// Fail lesson creation only for the first course's (nonexistent) lesson list —
	// simulate a downstream failure by making CreateLesson itself error.
	lessons.createLessonErr = errors.New("boom")

	uc := NewPracticumImportUseCase(remote, catalog, lessons, t.TempDir())
	summary, err := uc.ImportAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	// Both courses import fine (course creation never fails here); the lesson
	// creation failure is what's recorded, once per lesson attempted.
	if summary.CoursesImported != 2 {
		t.Errorf("courses imported = %d, want 2", summary.CoursesImported)
	}
	if len(summary.Errors) != 1 {
		t.Errorf("expected 1 recorded error, got %v", summary.Errors)
	}
}
