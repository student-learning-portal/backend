package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/student-learning-portal/backend/internal/domain"
)

// fakeLessonRepo is an in-memory domain.LessonRepository for tests.
type fakeLessonRepo struct {
	lesson    domain.Lesson
	lessonErr error
	media     []domain.Media
	materials []domain.Material
}

func (f *fakeLessonRepo) GetLessonsByCourseID(_ context.Context, _ string) ([]domain.Lesson, error) {
	return nil, nil
}

func (f *fakeLessonRepo) GetLesson(_ context.Context, _, _ string) (domain.Lesson, error) {
	return f.lesson, f.lessonErr
}

func (f *fakeLessonRepo) GetLessonMedia(_ context.Context, _ string) ([]domain.Media, error) {
	return f.media, nil
}

func (f *fakeLessonRepo) GetLessonMaterials(_ context.Context, _ string) ([]domain.Material, error) {
	return f.materials, nil
}

func (f *fakeLessonRepo) CreateLesson(_ context.Context, courseID, title, lessonType string) (domain.Lesson, error) {
	return domain.Lesson{CourseID: courseID, Title: title, Type: lessonType}, nil
}

func (f *fakeLessonRepo) UpdateLesson(_ context.Context, courseID, lessonID, title, lessonType string) (domain.Lesson, error) {
	return domain.Lesson{ID: lessonID, CourseID: courseID, Title: title, Type: lessonType}, f.lessonErr
}

func (f *fakeLessonRepo) ReorderLessons(_ context.Context, _ string, _ []string) error { return nil }

func (f *fakeLessonRepo) DeleteLesson(_ context.Context, _, _ string) error { return f.lessonErr }

func (f *fakeLessonRepo) SetLessonMedia(_ context.Context, lessonID, url string, durationMs int, mediaType string) (domain.Media, error) {
	return domain.Media{LessonID: lessonID, URL: url, DurationMs: durationMs, Type: mediaType}, nil
}

func (f *fakeLessonRepo) DeleteLessonMedia(_ context.Context, _ string) error { return nil }

func (f *fakeLessonRepo) AddMaterial(_ context.Context, lessonID, title, url, materialType string) (domain.Material, error) {
	return domain.Material{LessonID: lessonID, Title: title, URL: url, Type: materialType}, nil
}

func (f *fakeLessonRepo) DeleteMaterial(_ context.Context, _, _ string) error { return nil }

// fakeProgressRepo is an in-memory domain.ProgressRepository for tests.
type fakeProgressRepo struct {
	store map[string]domain.ProgressState
}

func newFakeProgressRepo() *fakeProgressRepo {
	return &fakeProgressRepo{store: map[string]domain.ProgressState{}}
}

func progressKey(actor, course, lesson string) string { return actor + "|" + course + "|" + lesson }

func (f *fakeProgressRepo) Save(_ context.Context, p domain.ProgressState) error {
	p.UpdatedAt = time.Unix(1_700_000_000, 0).UTC()
	f.store[progressKey(p.ActorID, p.CourseID, p.LessonID)] = p
	return nil
}

func (f *fakeProgressRepo) Get(_ context.Context, actor, course, lesson string) (domain.ProgressState, error) {
	p, ok := f.store[progressKey(actor, course, lesson)]
	if !ok {
		return domain.ProgressState{}, domain.ErrProgressNotFound
	}
	return p, nil
}

func newLesson() domain.Lesson {
	return domain.Lesson{ID: "lesson-1", CourseID: "course-1", Title: "Intro", Type: "video", Position: 1}
}

func TestSaveProgress_DerivesPercentFromDuration(t *testing.T) {
	lessons := &fakeLessonRepo{
		lesson: newLesson(),
		media:  []domain.Media{{DurationMs: 100_000, URL: "https://cdn/x.mp4", Type: "video"}},
	}
	uc := NewPlayerUseCase(lessons, newFakeProgressRepo())

	saved, err := uc.SaveProgress(context.Background(), "user-1", "course-1", "lesson-1", 50_000, false)
	if err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}
	if saved.PositionMs != 50_000 {
		t.Errorf("position_ms = %d, want 50000", saved.PositionMs)
	}
	if saved.PercentComplete != 50 {
		t.Errorf("percent_complete = %v, want 50", saved.PercentComplete)
	}
}

