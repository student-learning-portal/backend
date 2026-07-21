package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
)

// fakeNotificationRepo is a shared in-memory domain.NotificationRepository for
// the notification and chat-notification tests.
type fakeNotificationRepo struct {
	created   []domain.Notification
	unread    int
	list      []domain.Notification
	createErr error
	markedAll bool
	markedIDs []string
}

func (f *fakeNotificationRepo) Create(_ context.Context, n domain.Notification) (domain.Notification, error) {
	if f.createErr != nil {
		return domain.Notification{}, f.createErr
	}
	n.ID = "n-1"
	f.created = append(f.created, n)
	return n, nil
}

func (f *fakeNotificationRepo) ListForUser(_ context.Context, _ string, _ int) ([]domain.Notification, error) {
	return f.list, nil
}

func (f *fakeNotificationRepo) CountUnread(_ context.Context, _ string) (int, error) {
	return f.unread, nil
}

func (f *fakeNotificationRepo) MarkRead(_ context.Context, _, id string) error {
	f.markedIDs = append(f.markedIDs, id)
	return nil
}

func (f *fakeNotificationRepo) MarkAllRead(_ context.Context, _ string) error {
	f.markedAll = true
	return nil
}

func TestNotificationUseCase_UnreadCount(t *testing.T) {
	repo := &fakeNotificationRepo{unread: 3}
	uc := NewNotificationUseCase(repo)

	got, err := uc.UnreadCount(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("UnreadCount: %v", err)
	}
	if got != 3 {
		t.Errorf("unread = %d, want 3", got)
	}
}

func TestNotificationUseCase_MarkReadAndAll(t *testing.T) {
	repo := &fakeNotificationRepo{}
	uc := NewNotificationUseCase(repo)

	if err := uc.MarkRead(context.Background(), "user-1", "n-42"); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if len(repo.markedIDs) != 1 || repo.markedIDs[0] != "n-42" {
		t.Errorf("markedIDs = %v, want [n-42]", repo.markedIDs)
	}

	if err := uc.MarkAllRead(context.Background(), "user-1"); err != nil {
		t.Fatalf("MarkAllRead: %v", err)
	}
	if !repo.markedAll {
		t.Error("MarkAllRead did not reach the repository")
	}
}

func TestNotificationUseCase_ListPassesThrough(t *testing.T) {
	repo := &fakeNotificationRepo{list: []domain.Notification{{ID: "n-1"}, {ID: "n-2"}}}
	uc := NewNotificationUseCase(repo)

	got, err := uc.List(context.Background(), "user-1", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len(list) = %d, want 2", len(got))
	}
}

// --- chat -> notification emission ---

func TestStudentSend_NotifiesTeacher(t *testing.T) {
	notif := &fakeNotificationRepo{}
	cat := &stubCatalogRepository{course: domain.Course{ID: "course-1", TeacherID: "teach-1", Title: "Алгебра"}}
	users := &stubAuthUserRepo{user: domain.User{ID: "stu-1", FullName: "Иван Ученик"}}
	uc := NewChatUseCase(
		&fakeChatRepo{}, cat, &stubEntitlementRepo{hasActiveGrant: true},
		WithNotifications(notif, users),
	)

	if _, err := uc.StudentSend(context.Background(), "stu-1", "course-1", nil, "здравствуйте"); err != nil {
		t.Fatalf("StudentSend: %v", err)
	}
	if len(notif.created) != 1 {
		t.Fatalf("created %d notifications, want 1", len(notif.created))
	}
	n := notif.created[0]
	if n.UserID != "teach-1" {
		t.Errorf("recipient = %q, want teacher teach-1", n.UserID)
	}
	if n.Type != domain.NotificationTypeMessage {
		t.Errorf("type = %q, want message", n.Type)
	}
	if n.CourseID == nil || *n.CourseID != "course-1" {
		t.Errorf("course_id = %v, want course-1", n.CourseID)
	}
}

func TestTeacherSend_NotifiesStudent(t *testing.T) {
	notif := &fakeNotificationRepo{}
	cat := &stubCatalogRepository{course: domain.Course{ID: "course-1", TeacherID: "teach-1", Title: "Алгебра"}}
	users := &stubAuthUserRepo{user: domain.User{ID: "teach-1", FullName: "Пётр Преподаватель"}}
	uc := NewChatUseCase(
		&fakeChatRepo{}, cat, &stubEntitlementRepo{},
		WithNotifications(notif, users),
	)

	if _, err := uc.TeacherSend(context.Background(), "teach-1", "course-1", "stu-1", nil, "ответ"); err != nil {
		t.Fatalf("TeacherSend: %v", err)
	}
	if len(notif.created) != 1 {
		t.Fatalf("created %d notifications, want 1", len(notif.created))
	}
	if got := notif.created[0].UserID; got != "stu-1" {
		t.Errorf("recipient = %q, want student stu-1", got)
	}
}

func TestStudentSend_NotificationFailureDoesNotFailSend(t *testing.T) {
	notif := &fakeNotificationRepo{createErr: errors.New("db down")}
	cat := &stubCatalogRepository{course: domain.Course{ID: "course-1", TeacherID: "teach-1"}}
	users := &stubAuthUserRepo{user: domain.User{ID: "stu-1", FullName: "Иван"}}
	uc := NewChatUseCase(
		&fakeChatRepo{}, cat, &stubEntitlementRepo{hasActiveGrant: true},
		WithNotifications(notif, users),
	)

	msg, err := uc.StudentSend(context.Background(), "stu-1", "course-1", nil, "текст")
	if err != nil {
		t.Fatalf("StudentSend should succeed despite notification failure: %v", err)
	}
	if msg.Body != "текст" {
		t.Errorf("body = %q, want 'текст'", msg.Body)
	}
}
