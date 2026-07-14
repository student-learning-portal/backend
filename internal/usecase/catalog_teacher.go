package usecase

import (
	"context"
	"fmt"

	"github.com/student-learning-portal/backend/internal/domain"
)

// CourseInput carries the teacher-editable fields of a course.
type CourseInput struct {
	Title           string
	Description     string
	Subject         string
	Price           float64
	Currency        string
	Difficulty      domain.DifficultyLevel
	DurationMinutes int
}

var courseStatuses = map[string]bool{
	"draft":     true,
	"published": true,
	"archived":  true,
}

// requireOwnedCourse loads a course and confirms teacherID owns it, so every
// authoring method below rejects a teacher touching someone else's course the
// same way usecase.AnalyticsUseCase.TeacherDashboard already does.
func (uc *CatalogUseCase) requireOwnedCourse(ctx context.Context, teacherID, courseID string) (domain.Course, error) {
	course, err := uc.repo.GetByID(ctx, courseID)
	if err != nil {
		return domain.Course{}, err
	}
	if course.TeacherID != teacherID {
		return domain.Course{}, domain.ErrForbidden
	}
	return course, nil
}

func validateCourseInput(in CourseInput) error {
	if in.Title == "" {
		return fmt.Errorf("%w: title is required", ErrValidation)
	}
	if in.Price < 0 {
		return fmt.Errorf("%w: price must not be negative", ErrValidation)
	}
	if in.Difficulty != "" && !in.Difficulty.Valid() {
		return fmt.Errorf("%w: difficulty must be one of beginner/intermediate/advanced/all_levels", ErrValidation)
	}
	if in.DurationMinutes < 0 {
		return fmt.Errorf("%w: duration_minutes must not be negative", ErrValidation)
	}
	return nil
}

// CreateCourse creates a new draft course owned by teacherID.
func (uc *CatalogUseCase) CreateCourse(ctx context.Context, teacherID string, in CourseInput) (domain.Course, error) {
	if err := validateCourseInput(in); err != nil {
		return domain.Course{}, err
	}
	subject := in.Subject
	if subject == "" {
		subject = "general"
	}
	currency := in.Currency
	if currency == "" {
		currency = currencyUSD
	}
	difficulty := in.Difficulty
	if difficulty == "" {
		difficulty = domain.DifficultyAllLevels
	}

	course, err := uc.repo.Create(ctx, domain.Course{
		TeacherID:       teacherID,
		Title:           in.Title,
		Description:     in.Description,
		Subject:         subject,
		Price:           in.Price,
		Currency:        currency,
		Difficulty:      difficulty,
		DurationMinutes: in.DurationMinutes,
	})
	if err != nil {
		return domain.Course{}, fmt.Errorf("create course: %w", err)
	}
	return course, nil
}

// UpdateCourse overwrites an owned course's editable fields, including status.
func (uc *CatalogUseCase) UpdateCourse(
	ctx context.Context, teacherID, courseID string, in CourseInput, status string,
) (domain.Course, error) {
	if err := validateCourseInput(in); err != nil {
		return domain.Course{}, err
	}
	if !courseStatuses[status] {
		return domain.Course{}, fmt.Errorf("%w: status must be one of draft/published/archived", ErrValidation)
	}

	existing, err := uc.requireOwnedCourse(ctx, teacherID, courseID)
	if err != nil {
		return domain.Course{}, err
	}

	subject := in.Subject
	if subject == "" {
		subject = "general"
	}
	currency := in.Currency
	if currency == "" {
		currency = currencyUSD
	}
	difficulty := in.Difficulty
	if difficulty == "" {
		difficulty = domain.DifficultyAllLevels
	}

	updated, err := uc.repo.Update(ctx, domain.Course{
		ID:              existing.ID,
		TeacherID:       existing.TeacherID,
		Title:           in.Title,
		Description:     in.Description,
		Subject:         subject,
		Price:           in.Price,
		Currency:        currency,
		Status:          status,
		Difficulty:      difficulty,
		DurationMinutes: in.DurationMinutes,
	})
	if err != nil {
		return domain.Course{}, fmt.Errorf("update course: %w", err)
	}
	return updated, nil
}

