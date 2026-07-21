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
	Review         *ReviewHandler
	Rating         *RatingHandler
	Admin          *AdminHandler
	Notification   *NotificationHandler
}

// Deps bundles what the router's middleware needs, as opposed to the per-domain
// handlers in Handlers: the token service every guarded route verifies with, the
// repositories the entitlement/approval gates read, the analytics recorder those
// gates emit to, and the directory uploaded files are served from.
type Deps struct {
	Tokens       domain.TokenService
	Entitlements domain.EntitlementRepository
	Catalog      domain.CatalogRepository
	Users        domain.UserRepository
	Analytics    *usecase.AnalyticsRecorder
	UploadsDir   string
}

// NewRouter creates a new HTTP multiplexer and registers all project routes.
// The returned handler is wrapped in WithLogContext so every request carries the
// request-scoped logging context the analytics recorder reads, and in
// WithAccessLog so every request emits a structured operational log line.
func NewRouter(h Handlers, d Deps) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/hello", HelloHandler)
	mux.HandleFunc("/api/v1/health/db", DBHealthHandler)

	mux.HandleFunc("GET /api/v1/catalog/courses", h.Catalog.GetCourses)
	mux.HandleFunc("GET /api/v1/catalog/courses/{course_id}/lessons", h.Catalog.GetCourseLessons)
	// Proxies to the practicum-team integration service (internal/practicum) — see ReviewHandler.
	mux.HandleFunc("GET /api/v1/catalog/courses/{course_id}/rating", h.Review.RatingSummary)

	auth := RequireAuth(d.Tokens)

	mux.HandleFunc("POST /api/v1/auth/register", h.Auth.Register)
	mux.HandleFunc("POST /api/v1/auth/login", h.Auth.Login)
	mux.HandleFunc("GET /api/v1/auth/me", auth(h.Auth.Me))

	mux.HandleFunc("GET /api/v1/teachers/{teacher_id}", h.Auth.GetTeacher)

	registerRatingRoutes(mux, h, auth)
	guard := RequireEntitlement(d.Entitlements, d.Catalog, d.Analytics)

	// teacherOnly chains the approval gate after authentication: a teacher whose
	// registration an administrator hasn't confirmed yet is turned away before
	// reaching any authoring/monitoring handler (see RequireApprovedTeacher).
	approved := RequireApprovedTeacher(d.Users)
	teacherOnly := func(next http.HandlerFunc) http.HandlerFunc { return auth(approved(next)) }

	registerTeacherContentRoutes(mux, h, teacherOnly)
	registerAdminRoutes(mux, h, auth)
	registerChatRoutes(mux, h, auth, teacherOnly)
	registerNotificationRoutes(mux, h, auth)

	mux.HandleFunc("GET /api/v1/users/me/courses", auth(h.UserCourses.MyCourses))
	mux.HandleFunc("GET /api/v1/users/me/results", auth(h.Results.MyResults))

	// Proxies to the practicum-team integration service (internal/practicum) — see ReviewHandler.
	mux.HandleFunc("POST /api/v1/catalog/courses/{course_id}/comments", auth(h.Review.CreateReview))

	mux.HandleFunc("PATCH /api/v1/users/me/email", auth(h.Profile.PatchEmail))
	mux.HandleFunc("PATCH /api/v1/users/me/password", auth(h.Profile.PatchPassword))
	mux.HandleFunc("PATCH /api/v1/users/me/name", auth(h.Profile.PatchName))
	mux.HandleFunc("POST /api/v1/users/me/avatar", auth(h.Profile.PostAvatar))

	mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(d.UploadsDir))))

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
	mux.HandleFunc("GET /api/v1/analytics/teacher/dashboard", teacherOnly(h.Analytics.TeacherDashboard))
	mux.HandleFunc("GET /api/v1/analytics/student/me", auth(h.Analytics.StudentDashboard))

	return WithLogContext(WithAccessLog(mux))
}

// middleware is one link of a handler chain (RequireAuth, the teacherOnly
// composition, ...), named so the route groups below read as a short signature
// instead of repeating the function type.
type middleware = func(http.HandlerFunc) http.HandlerFunc

