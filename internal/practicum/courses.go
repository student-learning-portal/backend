package practicum

import (
	"context"
	"net/http"

	"github.com/student-learning-portal/backend/internal/domain"
)

// createCourseRequest / courseResponse match their
// internal/transport/http/courses/{create,response}.go exactly.
type createCourseRequest struct {
	Name        string `json:"name"`
	Subject     string `json:"subject"`
	Description string `json:"description"`
	Duration    int    `json:"duration"`
	Price       int    `json:"price"`
	Difficulty  string `json:"difficulty"`
}

type courseResponse struct {
	ID string `json:"id"`
}

// createCourse mirrors our course into their system, owned by
// integrationTeacherID (a teacher account that must already exist in *their*
// teachers table — see docs/practicum-integration.md for the one-time setup).
// Returns their course ID.
func (c *Client) createCourse(ctx context.Context, integrationTeacherID string, course domain.Course) (string, error) {
	if integrationTeacherID == "" {
		return "", errMissingIntegrationTeacher
	}
	token, err := c.mintToken(integrationTeacherID, "teacher")
	if err != nil {
		return "", err
	}

	var out courseResponse
	err = c.do(ctx, http.MethodPost, "/create-courses", token, createCourseRequest{
		Name:        course.Title,
		Subject:     course.Subject,
		Description: course.Description,
		Duration:    course.DurationMinutes,
		Price:       int(course.Price),
		Difficulty:  string(course.Difficulty),
	}, &out)
	if err != nil {
		return "", err
	}
	return out.ID, nil
}

// createCommentRequest / commentResponse / courseRatingResponse match their
// internal/transport/http/courses/comment.go exactly.
type createCommentRequest struct {
	Rating int    `json:"rating"`
	Text   string `json:"text"`
}

type commentResponse struct {
	ID        string `json:"id"`
	CourseID  string `json:"course_id"`
	StudentID string `json:"student_id"`
	Rating    int    `json:"rating"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}

// createComment submits a review as studentID for their externalCourseID.
func (c *Client) createComment(ctx context.Context, studentID, externalCourseID string, rating int, text string) (commentResponse, error) {
	token, err := c.mintToken(studentID, "student")
	if err != nil {
		return commentResponse{}, err
	}

	var out commentResponse
	err = c.do(ctx, http.MethodPost, "/courses/"+externalCourseID+"/comments", token, createCommentRequest{
		Rating: rating,
		Text:   text,
	}, &out)
	return out, err
}

type courseRatingResponse struct {
	AverageRating float64 `json:"average_rating"`
	ReviewsCount  int     `json:"reviews_count"`
}

// getCourseRating is public on their side — no token required.
func (c *Client) getCourseRating(ctx context.Context, externalCourseID string) (courseRatingResponse, error) {
	var out courseRatingResponse
	err := c.do(ctx, http.MethodGet, "/courses/"+externalCourseID+"/rating", "", nil, &out)
	return out, err
}

// remoteCourse / courseListResponse match their GET /courses (public,
// published-only) response exactly.
type remoteCourse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Subject     string `json:"subject"`
	Description string `json:"description"`
	Duration    int    `json:"duration"`
	Price       int    `json:"price"`
	Difficulty  string `json:"difficulty"`
}

type courseListResponse struct {
	Courses []remoteCourse `json:"courses"`
}

// listCourses fetches every published course on their service. Public —
// no token required.
func (c *Client) listCourses(ctx context.Context) ([]remoteCourse, error) {
	var out courseListResponse
	err := c.do(ctx, http.MethodGet, "/courses", "", nil, &out)
	return out.Courses, err
}

// remoteLesson / lessonListResponse match their GET /courses/{id}/lessons
// (public, published-only when no token is sent) response exactly. `content`
// is intentionally not modeled: it's an opaque, per-lesson JSON blob on
// their side with no fixed schema (their own frontend types it as
// Record<string, any>) — we only need lesson files (see remoteFile below)
// for the actual media/materials.
type remoteLesson struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type lessonListResponse struct {
	Lessons []remoteLesson `json:"lessons"`
}

// listLessons fetches the published lessons of a published remote course.
// Public — no token required.
func (c *Client) listLessons(ctx context.Context, remoteCourseID string) ([]remoteLesson, error) {
	var out lessonListResponse
	err := c.do(ctx, http.MethodGet, "/courses/"+remoteCourseID+"/lessons", "", nil, &out)
	return out.Lessons, err
}

// remoteFile / fileListResponse match their GET /lessons/{lesson_id}/files response exactly.
type remoteFile struct {
	ID               string `json:"id"`
	OriginalFilename string `json:"original_filename"`
	MimeType         string `json:"mime_type"`
}

type fileListResponse struct {
	Files []remoteFile `json:"files"`
}

// listLessonFiles fetches the files attached to a lesson. Requires an
// authenticated teacher or student (their access check only inspects the
// JWT's role claim for a published lesson — see importReaderUserID).
func (c *Client) listLessonFiles(ctx context.Context, remoteLessonID string) ([]remoteFile, error) {
	token, err := c.mintToken(importReaderUserID, "student")
	if err != nil {
		return nil, err
	}
	var out fileListResponse
	err = c.do(ctx, http.MethodGet, "/lessons/"+remoteLessonID+"/files", token, nil, &out)
	return out.Files, err
}

// downloadFile streams a file's raw bytes. Same access rule as listLessonFiles.
func (c *Client) downloadFile(ctx context.Context, remoteFileID string) ([]byte, error) {
	token, err := c.mintToken(importReaderUserID, "student")
	if err != nil {
		return nil, err
	}
	return c.getRaw(ctx, "/files/"+remoteFileID+"/download", token)
}
