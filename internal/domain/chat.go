package domain

import (
	"context"
	"errors"
	"time"
)

// ErrEmptyMessage is returned when a chat message has no body.
var ErrEmptyMessage = errors.New("message body is required")

// Message is one chat message within a course conversation. A conversation
// ("thread") is identified by (CourseID, StudentID): the student and the
// course's teacher are its two participants. LessonID optionally ties the
// message to a specific lesson.
type Message struct {
	ID         string    `json:"id"`
	CourseID   string    `json:"course_id"`
	StudentID  string    `json:"student_id"`
	LessonID   *string   `json:"lesson_id,omitempty"`
	SenderRole Role      `json:"sender_role"`
	SenderID   string    `json:"sender_id"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

// ThreadSummary is one student's conversation within a course, for the teacher's
// thread list: who it's with, how many messages, and the latest one.
type ThreadSummary struct {
	CourseID     string    `json:"course_id"`
	StudentID    string    `json:"student_id"`
	StudentName  string    `json:"student_name"`
	MessageCount int       `json:"message_count"`
	LastMessage  string    `json:"last_message"`
	LastAt       time.Time `json:"last_at"`
}

// ChatRepository persists and retrieves course chat messages.
type ChatRepository interface {
	// AddMessage inserts a message and returns it with its generated id and
	// created_at timestamp.
	AddMessage(ctx context.Context, m Message) (Message, error)
	// ListThread returns every message in the (course, student) conversation,
	// oldest first.
	ListThread(ctx context.Context, courseID, studentID string) ([]Message, error)
	// ListThreads summarises every student conversation in a course, most
	// recently active first (for the teacher's inbox).
	ListThreads(ctx context.Context, courseID string) ([]ThreadSummary, error)
}
