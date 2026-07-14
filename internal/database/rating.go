package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/student-learning-portal/backend/internal/domain"
)

type PostgresCourseRatingRepository struct {
	db *sql.DB
}

func NewPostgresCourseRatingRepository(db *sql.DB) domain.CourseRatingRepository {
	return &PostgresCourseRatingRepository{db: db}
}

// Upsert records studentID's score for courseID, overwriting any prior score
// from the same student for the same course (see course_ratings' unique
// (student_id, course_id) constraint).
func (r *PostgresCourseRatingRepository) Upsert(ctx context.Context, studentID, courseID string, score int) (domain.CourseRating, error) {
	var out domain.CourseRating
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO course_ratings (student_id, course_id, score)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (student_id, course_id) DO UPDATE SET score = $3, updated_at = now()
		 RETURNING id, student_id, course_id, score, created_at, updated_at`,
		studentID, courseID, score,
	).Scan(&out.ID, &out.StudentID, &out.CourseID, &out.Score, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		return domain.CourseRating{}, fmt.Errorf("upsert course rating: %w", err)
	}
	return out, nil
}

// Summary aggregates every rating recorded for courseID.
func (r *PostgresCourseRatingRepository) Summary(ctx context.Context, courseID string) (domain.RatingSummary, error) {
	var summary domain.RatingSummary
	var avg sql.NullFloat64
	err := r.db.QueryRowContext(ctx,
		`SELECT AVG(score), COUNT(*) FROM course_ratings WHERE course_id = $1`, courseID,
	).Scan(&avg, &summary.RatingsCount)
	if err != nil {
		return domain.RatingSummary{}, fmt.Errorf("summarize course ratings: %w", err)
	}
	summary.AverageScore = avg.Float64
	return summary, nil
}

// GetByStudent returns studentID's own rating for courseID.
func (r *PostgresCourseRatingRepository) GetByStudent(ctx context.Context, studentID, courseID string) (domain.CourseRating, error) {
	var out domain.CourseRating
	err := r.db.QueryRowContext(ctx,
		`SELECT id, student_id, course_id, score, created_at, updated_at
		 FROM course_ratings WHERE student_id = $1 AND course_id = $2`,
		studentID, courseID,
	).Scan(&out.ID, &out.StudentID, &out.CourseID, &out.Score, &out.CreatedAt, &out.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.CourseRating{}, domain.ErrRatingNotFound
	}
	if err != nil {
		return domain.CourseRating{}, fmt.Errorf("get course rating by student: %w", err)
	}
	return out, nil
}

type PostgresTeacherRatingRepository struct {
	db *sql.DB
}

func NewPostgresTeacherRatingRepository(db *sql.DB) domain.TeacherRatingRepository {
	return &PostgresTeacherRatingRepository{db: db}
}

// Upsert records studentID's score for teacherID, overwriting any prior score
// from the same student for the same teacher (see teacher_ratings' unique
// (student_id, teacher_id) constraint).
func (r *PostgresTeacherRatingRepository) Upsert(
	ctx context.Context, studentID, teacherID string, score int,
) (domain.TeacherRating, error) {
	var out domain.TeacherRating
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO teacher_ratings (student_id, teacher_id, score)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (student_id, teacher_id) DO UPDATE SET score = $3, updated_at = now()
		 RETURNING id, student_id, teacher_id, score, created_at, updated_at`,
		studentID, teacherID, score,
	).Scan(&out.ID, &out.StudentID, &out.TeacherID, &out.Score, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		return domain.TeacherRating{}, fmt.Errorf("upsert teacher rating: %w", err)
	}
	return out, nil
}

// Summary aggregates every rating recorded for teacherID.
func (r *PostgresTeacherRatingRepository) Summary(ctx context.Context, teacherID string) (domain.RatingSummary, error) {
	var summary domain.RatingSummary
	var avg sql.NullFloat64
	err := r.db.QueryRowContext(ctx,
		`SELECT AVG(score), COUNT(*) FROM teacher_ratings WHERE teacher_id = $1`, teacherID,
	).Scan(&avg, &summary.RatingsCount)
	if err != nil {
		return domain.RatingSummary{}, fmt.Errorf("summarize teacher ratings: %w", err)
	}
	summary.AverageScore = avg.Float64
	return summary, nil
}

// GetByStudent returns studentID's own rating for teacherID.
func (r *PostgresTeacherRatingRepository) GetByStudent(ctx context.Context, studentID, teacherID string) (domain.TeacherRating, error) {
	var out domain.TeacherRating
	err := r.db.QueryRowContext(ctx,
		`SELECT id, student_id, teacher_id, score, created_at, updated_at
		 FROM teacher_ratings WHERE student_id = $1 AND teacher_id = $2`,
		studentID, teacherID,
	).Scan(&out.ID, &out.StudentID, &out.TeacherID, &out.Score, &out.CreatedAt, &out.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.TeacherRating{}, domain.ErrRatingNotFound
	}
	if err != nil {
		return domain.TeacherRating{}, fmt.Errorf("get teacher rating by student: %w", err)
	}
	return out, nil
}
