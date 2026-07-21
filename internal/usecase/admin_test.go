package usecase

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/student-learning-portal/backend/internal/domain"
)

// stubApprovalRepo implements domain.TeacherApprovalRepository for admin tests,
// recording the arguments each call was made with.
type stubApprovalRepo struct {
	teachers   []domain.User
	listErr    error
	listStatus domain.TeacherStatus

	updated      domain.User
	setErr       error
	setStatus    domain.TeacherStatus
	setUserID    string
	setReviewer  string
	ensureLogin  string
	ensureHash   string
	ensureName   string
	ensureUser   domain.User
	ensureNewRow bool
	ensureErr    error
}

func (s *stubApprovalRepo) ListTeachersByStatus(
	_ context.Context, status domain.TeacherStatus,
) ([]domain.User, error) {
	s.listStatus = status
	return s.teachers, s.listErr
}

func (s *stubApprovalRepo) SetTeacherStatus(
	_ context.Context, userID string, status domain.TeacherStatus, reviewerID string,
) (domain.User, error) {
	s.setUserID, s.setStatus, s.setReviewer = userID, status, reviewerID
	if s.setErr != nil {
		return domain.User{}, s.setErr
	}
	u := s.updated
	u.ID, u.TeacherStatus = userID, status
	return u, nil
}

func (s *stubApprovalRepo) EnsureAdmin(
	_ context.Context, login, passwordHash, fullName string,
) (domain.User, bool, error) {
	s.ensureLogin, s.ensureHash, s.ensureName = login, passwordHash, fullName
	return s.ensureUser, s.ensureNewRow, s.ensureErr
}

// --- the review queue ---

func TestTeachers_FiltersOnRequestedStatus(t *testing.T) {
	repo := &stubApprovalRepo{teachers: []domain.User{{ID: "t1"}}}
	uc := NewAdminUseCase(repo)

	got, err := uc.Teachers(context.Background(), domain.TeacherStatusPending)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.listStatus != domain.TeacherStatusPending {
		t.Errorf("queried status = %q, want pending", repo.listStatus)
	}
	if len(got) != 1 {
		t.Errorf("got %d teachers, want 1", len(got))
	}
}

