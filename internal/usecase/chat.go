package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/logging"
)

// messagePreviewLen caps how much of a message body is copied into the
// notification body, so the bell feed shows a teaser rather than a wall of text.
const messagePreviewLen = 120

// ChatUseCase powers the student <-> teacher course chat. Authorization is
// enforced here: a student may only touch their own thread in a course they are
// enrolled in (active grant); a teacher may only touch threads in a course they
// own.
//
// notifications and users are optional (see WithNotifications): when wired, each
// posted message drops a 'message' notification into the *other* participant's
// bell feed. They are best-effort — a failure to notify never fails the send.
type ChatUseCase struct {
	chat          domain.ChatRepository
	catalog       domain.CatalogRepository
	entitlements  domain.EntitlementRepository
	notifications domain.NotificationRepository
	users         domain.UserRepository
}

// ChatOption configures optional ChatUseCase collaborators without widening the
// constructor for every existing caller (tests, e2e) that doesn't need them.
type ChatOption func(*ChatUseCase)

// WithNotifications enables bell-feed notifications on message send. users is
// used only to render the sender's name into the notification; if it or the
// lookup is unavailable, a generic sender label is used instead.
func WithNotifications(notifications domain.NotificationRepository, users domain.UserRepository) ChatOption {
	return func(uc *ChatUseCase) {
		uc.notifications = notifications
		uc.users = users
	}
}

func NewChatUseCase(
	chat domain.ChatRepository,
	catalog domain.CatalogRepository,
	entitlements domain.EntitlementRepository,
	opts ...ChatOption,
) *ChatUseCase {
	uc := &ChatUseCase{chat: chat, catalog: catalog, entitlements: entitlements}
	for _, opt := range opts {
		opt(uc)
	}
	return uc
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
	msg, err := uc.chat.AddMessage(ctx, domain.Message{
		CourseID:   courseID,
		StudentID:  studentID,
		LessonID:   normalizeLessonID(lessonID),
		SenderRole: domain.RoleStudent,
		SenderID:   studentID,
		Body:       body,
	})
	if err != nil {
		return domain.Message{}, err
	}
	// The other participant of a (course, student) thread is the course's
	// teacher; notify them of the incoming student message.
	if course, cerr := uc.catalog.GetByID(ctx, courseID); cerr == nil {
		uc.notify(ctx, course.TeacherID, msg, course.Title)
	}
	return msg, nil
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
	msg, err := uc.chat.AddMessage(ctx, domain.Message{
		CourseID:   courseID,
		StudentID:  studentID,
		LessonID:   normalizeLessonID(lessonID),
		SenderRole: domain.RoleTeacher,
		SenderID:   teacherID,
		Body:       body,
	})
	if err != nil {
		return domain.Message{}, err
	}
	// The student is the other participant of the thread; notify them of the
	// teacher's reply.
	courseTitle := ""
	if course, cerr := uc.catalog.GetByID(ctx, courseID); cerr == nil {
		courseTitle = course.Title
	}
	uc.notify(ctx, studentID, msg, courseTitle)
	return msg, nil
}

// notify drops a best-effort 'message' notification into recipientID's bell
// feed. It is a no-op when notifications are not wired (WithNotifications) or the
// recipient is empty, and it never returns an error: failing to notify must not
// fail the message that was already stored.
func (uc *ChatUseCase) notify(ctx context.Context, recipientID string, msg domain.Message, courseTitle string) {
	if uc.notifications == nil || recipientID == "" {
		return
	}
	senderName := uc.senderName(msg.SenderID, msg.SenderRole)

	title := "Новое сообщение"
	if courseTitle != "" {
		title = "Новое сообщение · " + courseTitle
	}
	body := senderName + ": " + previewText(msg.Body)

	courseID := msg.CourseID
	if _, err := uc.notifications.Create(ctx, domain.Notification{
		UserID:   recipientID,
		Type:     domain.NotificationTypeMessage,
		Title:    title,
		Body:     body,
		CourseID: &courseID,
	}); err != nil {
		logging.FromContext(ctx).Error("chat: failed to create message notification",
			slog.String("recipient_id", recipientID),
			slog.String("course_id", courseID),
			slog.Any("error", err),
		)
	}
}

// senderName resolves a human label for the message sender, falling back to a
// role-based label when the user lookup is unavailable or the name is blank.
func (uc *ChatUseCase) senderName(senderID string, role domain.Role) string {
	if uc.users != nil {
		if u, err := uc.users.GetByID(senderID); err == nil && strings.TrimSpace(u.FullName) != "" {
			return u.FullName
		}
	}
	if role == domain.RoleTeacher {
		return "Преподаватель"
	}
	return "Ученик"
}

// previewText trims a message body to a short single-line teaser for the bell
// feed.
func previewText(body string) string {
	body = strings.TrimSpace(body)
	runes := []rune(body)
	if len(runes) <= messagePreviewLen {
		return body
	}
	return string(runes[:messagePreviewLen]) + "…"
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
