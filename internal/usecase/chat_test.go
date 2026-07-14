package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/student-learning-portal/backend/internal/domain"
)

type fakeChatRepo struct {
	added   []domain.Message
	thread  []domain.Message
	threads []domain.ThreadSummary
	err     error
}

func (f *fakeChatRepo) AddMessage(_ context.Context, m domain.Message) (domain.Message, error) {
	if f.err != nil {
		return domain.Message{}, f.err
	}
	m.ID = "m-1"
	f.added = append(f.added, m)
	return m, nil
}

func (f *fakeChatRepo) ListThread(_ context.Context, _, _ string) ([]domain.Message, error) {
	return f.thread, f.err
}

func (f *fakeChatRepo) ListThreads(_ context.Context, _ string) ([]domain.ThreadSummary, error) {
	return f.threads, f.err
}

func TestStudentSend_EnrolledStoresStudentMessage(t *testing.T) {
	chat := &fakeChatRepo{}
	uc := NewChatUseCase(chat, &stubCatalogRepository{}, &stubEntitlementRepo{hasActiveGrant: true})

	msg, err := uc.StudentSend(context.Background(), "stu-1", "course-1", nil, "  hello teacher  ")
	if err != nil {
		t.Fatalf("StudentSend: %v", err)
	}
	if msg.Body != "hello teacher" {
		t.Errorf("body = %q, want trimmed 'hello teacher'", msg.Body)
	}
	if msg.SenderRole != domain.RoleStudent || msg.SenderID != "stu-1" || msg.StudentID != "stu-1" {
		t.Errorf("message identity wrong: %+v", msg)
	}
}

func TestStudentSend_NotEnrolledForbidden(t *testing.T) {
	uc := NewChatUseCase(&fakeChatRepo{}, &stubCatalogRepository{}, &stubEntitlementRepo{hasActiveGrant: false})
	_, err := uc.StudentSend(context.Background(), "stu-1", "course-1", nil, "hi")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("err = %v, want ErrForbidden", err)
	}
}

func TestStudentSend_EmptyBodyRejected(t *testing.T) {
	uc := NewChatUseCase(&fakeChatRepo{}, &stubCatalogRepository{}, &stubEntitlementRepo{hasActiveGrant: true})
	_, err := uc.StudentSend(context.Background(), "stu-1", "course-1", nil, "   ")
	if !errors.Is(err, domain.ErrEmptyMessage) {
		t.Errorf("err = %v, want ErrEmptyMessage", err)
	}
}

func TestStudentThread_NotEnrolledForbidden(t *testing.T) {
	uc := NewChatUseCase(&fakeChatRepo{}, &stubCatalogRepository{}, &stubEntitlementRepo{hasActiveGrant: false})
	_, err := uc.StudentThread(context.Background(), "stu-1", "course-1")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("err = %v, want ErrForbidden", err)
	}
}

func TestTeacherSend_OwnerStoresTeacherMessage(t *testing.T) {
	chat := &fakeChatRepo{}
	cat := &stubCatalogRepository{course: domain.Course{ID: "course-1", TeacherID: "teach-1"}}
	uc := NewChatUseCase(chat, cat, &stubEntitlementRepo{})

	msg, err := uc.TeacherSend(context.Background(), "teach-1", "course-1", "stu-1", nil, "here's help")
	if err != nil {
		t.Fatalf("TeacherSend: %v", err)
	}
	if msg.SenderRole != domain.RoleTeacher || msg.SenderID != "teach-1" || msg.StudentID != "stu-1" {
		t.Errorf("message identity wrong: %+v", msg)
	}
}

func TestTeacherSend_NotOwnerForbidden(t *testing.T) {
	cat := &stubCatalogRepository{course: domain.Course{ID: "course-1", TeacherID: "someone-else"}}
	uc := NewChatUseCase(&fakeChatRepo{}, cat, &stubEntitlementRepo{})
	_, err := uc.TeacherSend(context.Background(), "teach-1", "course-1", "stu-1", nil, "hi")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("err = %v, want ErrForbidden", err)
	}
}

func TestTeacherThreads_CourseNotFoundPropagates(t *testing.T) {
	cat := &stubCatalogRepository{courseErr: domain.ErrCourseNotFound}
	uc := NewChatUseCase(&fakeChatRepo{}, cat, &stubEntitlementRepo{})
	_, err := uc.TeacherThreads(context.Background(), "teach-1", "missing")
	if !errors.Is(err, domain.ErrCourseNotFound) {
		t.Errorf("err = %v, want ErrCourseNotFound", err)
	}
}

func TestTeacherThreads_OwnerReturnsThreads(t *testing.T) {
	cat := &stubCatalogRepository{course: domain.Course{ID: "course-1", TeacherID: "teach-1"}}
	chat := &fakeChatRepo{threads: []domain.ThreadSummary{{StudentID: "stu-1", MessageCount: 2}}}
	uc := NewChatUseCase(chat, cat, &stubEntitlementRepo{})

	got, err := uc.TeacherThreads(context.Background(), "teach-1", "course-1")
	if err != nil {
		t.Fatalf("TeacherThreads: %v", err)
	}
	if len(got) != 1 || got[0].StudentID != "stu-1" {
		t.Errorf("threads = %+v, want one for stu-1", got)
	}
}
