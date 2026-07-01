package e2e

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/student-learning-portal/backend/internal/domain"
)

// TestChain_CheckoutAccessPlayProgressRefund walks the full lifecycle the issue
// calls out: a learner buys a course, the entitlement unlocks the player, they
// make and resume progress, then a refund revokes access — all through the real
// endpoints against a real database.
func TestChain_CheckoutAccessPlayProgressRefund(t *testing.T) {
	e := newTestEnv(t)
	teacher, _ := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	_, studentTok := e.register("student@example.com", "Student", domain.RoleStudent)

	courseID := e.insertCourse(teacher, "Go Mastery", "Programming", 100.00, "published")
	lessonID := e.insertLesson(courseID, "Intro", "video", 1)
	e.insertMedia(lessonID, 120000) // 120s

	lessonPath := "/api/v1/player/courses/" + courseID + "/lessons/" + lessonID
	progressPath := lessonPath + "/progress"

	// 1. Before buying: player is forbidden, and the course isn't in "my courses".
	e.requireStatus(e.do(http.MethodGet, lessonPath, studentTok, nil), http.StatusForbidden)
	if got := e.myCourseIDs(studentTok); len(got) != 0 {
		t.Fatalf("pre-purchase my courses = %v, want none", got)
	}

	// 2. Checkout: succeeds and deducts the price from the 1000 starting wallet.
	checkout := e.do(http.MethodPost, "/api/v1/purchase/checkout", studentTok, courseIDBody{CourseID: courseID})
	e.requireStatus(checkout, http.StatusOK)
	var co struct {
		Status  string  `json:"status"`
		Amount  float64 `json:"amount"`
		Balance float64 `json:"balance"`
	}
	e.decode(checkout, &co)
	if co.Status != "succeeded" || co.Amount != 100.00 || co.Balance != 900.00 {
		t.Fatalf("checkout = %+v, want succeeded/100/900", co)
	}

	// 3. Access is now visible in "my courses".
	if got := e.myCourseIDs(studentTok); len(got) != 1 || got[0] != courseID {
		t.Fatalf("post-purchase my courses = %v, want [%s]", got, courseID)
	}

	// 4. Player now serves the content, with no saved progress yet.
	play := e.do(http.MethodGet, lessonPath, studentTok, nil)
	e.requireStatus(play, http.StatusOK)
	var ld lessonData
	e.decode(play, &ld)
	if ld.ContentURL == "" || ld.LastProgressSeconds != 0 {
		t.Fatalf("first play = %+v, want content + 0 resume", ld)
	}

	// 5. Save progress at 60s (50% of 120s).
	save := e.do(http.MethodPost, progressPath, studentTok, map[string]any{"progress_seconds": 60})
	e.requireStatus(save, http.StatusOK)
	var saved progressData
	e.decode(save, &saved)
	if saved.ProgressSeconds != 60 || saved.PercentComplete != 50 {
		t.Fatalf("save = %+v, want 60s/50%%", saved)
	}

	// 6. Resume: both the dedicated progress endpoint and the lesson payload reflect it.
	resume := e.do(http.MethodGet, progressPath, studentTok, nil)
	e.requireStatus(resume, http.StatusOK)
	var resumed progressData
	e.decode(resume, &resumed)
	if resumed.ProgressSeconds != 60 {
		t.Errorf("resume progress = %d, want 60", resumed.ProgressSeconds)
	}
	play2 := e.do(http.MethodGet, lessonPath, studentTok, nil)
	var ld2 lessonData
	e.decode(play2, &ld2)
	if ld2.LastProgressSeconds != 60 || ld2.PercentComplete != 50 {
		t.Errorf("lesson resume = %d/%v, want 60/50", ld2.LastProgressSeconds, ld2.PercentComplete)
	}

	// 7. Refund: wallet restored, status refunded.
	refund := e.do(http.MethodPost, "/api/v1/purchase/refund", studentTok, courseIDBody{CourseID: courseID})
	e.requireStatus(refund, http.StatusOK)
	var rf struct {
		Status  string  `json:"status"`
		Balance float64 `json:"balance"`
	}
	e.decode(refund, &rf)
	if rf.Status != "refunded" || rf.Balance != 1000.00 {
		t.Fatalf("refund = %+v, want refunded/1000", rf)
	}

	// 8. Access is revoked: player forbidden again, course gone from "my courses".
	e.requireStatus(e.do(http.MethodGet, lessonPath, studentTok, nil), http.StatusForbidden)
	if got := e.myCourseIDs(studentTok); len(got) != 0 {
		t.Fatalf("post-refund my courses = %v, want none", got)
	}

	// 9. Double refund: no active grant remains → 404.
	e.requireStatus(
		e.do(http.MethodPost, "/api/v1/purchase/refund", studentTok, courseIDBody{CourseID: courseID}),
		http.StatusNotFound,
	)
}

