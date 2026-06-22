package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/student-learning-portal/backend/internal/domain"
)

type PlayerUseCase struct {
	lessons  domain.LessonRepository
	progress domain.ProgressRepository
}

func NewPlayerUseCase(lessons domain.LessonRepository, progress domain.ProgressRepository) *PlayerUseCase {
	return &PlayerUseCase{lessons: lessons, progress: progress}
}

// LessonContent is everything the player needs to render and resume a lesson:
// the lesson metadata, its playable media and attachments, and the learner's
// last saved resume point (Progress is the zero value if they never started it).
type LessonContent struct {
	Lesson    domain.Lesson
	Media     []domain.Media
	Materials []domain.Material
	Progress  domain.ProgressState
}

// GetLessonContent loads a lesson's content for an entitled learner and folds in
// their saved resume point so playback can continue where they left off.
// Entitlement is enforced upstream by the RequireEntitlement middleware; here we
// additionally guarantee the lesson actually belongs to the requested course
// (GetLesson filters on both ids), so a grant for one course can't read another's.
func (uc *PlayerUseCase) GetLessonContent(ctx context.Context, actorID, courseID, lessonID string) (LessonContent, error) {
	lesson, err := uc.lessons.GetLesson(ctx, courseID, lessonID)
	if err != nil {
		return LessonContent{}, fmt.Errorf("get lesson: %w", err)
	}

	media, err := uc.lessons.GetLessonMedia(ctx, lessonID)
	if err != nil {
		return LessonContent{}, fmt.Errorf("get lesson media: %w", err)
	}

	materials, err := uc.lessons.GetLessonMaterials(ctx, lessonID)
	if err != nil {
		return LessonContent{}, fmt.Errorf("get lesson materials: %w", err)
	}

	progress, err := uc.progress.Get(ctx, actorID, courseID, lessonID)
	if err != nil && !errors.Is(err, domain.ErrProgressNotFound) {
		return LessonContent{}, fmt.Errorf("get progress: %w", err)
	}

	return LessonContent{Lesson: lesson, Media: media, Materials: materials, Progress: progress}, nil
}

// SaveProgress upserts the learner's resume point for a lesson. positionMs is the
// playback offset; completed marks the lesson finished. percent_complete is derived
// from the media duration when known (100 once completed) so the stored value stays
// consistent without trusting a client-supplied percentage. The lesson is validated
// to exist within the course first, so we never persist progress for a stray id.
func (uc *PlayerUseCase) SaveProgress(ctx context.Context, actorID, courseID, lessonID string, positionMs int, completed bool) (domain.ProgressState, error) {
	if positionMs < 0 {
		return domain.ProgressState{}, fmt.Errorf("%w: position_ms must not be negative", ErrValidation)
	}

	if _, err := uc.lessons.GetLesson(ctx, courseID, lessonID); err != nil {
		return domain.ProgressState{}, fmt.Errorf("save progress: %w", err)
	}

	media, err := uc.lessons.GetLessonMedia(ctx, lessonID)
	if err != nil {
		return domain.ProgressState{}, fmt.Errorf("save progress: media: %w", err)
	}

	state := domain.ProgressState{
		ActorID:         actorID,
		CourseID:        courseID,
		LessonID:        lessonID,
		PositionMs:      positionMs,
		PercentComplete: derivePercent(positionMs, completed, media),
	}
	if err := uc.progress.Save(ctx, state); err != nil {
		return domain.ProgressState{}, fmt.Errorf("save progress: %w", err)
	}

	// Reflect what was just stored, including the server-side timestamp.
	saved, err := uc.progress.Get(ctx, actorID, courseID, lessonID)
	if err != nil {
		return domain.ProgressState{}, fmt.Errorf("save progress: reload: %w", err)
	}
	return saved, nil
}

// GetProgress returns the learner's saved resume point for a lesson, or
// domain.ErrProgressNotFound if they have not started it yet.
func (uc *PlayerUseCase) GetProgress(ctx context.Context, actorID, courseID, lessonID string) (domain.ProgressState, error) {
	progress, err := uc.progress.Get(ctx, actorID, courseID, lessonID)
	if err != nil {
		return domain.ProgressState{}, fmt.Errorf("get progress: %w", err)
	}
	return progress, nil
}

// derivePercent computes percent_complete from the playback position relative to
// the longest media asset on the lesson. A completed lesson is always 100%; a
// lesson with no timed media falls back to 0/100 based on the completed flag.
func derivePercent(positionMs int, completed bool, media []domain.Media) float64 {
	if completed {
		return 100
	}

	maxDuration := 0
	for _, m := range media {
		if m.DurationMs > maxDuration {
			maxDuration = m.DurationMs
		}
	}
	if maxDuration <= 0 {
		return 0
	}

	percent := float64(positionMs) / float64(maxDuration) * 100
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}
