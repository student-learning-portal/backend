package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
)

func TestCreateCourse_AppliesDefaultsAndOwnership(t *testing.T) {
	repo := &stubCatalogRepository{}
	uc := NewCatalogUseCase(repo, &fakeLessonRepo{})

	course, err := uc.CreateCourse(context.Background(), "teacher-1", CourseInput{Title: "Intro to Go", Price: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if course.TeacherID != "teacher-1" {
		t.Errorf("TeacherID = %q, want teacher-1", course.TeacherID)
	}
	if course.Subject != "general" {
		t.Errorf("Subject = %q, want general (default)", course.Subject)
	}
	if course.Currency != "USD" {
		t.Errorf("Currency = %q, want USD (default)", course.Currency)
	}
}

func TestCreateCourse_ValidationErrors(t *testing.T) {
	uc := NewCatalogUseCase(&stubCatalogRepository{}, &fakeLessonRepo{})

	if _, err := uc.CreateCourse(context.Background(), "teacher-1", CourseInput{Title: ""}); !errors.Is(err, ErrValidation) {
		t.Errorf("empty title: err = %v, want ErrValidation", err)
	}
	if _, err := uc.CreateCourse(context.Background(), "teacher-1", CourseInput{Title: "x", Price: -1}); !errors.Is(err, ErrValidation) {
		t.Errorf("negative price: err = %v, want ErrValidation", err)
	}
}

func TestUpdateCourse_RejectsNonOwner(t *testing.T) {
	repo := &stubCatalogRepository{course: domain.Course{ID: "c1", TeacherID: "teacher-1"}}
	uc := NewCatalogUseCase(repo, &fakeLessonRepo{})

	_, err := uc.UpdateCourse(context.Background(), "teacher-2", "c1", CourseInput{Title: "x"}, "draft")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("err = %v, want ErrForbidden", err)
	}
}

func TestUpdateCourse_RejectsUnknownStatus(t *testing.T) {
	repo := &stubCatalogRepository{course: domain.Course{ID: "c1", TeacherID: "teacher-1"}}
	uc := NewCatalogUseCase(repo, &fakeLessonRepo{})

	_, err := uc.UpdateCourse(context.Background(), "teacher-1", "c1", CourseInput{Title: "x"}, "deleted")
	if !errors.Is(err, ErrValidation) {
		t.Errorf("err = %v, want ErrValidation", err)
	}
}

func TestUpdateCourse_Success(t *testing.T) {
	repo := &stubCatalogRepository{course: domain.Course{ID: "c1", TeacherID: "teacher-1"}}
	uc := NewCatalogUseCase(repo, &fakeLessonRepo{})

	updated, err := uc.UpdateCourse(context.Background(), "teacher-1", "c1", CourseInput{Title: "New title", Price: 5}, "published")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Title != "New title" || updated.Status != "published" {
		t.Errorf("updated = %+v, want title=New title status=published", updated)
	}
}

func TestDeleteCourse_RequiresDraft(t *testing.T) {
	repo := &stubCatalogRepository{course: domain.Course{ID: "c1", TeacherID: "teacher-1", Status: "published"}}
	uc := NewCatalogUseCase(repo, &fakeLessonRepo{})

	if err := uc.DeleteCourse(context.Background(), "teacher-1", "c1"); !errors.Is(err, domain.ErrCourseNotDraft) {
		t.Errorf("err = %v, want ErrCourseNotDraft", err)
	}
}

func TestDeleteCourse_RejectsNonOwner(t *testing.T) {
	repo := &stubCatalogRepository{course: domain.Course{ID: "c1", TeacherID: "teacher-1", Status: "draft"}}
	uc := NewCatalogUseCase(repo, &fakeLessonRepo{})

	if err := uc.DeleteCourse(context.Background(), "teacher-2", "c1"); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("err = %v, want ErrForbidden", err)
	}
}

