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