// DeleteCourse removes an owned course. Only drafts may be deleted — once a
// course has been published it may have been purchased, so it must be
// archived instead (via UpdateCourse) rather than deleted.
func (uc *CatalogUseCase) DeleteCourse(ctx context.Context, teacherID, courseID string) error {
	course, err := uc.requireOwnedCourse(ctx, teacherID, courseID)
	if err != nil {
		return err
	}
	if course.Status != "draft" {
		return domain.ErrCourseNotDraft
	}
	if err := uc.repo.Delete(ctx, courseID); err != nil {
		return fmt.Errorf("delete course: %w", err)
	}
	return nil
}

// CreateLesson appends a new lesson to an owned course.
func (uc *CatalogUseCase) CreateLesson(ctx context.Context, teacherID, courseID, title, lessonType string) (domain.Lesson, error) {
	if title == "" {
		return domain.Lesson{}, fmt.Errorf("%w: title is required", ErrValidation)
	}
	if !lessonTypes[lessonType] {
		return domain.Lesson{}, fmt.Errorf("%w: lesson_type must be one of video/text/quiz/mixed", ErrValidation)
	}
	if _, err := uc.requireOwnedCourse(ctx, teacherID, courseID); err != nil {
		return domain.Lesson{}, err
	}

	lesson, err := uc.lessons.CreateLesson(ctx, courseID, title, lessonType)
	if err != nil {
		return domain.Lesson{}, fmt.Errorf("create lesson: %w", err)
	}
	return lesson, nil
}

const (
	currencyUSD = "USD"
	videoType   = "video"
)

var lessonTypes = map[string]bool{
	videoType: true,
	"text":    true,
	"quiz":    true,
	"mixed":   true,
}

// UpdateLesson changes an owned lesson's title/type.
func (uc *CatalogUseCase) UpdateLesson(
	ctx context.Context, teacherID, courseID, lessonID, title, lessonType string,
) (domain.Lesson, error) {
	if title == "" {
		return domain.Lesson{}, fmt.Errorf("%w: title is required", ErrValidation)
	}
	if !lessonTypes[lessonType] {
		return domain.Lesson{}, fmt.Errorf("%w: lesson_type must be one of video/text/quiz/mixed", ErrValidation)
	}
	if _, err := uc.requireOwnedCourse(ctx, teacherID, courseID); err != nil {
		return domain.Lesson{}, err
	}

	lesson, err := uc.lessons.UpdateLesson(ctx, courseID, lessonID, title, lessonType)
	if err != nil {
		return domain.Lesson{}, fmt.Errorf("update lesson: %w", err)
	}
	return lesson, nil
}

// ReorderLessons reassigns lesson positions within an owned course.
func (uc *CatalogUseCase) ReorderLessons(ctx context.Context, teacherID, courseID string, orderedLessonIDs []string) error {
	if len(orderedLessonIDs) == 0 {
		return fmt.Errorf("%w: lesson_ids must not be empty", ErrValidation)
	}
	if _, err := uc.requireOwnedCourse(ctx, teacherID, courseID); err != nil {
		return err
	}
	if err := uc.lessons.ReorderLessons(ctx, courseID, orderedLessonIDs); err != nil {
		return fmt.Errorf("reorder lessons: %w", err)
	}
	return nil
}

// DeleteLesson removes an owned lesson (and its media/materials).
func (uc *CatalogUseCase) DeleteLesson(ctx context.Context, teacherID, courseID, lessonID string) error {
	if _, err := uc.requireOwnedCourse(ctx, teacherID, courseID); err != nil {
		return err
	}
	if err := uc.lessons.DeleteLesson(ctx, courseID, lessonID); err != nil {
		return fmt.Errorf("delete lesson: %w", err)
	}
	return nil
}