func TestChain_CheckoutInsufficientFunds(t *testing.T) {
	e := newTestEnv(t)
	teacher, _ := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	_, studentTok := e.register("student@example.com", "Student", domain.RoleStudent)
	// Price above the 1000 starting wallet.
	courseID := e.insertCourse(teacher, "Expensive", "Programming", 5000.00, "published")

	resp := e.do(http.MethodPost, "/api/v1/purchase/checkout", studentTok, courseIDBody{CourseID: courseID})
	e.requireStatus(resp, http.StatusPaymentRequired) // 402
	if msg := e.errorMessage(resp); msg != "insufficient wallet balance" {
		t.Errorf("error = %q", msg)
	}
}

func TestChain_CheckoutUnknownCourseIsNotFound(t *testing.T) {
	e := newTestEnv(t)
	_, studentTok := e.register("student@example.com", "Student", domain.RoleStudent)
	resp := e.do(http.MethodPost, "/api/v1/purchase/checkout", studentTok, courseIDBody{CourseID: uuid.NewString()})
	e.requireStatus(resp, http.StatusNotFound)
}

// TestChain_WebhookGrantsThenRevokes exercises the unauthenticated gateway
// webhook path: SUCCESS grants access (player works), REFUNDED revokes it (403).
func TestChain_WebhookGrantsThenRevokes(t *testing.T) {
	e := newTestEnv(t)
	teacher, _ := e.register("teacher@example.com", "Teacher", domain.RoleTeacher)
	studentID, studentTok := e.register("student@example.com", "Student", domain.RoleStudent)
	courseID := e.insertCourse(teacher, "Go", "Programming", 49.99, "published")
	lessonID := e.insertLesson(courseID, "Intro", "video", 1)
	e.insertMedia(lessonID, 60000)
	lessonPath := "/api/v1/player/courses/" + courseID + "/lessons/" + lessonID

	txn := uuid.NewString()

	// Gateway confirms payment: webhook is public (no auth).
	ok := e.do(http.MethodPost, "/api/v1/purchase/webhook", "", webhookBody{
		TransactionID: txn, Status: "SUCCESS", UserID: studentID, CourseID: courseID,
	})
	e.requireStatus(ok, http.StatusOK)
	e.requireStatus(e.do(http.MethodGet, lessonPath, studentTok, nil), http.StatusOK)

	// Gateway reports a refund: access is revoked.
	ref := e.do(http.MethodPost, "/api/v1/purchase/webhook", "", webhookBody{
		TransactionID: txn, Status: "REFUNDED", UserID: studentID, CourseID: courseID,
	})
	e.requireStatus(ref, http.StatusOK)
	e.requireStatus(e.do(http.MethodGet, lessonPath, studentTok, nil), http.StatusForbidden)
}

func TestChain_WebhookUnknownStatusIsBadRequest(t *testing.T) {
	e := newTestEnv(t)
	resp := e.do(http.MethodPost, "/api/v1/purchase/webhook", "", webhookBody{
		TransactionID: uuid.NewString(), Status: "bogus", UserID: uuid.NewString(), CourseID: uuid.NewString(),
	})
	e.requireStatus(resp, http.StatusBadRequest)
}

// myCourseIDs returns the course ids the token's owner currently has access to.
func (e *testEnv) myCourseIDs(token string) []string {
	e.t.Helper()
	resp := e.do(http.MethodGet, "/api/v1/users/me/courses", token, nil)
	e.requireStatus(resp, http.StatusOK)
	var courses []course
	e.decode(resp, &courses)
	ids := make([]string, 0, len(courses))
	for _, c := range courses {
		ids = append(ids, c.ID)
	}
	return ids
}