func TestTeachers_EmptyStatusListsEveryone(t *testing.T) {
	repo := &stubApprovalRepo{}
	uc := NewAdminUseCase(repo)

	if _, err := uc.Teachers(context.Background(), ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.listStatus != "" {
		t.Errorf("queried status = %q, want the unfiltered listing", repo.listStatus)
	}
}

func TestTeachers_RejectsUnknownStatus(t *testing.T) {
	uc := NewAdminUseCase(&stubApprovalRepo{})

	_, err := uc.Teachers(context.Background(), domain.TeacherStatus("maybe"))
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

// --- decisions ---

func TestApproveTeacher_RecordsStatusAndReviewer(t *testing.T) {
	repo := &stubApprovalRepo{}
	uc := NewAdminUseCase(repo)

	user, err := uc.ApproveTeacher(context.Background(), "t1", "admin-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.setUserID != "t1" || repo.setReviewer != "admin-1" {
		t.Errorf("decision recorded for (%q, reviewer %q), want (t1, admin-1)", repo.setUserID, repo.setReviewer)
	}
	if repo.setStatus != domain.TeacherStatusApproved || user.TeacherStatus != domain.TeacherStatusApproved {
		t.Errorf("status = %q, want approved", repo.setStatus)
	}
}

func TestRejectTeacher_RecordsRejection(t *testing.T) {
	repo := &stubApprovalRepo{}
	uc := NewAdminUseCase(repo)

	if _, err := uc.RejectTeacher(context.Background(), "t1", "admin-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.setStatus != domain.TeacherStatusRejected {
		t.Errorf("status = %q, want rejected", repo.setStatus)
	}
}

func TestApproveTeacher_RequiresTeacherID(t *testing.T) {
	uc := NewAdminUseCase(&stubApprovalRepo{})

	_, err := uc.ApproveTeacher(context.Background(), "   ", "admin-1")
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestApproveTeacher_PropagatesNotTeacher(t *testing.T) {
	uc := NewAdminUseCase(&stubApprovalRepo{setErr: domain.ErrNotTeacher})

	_, err := uc.ApproveTeacher(context.Background(), "s1", "admin-1")
	if !errors.Is(err, domain.ErrNotTeacher) {
		t.Fatalf("error = %v, want ErrNotTeacher", err)
	}
}

// --- bootstrap ---

func TestEnsureAdminAccount_HashesPasswordAndNormalizesLogin(t *testing.T) {
	repo := &stubApprovalRepo{ensureNewRow: true}
	uc := NewAdminUseCase(repo)

	created, err := uc.EnsureAdminAccount(context.Background(), "  Admin  ", "admin111", "Администратор")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("created = false, want true when the repository inserted a row")
	}
	if repo.ensureLogin != "admin" {
		t.Errorf("login = %q, want %q", repo.ensureLogin, "admin")
	}
	if repo.ensureHash == "admin111" || repo.ensureHash == "" {
		t.Fatalf("password must be stored hashed, got %q", repo.ensureHash)
	}
	if err = bcrypt.CompareHashAndPassword([]byte(repo.ensureHash), []byte("admin111")); err != nil {
		t.Errorf("stored hash does not verify the configured password: %v", err)
	}
}

func TestEnsureAdminAccount_RejectsWeakOrMissingCredentials(t *testing.T) {
	uc := NewAdminUseCase(&stubApprovalRepo{})

	for name, tc := range map[string]struct{ login, password string }{
		"no login":       {"", "admin111"},
		"no password":    {"admin", ""},
		"short password": {"admin", "admin"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := uc.EnsureAdminAccount(context.Background(), tc.login, tc.password, "Admin"); !errors.Is(err, ErrValidation) {
				t.Fatalf("error = %v, want ErrValidation", err)
			}
		})
	}
}

func TestEnsureAdminAccount_ReportsLoginTakenByAnotherAccount(t *testing.T) {
	uc := NewAdminUseCase(&stubApprovalRepo{ensureErr: domain.ErrLoginTaken})

	_, err := uc.EnsureAdminAccount(context.Background(), "admin", "admin111", "Admin")
	if !errors.Is(err, domain.ErrLoginTaken) {
		t.Fatalf("error = %v, want ErrLoginTaken", err)
	}
}

// --- registration side of the workflow ---

func TestRegister_TeacherLandsInTheApprovalQueue(t *testing.T) {
	repo := &stubAuthUserRepo{}
	uc := NewAuthUseCase(repo, &stubAuthTokenService{token: "tok"})

	_, user, err := uc.Register(domain.RegisterInput{
		Email: "tess@example.com", Password: "password1", FullName: "Tess", Role: domain.RoleTeacher,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.TeacherStatus != domain.TeacherStatusPending {
		t.Errorf("teacher_status = %q, want pending", user.TeacherStatus)
	}
}

func TestRegister_StudentHasNoApprovalState(t *testing.T) {
	repo := &stubAuthUserRepo{}
	uc := NewAuthUseCase(repo, &stubAuthTokenService{token: "tok"})

	_, user, err := uc.Register(domain.RegisterInput{
		Email: "sam@example.com", Password: "password1", FullName: "Sam", Role: domain.RoleStudent,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.TeacherStatus != "" {
		t.Errorf("teacher_status = %q, want empty for a student", user.TeacherStatus)
	}
}

func TestRegister_AdminRoleIsNotSelfAssignable(t *testing.T) {
	uc := NewAuthUseCase(&stubAuthUserRepo{}, &stubAuthTokenService{token: "tok"})

	_, _, err := uc.Register(domain.RegisterInput{
		Email: "wannabe@example.com", Password: "password1", FullName: "Wannabe", Role: domain.RoleAdmin,
	})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}
