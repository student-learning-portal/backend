package practicum

import (
	"context"
	"fmt"

	"github.com/student-learning-portal/backend/internal/domain"
)

// ImportRepository implements domain.CourseImportRepository by reading the
// practicum-team service's public course/lesson catalog (and their
// authenticated lesson files) — the reverse direction of ReviewRepository:
// that proxies OUR courses' ratings to THEIR rating engine; this copies
// THEIR courses into OUR catalog (see internal/usecase's
// PracticumImportUseCase, the one-shot import command that drives it).
type ImportRepository struct {
	client *Client
}

// NewImportRepository wires the integration. No integration teacher/student
// account is needed: course/lesson listing is public on their side, and
// their file-access check only inspects the JWT role claim (see
// importReaderUserID).
func NewImportRepository(client *Client) *ImportRepository {
	return &ImportRepository{client: client}
}

func (r *ImportRepository) ListRemoteCourses(ctx context.Context) ([]domain.RemoteCourse, error) {
	courses, err := r.client.listCourses(ctx)
	if err != nil {
		return nil, fmt.Errorf("list remote courses: %w", err)
	}
	out := make([]domain.RemoteCourse, 0, len(courses))
	for _, c := range courses {
		out = append(out, domain.RemoteCourse{
			ID:              c.ID,
			Title:           c.Name,
			Subject:         c.Subject,
			Description:     c.Description,
			Price:           float64(c.Price),
			Difficulty:      domain.DifficultyLevel(c.Difficulty),
			DurationMinutes: c.Duration,
		})
	}
	return out, nil
}

func (r *ImportRepository) ListRemoteLessons(ctx context.Context, remoteCourseID string) ([]domain.RemoteLesson, error) {
	lessons, err := r.client.listLessons(ctx, remoteCourseID)
	if err != nil {
		return nil, fmt.Errorf("list remote lessons: %w", err)
	}
	out := make([]domain.RemoteLesson, 0, len(lessons))
	for _, l := range lessons {
		out = append(out, domain.RemoteLesson{ID: l.ID, Title: l.Name})
	}
	return out, nil
}

func (r *ImportRepository) ListRemoteLessonFiles(ctx context.Context, remoteLessonID string) ([]domain.RemoteFile, error) {
	files, err := r.client.listLessonFiles(ctx, remoteLessonID)
	if err != nil {
		return nil, fmt.Errorf("list remote lesson files: %w", err)
	}
	out := make([]domain.RemoteFile, 0, len(files))
	for _, f := range files {
		out = append(out, domain.RemoteFile{
			ID:               f.ID,
			OriginalFilename: f.OriginalFilename,
			MimeType:         f.MimeType,
		})
	}
	return out, nil
}

func (r *ImportRepository) DownloadRemoteFile(ctx context.Context, remoteFileID string) ([]byte, error) {
	data, err := r.client.downloadFile(ctx, remoteFileID)
	if err != nil {
		return nil, fmt.Errorf("download remote file: %w", err)
	}
	return data, nil
}

func (r *ImportRepository) GetRemotePrimaryMediaFileID(ctx context.Context, remoteLessonID string) (string, bool, error) {
	id, ok, err := r.client.getLessonPrimaryMediaFileID(ctx, remoteLessonID)
	if err != nil {
		return "", false, fmt.Errorf("get lesson content: %w", err)
	}
	return id, ok, nil
}
