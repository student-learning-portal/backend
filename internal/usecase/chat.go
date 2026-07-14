package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/student-learning-portal/backend/internal/domain"
)

// ChatUseCase powers the student <-> teacher course chat. Authorization is
// enforced here: a student may only touch their own thread in a course they are
// enrolled in (active grant); a teacher may only touch threads in a course they
// own.
type ChatUseCase struct {
	chat         domain.ChatRepository
	catalog      domain.CatalogRepository
	entitlements domain.EntitlementRepository
}

func NewChatUseCase(chat domain.ChatRepository, catalog domain.CatalogRepository, entitlements domain.EntitlementRepository) *ChatUseCase {
	return &ChatUseCase{chat: chat, catalog: catalog, entitlements: entitlements}
}

// StudentSend posts a message from an enrolled student to the course's teacher.
func (uc *ChatUseCase) StudentSend(ctx context.Context, studentID, courseID string, lessonID *string, body string) (domain.Message, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return domain.Message{}, domain.ErrEmptyMessage
	}
	if err := uc.requireEnrolled(ctx, studentID, courseID); err != nil {
		return domain.Message{}, err
	}
	return uc.chat.AddMessage(ctx, domain.Message{
		CourseID:   courseID,
		StudentID:  studentID,
		LessonID:   normalizeLessonID(lessonID),
		SenderRole: domain.RoleStudent,
		SenderID:   studentID,
		Body:       body,
	})
}

// StudentThread returns the enrolled student's own conversation for the course.
func (uc *ChatUseCase) StudentThread(ctx context.Context, studentID, courseID string) ([]domain.Message, error) {
	if err := uc.requireEnrolled(ctx, studentID, courseID); err != nil {
		return nil, err
	}
	return uc.chat.ListThread(ctx, courseID, studentID)
}

// TeacherThreads lists the student conversations in a course the teacher owns.
func (uc *ChatUseCase) TeacherThreads(ctx context.Context, teacherID, courseID string) ([]domain.ThreadSummary, error) {
	if err := uc.requireOwner(ctx, teacherID, courseID); err != nil {
		return nil, err
	}
	return uc.chat.ListThreads(ctx, courseID)
}

// TeacherThread returns one student's conversation for a course the teacher owns.
func (uc *ChatUseCase) TeacherThread(ctx context.Context, teacherID, courseID, studentID string) ([]domain.Message, error) {
	if err := uc.requireOwner(ctx, teacherID, courseID); err != nil {
		return nil, err
	}
	return uc.chat.ListThread(ctx, courseID, studentID)
}

// TeacherSend posts a teacher reply into a specific student's thread.
func (uc *ChatUseCase) TeacherSend(
	ctx context.Context, teacherID, courseID, studentID string, lessonID *string, body string,
) (domain.Message, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return domain.Message{}, domain.ErrEmptyMessage
	}
	if err := uc.requireOwner(ctx, teacherID, courseID); err != nil {
		return domain.Message{}, err
	}
	return uc.chat.AddMessage(ctx, domain.Message{
		CourseID:   courseID,
		StudentID:  studentID,
		LessonID:   normalizeLessonID(lessonID),
		SenderRole: domain.RoleTeacher,
		SenderID:   teacherID,
		Body:       body,
	})
}

// requireEnrolled fails with domain.ErrForbidden unless the student holds an
// active grant for the course.
func (uc *ChatUseCase) requireEnrolled(ctx context.Context, studentID, courseID string) error {
	ok, err := uc.entitlements.HasActiveGrant(ctx, studentID, courseID)
	if err != nil {
		return fmt.Errorf("check enrollment: %w", err)
	}
	if !ok {
		return domain.ErrForbidden
	}
	return nil
}

// requireOwner fails with domain.ErrCourseNotFound / domain.ErrForbidden unless
// the teacher owns the course.
func (uc *ChatUseCase) requireOwner(ctx context.Context, teacherID, courseID string) error {
	course, err := uc.catalog.GetByID(ctx, courseID)
	if err != nil {
		return err
	}
	if course.TeacherID != teacherID {
		return domain.ErrForbidden
	}
	return nil
}

func normalizeLessonID(lessonID *string) *string {
	if lessonID == nil || strings.TrimSpace(*lessonID) == "" {
		return nil
	}
	return lessonID
}