// registerTeacherContentRoutes wires the teacher's course/lesson authoring
// surface. Every route takes teacherOnly, so an unapproved teacher is stopped
// at the door; ownership of the specific course is enforced further in, by the
// use case. Split out of NewRouter to keep it readable.
func registerTeacherContentRoutes(mux *http.ServeMux, h Handlers, teacherOnly middleware) {
	c := h.TeacherContent
	const courses = "/api/v1/teacher/courses"
	const lessons = courses + "/{course_id}/lessons/{lesson_id}"

	mux.HandleFunc("POST "+courses, teacherOnly(c.CreateCourse))
	mux.HandleFunc("PATCH "+courses+"/{course_id}", teacherOnly(c.UpdateCourse))
	mux.HandleFunc("DELETE "+courses+"/{course_id}", teacherOnly(c.DeleteCourse))

	mux.HandleFunc("POST "+courses+"/{course_id}/lessons", teacherOnly(c.CreateLesson))
	mux.HandleFunc("PUT "+courses+"/{course_id}/lessons/order", teacherOnly(c.ReorderLessons))
	mux.HandleFunc("PATCH "+lessons, teacherOnly(c.UpdateLesson))
	mux.HandleFunc("DELETE "+lessons, teacherOnly(c.DeleteLesson))

	mux.HandleFunc("PUT "+lessons+"/media", teacherOnly(c.SetLessonMedia))
	mux.HandleFunc("POST "+lessons+"/media/upload", teacherOnly(c.UploadLessonMedia))
	mux.HandleFunc("DELETE "+lessons+"/media", teacherOnly(c.DeleteLessonMedia))

	mux.HandleFunc("POST "+lessons+"/materials", teacherOnly(c.AddMaterial))
	mux.HandleFunc("POST "+lessons+"/materials/upload", teacherOnly(c.UploadMaterial))
	mux.HandleFunc("DELETE "+lessons+"/materials/{material_id}", teacherOnly(c.DeleteMaterial))
}

// registerAdminRoutes wires the teacher approval queue. RequireAdmin rejects
// every other role, so a teacher can never confirm their own registration.
func registerAdminRoutes(mux *http.ServeMux, h Handlers, auth middleware) {
	mux.HandleFunc("GET /api/v1/admin/teachers", auth(RequireAdmin(h.Admin.ListTeachers)))
	mux.HandleFunc("POST /api/v1/admin/teachers/{user_id}/approve", auth(RequireAdmin(h.Admin.ApproveTeacher)))
	mux.HandleFunc("POST /api/v1/admin/teachers/{user_id}/reject", auth(RequireAdmin(h.Admin.RejectTeacher)))
}

// registerChatRoutes wires the course-scoped student <-> teacher chat. A student
// uses their own thread (must be enrolled); a teacher uses per-student threads
// on courses they own. Role + enrollment/ownership are enforced inside the
// handlers; the teacher side additionally requires an approved account.
func registerChatRoutes(mux *http.ServeMux, h Handlers, auth, teacherOnly middleware) {
	const threads = "/api/v1/teacher/courses/{course_id}/threads"

	mux.HandleFunc("GET /api/v1/courses/{course_id}/messages", auth(h.Chat.StudentThread))
	mux.HandleFunc("POST /api/v1/courses/{course_id}/messages", auth(h.Chat.StudentSend))
	mux.HandleFunc("GET "+threads, teacherOnly(h.Chat.TeacherThreads))
	mux.HandleFunc("GET "+threads+"/{student_id}/messages", teacherOnly(h.Chat.TeacherThread))
	mux.HandleFunc("POST "+threads+"/{student_id}/messages", teacherOnly(h.Chat.TeacherSend))
}

// registerNotificationRoutes wires the authenticated user's in-app "bell" feed.
// Every route is auth-guarded and scoped to the caller inside the handler, so
// any signed-in role (student, teacher, admin) sees only their own feed.
func registerNotificationRoutes(mux *http.ServeMux, h Handlers, auth middleware) {
	const base = "/api/v1/notifications"

	mux.HandleFunc("GET "+base, auth(h.Notification.List))
	mux.HandleFunc("GET "+base+"/unread-count", auth(h.Notification.UnreadCount))
	mux.HandleFunc("POST "+base+"/read-all", auth(h.Notification.MarkAllRead))
	mux.HandleFunc("POST "+base+"/{id}/read", auth(h.Notification.MarkRead))
}

// registerRatingRoutes wires the local 1-10 rating system (separate from the
// practicum-proxied course review/rating under /catalog/courses/{course_id}/rating
// and /comments): see RatingHandler. Split out of NewRouter to keep it readable.
func registerRatingRoutes(mux *http.ServeMux, h Handlers, auth middleware) {
	mux.HandleFunc("GET /api/v1/teachers/{teacher_id}/ratings", h.Rating.TeacherRatingSummary)
	mux.HandleFunc("GET /api/v1/catalog/courses/{course_id}/ratings", h.Rating.CourseRatingSummary)

	mux.HandleFunc("POST /api/v1/catalog/courses/{course_id}/ratings", auth(h.Rating.RateCourse))
	mux.HandleFunc("POST /api/v1/teachers/{teacher_id}/ratings", auth(h.Rating.RateTeacher))
	mux.HandleFunc("GET /api/v1/catalog/courses/{course_id}/ratings/me", auth(h.Rating.MyCourseRating))
	mux.HandleFunc("GET /api/v1/teachers/{teacher_id}/ratings/me", auth(h.Rating.MyTeacherRating))
}
