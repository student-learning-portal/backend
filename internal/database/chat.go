package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/student-learning-portal/backend/internal/domain"
)

type PostgresChatRepository struct {
	db *sql.DB
}

func NewPostgresChatRepository(db *sql.DB) domain.ChatRepository {
	return &PostgresChatRepository{db: db}
}

func (r *PostgresChatRepository) AddMessage(ctx context.Context, m domain.Message) (domain.Message, error) {
	lessonID := sql.NullString{}
	if m.LessonID != nil && *m.LessonID != "" {
		lessonID = sql.NullString{String: *m.LessonID, Valid: true}
	}
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO messages (course_id, student_id, lesson_id, sender_role, sender_id, body)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, created_at`,
		m.CourseID, m.StudentID, lessonID, m.SenderRole, m.SenderID, m.Body,
	).Scan(&m.ID, &m.CreatedAt)
	if err != nil {
		return domain.Message{}, fmt.Errorf("add message: %w", err)
	}
	return m, nil
}

func (r *PostgresChatRepository) ListThread(ctx context.Context, courseID, studentID string) ([]domain.Message, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, course_id, student_id, lesson_id, sender_role, sender_id, body, created_at
		 FROM messages
		 WHERE course_id = $1 AND student_id = $2
		 ORDER BY created_at`,
		courseID, studentID,
	)
	if err != nil {
		return nil, fmt.Errorf("list thread: %w", err)
	}
	defer rows.Close()

	messages := []domain.Message{}
	for rows.Next() {
		var (
			m        domain.Message
			lessonID sql.NullString
		)
		if err := rows.Scan(&m.ID, &m.CourseID, &m.StudentID, &lessonID,
			&m.SenderRole, &m.SenderID, &m.Body, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		if lessonID.Valid {
			m.LessonID = &lessonID.String
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate thread: %w", err)
	}
	return messages, nil
}

func (r *PostgresChatRepository) ListThreads(ctx context.Context, courseID string) ([]domain.ThreadSummary, error) {
	// One row per student: the latest message (DISTINCT ON), the total count
	// (window), and the student's name; ordered most-recently-active first.
	rows, err := r.db.QueryContext(ctx,
		`SELECT t.student_id, t.student_name, t.message_count, t.last_message, t.last_at
		 FROM (
		     SELECT DISTINCT ON (m.student_id)
		            m.student_id,
		            COALESCE(u.full_name, '')                       AS student_name,
		            count(*) OVER (PARTITION BY m.student_id)        AS message_count,
		            m.body                                          AS last_message,
		            m.created_at                                    AS last_at
		     FROM messages m
		     LEFT JOIN users u ON u.id = m.student_id
		     WHERE m.course_id = $1
		     ORDER BY m.student_id, m.created_at DESC
		 ) t
		 ORDER BY t.last_at DESC`,
		courseID,
	)
	if err != nil {
		return nil, fmt.Errorf("list threads: %w", err)
	}
	defer rows.Close()

	threads := []domain.ThreadSummary{}
	for rows.Next() {
		s := domain.ThreadSummary{CourseID: courseID}
		if err := rows.Scan(&s.StudentID, &s.StudentName, &s.MessageCount, &s.LastMessage, &s.LastAt); err != nil {
			return nil, fmt.Errorf("scan thread summary: %w", err)
		}
		threads = append(threads, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate threads: %w", err)
	}
	return threads, nil
}