func TestDeleteCourse_DraftSucceeds(t *testing.T) {
	repo := &stubCatalogRepository{course: domain.Course{ID: "c1", TeacherID: "teacher-1", Status: "draft"}}
	uc := NewCatalogUseCase(repo, &fakeLessonRepo{})

	if err := uc.DeleteCourse(context.Background(), "teacher-1", "c1"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCreateLesson_RejectsUnknownType(t *testing.T) {
	repo := &stubCatalogRepository{course: domain.Course{ID: "c1", TeacherID: "teacher-1"}}
	uc := NewCatalogUseCase(repo, &fakeLessonRepo{})

	_, err := uc.CreateLesson(context.Background(), "teacher-1", "c1", "Lesson 1", "podcast")
	if !errors.Is(err, ErrValidation) {
		t.Errorf("err = %v, want ErrValidation", err)
	}
}

func TestCreateLesson_RejectsNonOwner(t *testing.T) {
	repo := &stubCatalogRepository{course: domain.Course{ID: "c1", TeacherID: "teacher-1"}}
	uc := NewCatalogUseCase(repo, &fakeLessonRepo{})

	_, err := uc.CreateLesson(context.Background(), "teacher-2", "c1", "Lesson 1", "video")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("err = %v, want ErrForbidden", err)
	}
}

func TestCreateLesson_Success(t *testing.T) {
	repo := &stubCatalogRepository{course: domain.Course{ID: "c1", TeacherID: "teacher-1"}}
	uc := NewCatalogUseCase(repo, &fakeLessonRepo{})

	lesson, err := uc.CreateLesson(context.Background(), "teacher-1", "c1", "Lesson 1", "video")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lesson.Title != "Lesson 1" || lesson.Type != "video" {
		t.Errorf("lesson = %+v, want title=Lesson 1 type=video", lesson)
	}
}

func TestReorderLessons_RejectsEmptyList(t *testing.T) {
	repo := &stubCatalogRepository{course: domain.Course{ID: "c1", TeacherID: "teacher-1"}}
	uc := NewCatalogUseCase(repo, &fakeLessonRepo{})

	if err := uc.ReorderLessons(context.Background(), "teacher-1", "c1", nil); !errors.Is(err, ErrValidation) {
		t.Errorf("err = %v, want ErrValidation", err)
	}
}

func TestReorderLessons_RejectsNonOwner(t *testing.T) {
	repo := &stubCatalogRepository{course: domain.Course{ID: "c1", TeacherID: "teacher-1"}}
	uc := NewCatalogUseCase(repo, &fakeLessonRepo{})

	err := uc.ReorderLessons(context.Background(), "teacher-2", "c1", []string{"l1", "l2"})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("err = %v, want ErrForbidden", err)
	}
}

func TestSetLessonMedia_ValidationErrors(t *testing.T) {
	repo := &stubCatalogRepository{course: domain.Course{ID: "c1", TeacherID: "teacher-1"}}
	uc := NewCatalogUseCase(repo, &fakeLessonRepo{})

	cases := []struct {
		name      string
		url       string
		duration  int
		mediaType string
	}{
		{"empty url", "", 10, "video"},
		{"negative duration", "https://x/y.mp4", -1, "video"},
		{"unknown media type", "https://x/y.mp4", 10, "pdf"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := uc.SetLessonMedia(context.Background(), "teacher-1", "c1", "l1", MediaInput{
				URL: c.url, DurationSeconds: c.duration, MediaType: c.mediaType,
			})
			if !errors.Is(err, ErrValidation) {
				t.Errorf("err = %v, want ErrValidation", err)
			}
		})
	}
}

func TestSetLessonMedia_Success(t *testing.T) {
	repo := &stubCatalogRepository{course: domain.Course{ID: "c1", TeacherID: "teacher-1"}}
	uc := NewCatalogUseCase(repo, &fakeLessonRepo{lesson: domain.Lesson{ID: "l1", CourseID: "c1"}})

	media, err := uc.SetLessonMedia(context.Background(), "teacher-1", "c1", "l1", MediaInput{
		URL: "https://x/y.mp4", DurationSeconds: 90, MediaType: "video",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if media.URL != "https://x/y.mp4" || media.DurationMs != 90000 {
		t.Errorf("media = %+v, want url=https://x/y.mp4 durationMs=90000", media)
	}
}

func TestAddMaterial_RequiresTitleAndURL(t *testing.T) {
	repo := &stubCatalogRepository{course: domain.Course{ID: "c1", TeacherID: "teacher-1"}}
	uc := NewCatalogUseCase(repo, &fakeLessonRepo{lesson: domain.Lesson{ID: "l1", CourseID: "c1"}})

	if _, err := uc.AddMaterial(context.Background(), "teacher-1", "c1", "l1", MaterialInput{
		Title: "", URL: "https://x/y.pdf", Type: "pdf",
	}); !errors.Is(err, ErrValidation) {
		t.Errorf("missing title: err = %v, want ErrValidation", err)
	}
	if _, err := uc.AddMaterial(context.Background(), "teacher-1", "c1", "l1", MaterialInput{
		Title: "Slides", URL: "", Type: "pdf",
	}); !errors.Is(err, ErrValidation) {
		t.Errorf("missing url: err = %v, want ErrValidation", err)
	}
}