func TestSaveProgress_CompletedIsAlways100(t *testing.T) {
	lessons := &fakeLessonRepo{
		lesson: newLesson(),
		media:  []domain.Media{{DurationMs: 100_000}},
	}
	uc := NewPlayerUseCase(lessons, newFakeProgressRepo())

	saved, err := uc.SaveProgress(context.Background(), "user-1", "course-1", "lesson-1", 10_000, true)
	if err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}
	if saved.PercentComplete != 100 {
		t.Errorf("percent_complete = %v, want 100", saved.PercentComplete)
	}
}

func TestSaveProgress_NoMediaDurationFallsBackToZero(t *testing.T) {
	lessons := &fakeLessonRepo{lesson: newLesson()} // no media
	uc := NewPlayerUseCase(lessons, newFakeProgressRepo())

	saved, err := uc.SaveProgress(context.Background(), "user-1", "course-1", "lesson-1", 10_000, false)
	if err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}
	if saved.PercentComplete != 0 {
		t.Errorf("percent_complete = %v, want 0", saved.PercentComplete)
	}
}

func TestSaveProgress_NegativePositionRejected(t *testing.T) {
	uc := NewPlayerUseCase(&fakeLessonRepo{lesson: newLesson()}, newFakeProgressRepo())
	_, err := uc.SaveProgress(context.Background(), "user-1", "course-1", "lesson-1", -1, false)
	if !errors.Is(err, ErrValidation) {
		t.Errorf("err = %v, want ErrValidation", err)
	}
}

func TestSaveProgress_LessonNotFound(t *testing.T) {
	lessons := &fakeLessonRepo{lessonErr: domain.ErrLessonNotFound}
	uc := NewPlayerUseCase(lessons, newFakeProgressRepo())
	_, err := uc.SaveProgress(context.Background(), "user-1", "course-1", "missing", 0, false)
	if !errors.Is(err, domain.ErrLessonNotFound) {
		t.Errorf("err = %v, want ErrLessonNotFound", err)
	}
}

func TestGetLessonContent_FoldsInSavedProgress(t *testing.T) {
	lessons := &fakeLessonRepo{
		lesson:    newLesson(),
		media:     []domain.Media{{DurationMs: 100_000, URL: "https://cdn/x.mp4"}},
		materials: []domain.Material{{Title: "Slides", URL: "https://cdn/x.pdf", Type: "pdf"}},
	}
	progress := newFakeProgressRepo()
	uc := NewPlayerUseCase(lessons, progress)

	// Learner had previously made progress.
	if _, err := uc.SaveProgress(context.Background(), "user-1", "course-1", "lesson-1", 40_000, false); err != nil {
		t.Fatalf("seed SaveProgress: %v", err)
	}

	content, err := uc.GetLessonContent(context.Background(), "user-1", "course-1", "lesson-1")
	if err != nil {
		t.Fatalf("GetLessonContent: %v", err)
	}
	if content.Progress.PositionMs != 40_000 {
		t.Errorf("resume position = %d, want 40000", content.Progress.PositionMs)
	}
	if len(content.Materials) != 1 {
		t.Errorf("materials = %d, want 1", len(content.Materials))
	}
}

func TestGetLessonContent_NoProgressIsNotAnError(t *testing.T) {
	lessons := &fakeLessonRepo{lesson: newLesson(), media: []domain.Media{{URL: "https://cdn/x.mp4"}}}
	uc := NewPlayerUseCase(lessons, newFakeProgressRepo())

	content, err := uc.GetLessonContent(context.Background(), "fresh-user", "course-1", "lesson-1")
	if err != nil {
		t.Fatalf("GetLessonContent returned error for fresh learner: %v", err)
	}
	if content.Progress.PositionMs != 0 {
		t.Errorf("resume position = %d, want 0", content.Progress.PositionMs)
	}
}

func TestGetProgress_NotFound(t *testing.T) {
	uc := NewPlayerUseCase(&fakeLessonRepo{lesson: newLesson()}, newFakeProgressRepo())
	_, err := uc.GetProgress(context.Background(), "user-1", "course-1", "lesson-1")
	if !errors.Is(err, domain.ErrProgressNotFound) {
		t.Errorf("err = %v, want ErrProgressNotFound", err)
	}
}
