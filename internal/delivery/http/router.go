package http

import (
	"net/http"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

// Handlers groups the per-domain HTTP handlers so the router assembly stays a
// single dependency bundle rather than a long positional parameter list.
type Handlers struct {
	Catalog        *CatalogHandler
	Auth           *AuthHandler
	Purchase       *PurchaseHandler
	Player         *PlayerHandler
	UserCourses    *UserCoursesHandler
	Profile        *ProfileHandler
	Analytics      *AnalyticsHandler
	Results        *ResultsHandler
	TeacherContent *TeacherContentHandler
	Chat           *ChatHandler
}

// NewRouter creates a new HTTP multiplexer and registers all project routes.
// The returned handler is wrapped in WithLogContext so every request carries the
// request-scoped logging context the analytics recorder reads, and in
// WithAccessLog so every request emits a structured operational log line.
func NewRouter(
	h Handlers,
	tokens domain.TokenService,
	entitlements domain.EntitlementRepository,
	catalog domain.CatalogRepository,
	analytics *usecase.AnalyticsRecorder,
	uploadsDir string,
) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/hello", HelloHandler)
	mux.HandleFunc("/api/v1/health/db", DBHealthHandler)

	mux.HandleFunc("GET /api/v1/catalog/courses", h.Catalog.GetCourses)
	mux.HandleFunc("GET /api/v1/catalog/courses/{course_id}/lessons", h.Catalog.GetCourseLessons)

	mux.HandleFunc("POST /api/v1/auth/register", h.Auth.Register)
	mux.HandleFunc("POST /api/v1/auth/login", h.Auth.Login)
	mux.HandleFunc("GET /api/v1/auth/me", RequireAuth(tokens)(h.Auth.Me))

	mux.HandleFunc("GET /api/v1/teachers/{teacher_id}", h.Auth.GetTeacher)

	auth := RequireAuth(tokens)
	guard := RequireEntitlement(entitlements, catalog, analytics)

	mux.HandleFunc("GET /api/v1/users/me/courses", auth(h.UserCourses.MyCourses))
	mux.HandleFunc("GET /api/v1/users/me/results", auth(h.Results.MyResults))

	mux.HandleFunc("PATCH /api/v1/users/me/email", auth(h.Profile.PatchEmail))
	mux.HandleFunc("PATCH /api/v1/users/me/password", auth(h.Profile.PatchPassword))
	mux.HandleFunc("PATCH /api/v1/users/me/name", auth(h.Profile.PatchName))
	mux.HandleFunc("POST /api/v1/users/me/avatar", auth(h.Profile.PostAvatar))

	mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(uploadsDir))))

	mux.HandleFunc("POST /api/v1/purchase/checkout", auth(h.Purchase.Checkout))
	mux.HandleFunc("POST /api/v1/purchase/refund", auth(h.Purchase.Refund))
	mux.HandleFunc("POST /api/v1/purchase/webhook", h.Purchase.Webhook)
	mux.HandleFunc("GET /api/v1/purchase/history", auth(h.Purchase.History))

	mux.HandleFunc(
		"GET /api/v1/player/courses/{course_id}/lessons/{lesson_id}",
		auth(guard(h.Player.GetLesson)),
	)
	mux.HandleFunc(
		"POST /api/v1/player/courses/{course_id}/lessons/{lesson_id}/progress",
		auth(guard(h.Player.SaveProgress)),
	)
	mux.HandleFunc(
		"GET /api/v1/player/courses/{course_id}/lessons/{lesson_id}/progress",
		auth(guard(h.Player.GetProgress)),
	)

	// Analytics: role (+ ownership, for the teacher view) are enforced inside the handlers.
	mux.HandleFunc("GET /api/v1/analytics/teacher/dashboard", auth(h.Analytics.TeacherDashboard))
	mux.HandleFunc("GET /api/v1/analytics/student/me", auth(h.Analytics.StudentDashboard))

	// Teacher course/lesson authoring: role + ownership are enforced inside the handlers.
	mux.HandleFunc("POST /api/v1/teacher/courses", auth(h.TeacherContent.CreateCourse))
	mux.HandleFunc("PATCH /api/v1/teacher/courses/{course_id}", auth(h.TeacherContent.UpdateCourse))
	mux.HandleFunc("DELETE /api/v1/teacher/courses/{course_id}", auth(h.TeacherContent.DeleteCourse))

	mux.HandleFunc("POST /api/v1/teacher/courses/{course_id}/lessons", auth(h.TeacherContent.CreateLesson))
	mux.HandleFunc("PUT /api/v1/teacher/courses/{course_id}/lessons/order", auth(h.TeacherContent.ReorderLessons))
	mux.HandleFunc("PATCH /api/v1/teacher/courses/{course_id}/lessons/{lesson_id}", auth(h.TeacherContent.UpdateLesson))
	mux.HandleFunc("DELETE /api/v1/teacher/courses/{course_id}/lessons/{lesson_id}", auth(h.TeacherContent.DeleteLesson))

	mux.HandleFunc("PUT /api/v1/teacher/courses/{course_id}/lessons/{lesson_id}/media", auth(h.TeacherContent.SetLessonMedia))
	mux.HandleFunc("DELETE /api/v1/teacher/courses/{course_id}/lessons/{lesson_id}/media", auth(h.TeacherContent.DeleteLessonMedia))

	mux.HandleFunc("POST /api/v1/teacher/courses/{course_id}/lessons/{lesson_id}/materials", auth(h.TeacherContent.AddMaterial))
	mux.HandleFunc("DELETE /api/v1/teacher/courses/{course_id}/lessons/{lesson_id}/materials/{material_id}",
		auth(h.TeacherContent.DeleteMaterial))

	// Chat: student <-> teacher, scoped to a course. A student uses their own
	// thread (must be enrolled); a teacher uses per-student threads on courses
	// they own. Role + enrollment/ownership are enforced inside the handlers.
	mux.HandleFunc("GET /api/v1/courses/{course_id}/messages", auth(h.Chat.StudentThread))
	mux.HandleFunc("POST /api/v1/courses/{course_id}/messages", auth(h.Chat.StudentSend))
	mux.HandleFunc("GET /api/v1/teacher/courses/{course_id}/threads", auth(h.Chat.TeacherThreads))
	mux.HandleFunc("GET /api/v1/teacher/courses/{course_id}/threads/{student_id}/messages", auth(h.Chat.TeacherThread))
	mux.HandleFunc("POST /api/v1/teacher/courses/{course_id}/threads/{student_id}/messages", auth(h.Chat.TeacherSend))

	return WithLogContext(WithAccessLog(mux))
}
