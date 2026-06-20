package usecase

import (
	"context"
	"fmt"

	"github.com/student-learning-portal/backend/internal/domain"
)

type PlayerUseCase struct {
	lessons domain.LessonRepository
}

func NewPlayerUseCase(lessons domain.LessonRepository) *PlayerUseCase {
	return &PlayerUseCase{lessons: lessons}
}

func (uc *PlayerUseCase) GetLesson(ctx context.Context, courseID, lessonID string) (domain.Lesson, error) {
	lesson, err := uc.lessons.GetLesson(ctx, courseID, lessonID)
	if err != nil {
		return domain.Lesson{}, fmt.Errorf("get lesson: %w", err)
	}
	return lesson, nil
}