var mediaTypes = map[string]bool{
	videoType: true,
	"audio":   true,
}

// MediaInput carries the fields of PUT .../lessons/{id}/media, bundled into a
// struct so the method itself stays under the linter's argument-count limit.
type MediaInput struct {
	URL             string
	DurationSeconds int
	MediaType       string
}

// SetLessonMedia replaces an owned lesson's media asset. lessonID is verified
// to belong to courseID via GetLesson before writing.
func (uc *CatalogUseCase) SetLessonMedia(ctx context.Context, teacherID, courseID, lessonID string, in MediaInput) (domain.Media, error) {
	if in.URL == "" {
		return domain.Media{}, fmt.Errorf("%w: url is required", ErrValidation)
	}
	if in.DurationSeconds < 0 {
		return domain.Media{}, fmt.Errorf("%w: duration_seconds must not be negative", ErrValidation)
	}
	if !mediaTypes[in.MediaType] {
		return domain.Media{}, fmt.Errorf("%w: media_type must be one of video/audio", ErrValidation)
	}
	if _, err := uc.requireOwnedCourse(ctx, teacherID, courseID); err != nil {
		return domain.Media{}, err
	}
	if _, err := uc.lessons.GetLesson(ctx, courseID, lessonID); err != nil {
		return domain.Media{}, err
	}

	const msPerSecond = 1000
	media, err := uc.lessons.SetLessonMedia(ctx, lessonID, in.URL, in.DurationSeconds*msPerSecond, in.MediaType)
	if err != nil {
		return domain.Media{}, fmt.Errorf("set lesson media: %w", err)
	}
	return media, nil
}

// DeleteLessonMedia removes an owned lesson's media asset, if any.
func (uc *CatalogUseCase) DeleteLessonMedia(ctx context.Context, teacherID, courseID, lessonID string) error {
	if _, err := uc.requireOwnedCourse(ctx, teacherID, courseID); err != nil {
		return err
	}
	if _, err := uc.lessons.GetLesson(ctx, courseID, lessonID); err != nil {
		return err
	}
	if err := uc.lessons.DeleteLessonMedia(ctx, lessonID); err != nil {
		return fmt.Errorf("delete lesson media: %w", err)
	}
	return nil
}

// MaterialInput carries the fields of POST .../lessons/{id}/materials.
type MaterialInput struct {
	Title string
	URL   string
	Type  string
}

// AddMaterial attaches a new downloadable material to an owned lesson.
func (uc *CatalogUseCase) AddMaterial(
	ctx context.Context, teacherID, courseID, lessonID string, in MaterialInput,
) (domain.Material, error) {
	if in.Title == "" || in.URL == "" {
		return domain.Material{}, fmt.Errorf("%w: title and url are required", ErrValidation)
	}
	if _, err := uc.requireOwnedCourse(ctx, teacherID, courseID); err != nil {
		return domain.Material{}, err
	}
	if _, err := uc.lessons.GetLesson(ctx, courseID, lessonID); err != nil {
		return domain.Material{}, err
	}

	material, err := uc.lessons.AddMaterial(ctx, lessonID, in.Title, in.URL, in.Type)
	if err != nil {
		return domain.Material{}, fmt.Errorf("add material: %w", err)
	}
	return material, nil
}

// DeleteMaterial removes a single material from an owned lesson.
func (uc *CatalogUseCase) DeleteMaterial(ctx context.Context, teacherID, courseID, lessonID, materialID string) error {
	if _, err := uc.requireOwnedCourse(ctx, teacherID, courseID); err != nil {
		return err
	}
	if _, err := uc.lessons.GetLesson(ctx, courseID, lessonID); err != nil {
		return err
	}
	if err := uc.lessons.DeleteMaterial(ctx, lessonID, materialID); err != nil {
		return fmt.Errorf("delete material: %w", err)
	}
	return nil
}
