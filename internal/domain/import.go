package domain

import "context"

// RemoteCourse is a published course as returned by the practicum team's
// public course list (see internal/practicum), used by the one-shot import
// command (internal/usecase.PracticumImportUseCase) to copy their catalog
// into ours.
type RemoteCourse struct {
	ID              string
	Title           string
	Subject         string
	Description     string
	Price           float64
	Difficulty      DifficultyLevel
	DurationMinutes int
}

// RemoteLesson is a published lesson as returned by the practicum team's
// public lesson list for a course.
type RemoteLesson struct {
	ID    string
	Title string
}

// RemoteFile is a file attached to a practicum-team lesson (media or
// material), as returned by their lesson-files endpoint.
type RemoteFile struct {
	ID               string
	OriginalFilename string
	MimeType         string
}

// CourseImportRepository reads the practicum team's course/lesson catalog
// and lesson files so PracticumImportUseCase can copy their published
// courses into our own catalog. See internal/practicum for the
// implementation — every call proxies to their already-running API; nothing
// here reimplements their business logic.
type CourseImportRepository interface {
	// ListRemoteCourses returns every published course on their service
	// (their GET /courses is public, unfiltered).
	ListRemoteCourses(ctx context.Context) ([]RemoteCourse, error)
	// ListRemoteLessons returns the published lessons of a published remote
	// course (their GET /courses/{id}/lessons, public).
	ListRemoteLessons(ctx context.Context, remoteCourseID string) ([]RemoteLesson, error)
	// ListRemoteLessonFiles returns the files attached to a remote lesson.
	// Their endpoint requires an authenticated teacher or student — see
	// internal/practicum's Client for how that's satisfied.
	ListRemoteLessonFiles(ctx context.Context, remoteLessonID string) ([]RemoteFile, error)
	// DownloadRemoteFile fetches a file's raw bytes.
	DownloadRemoteFile(ctx context.Context, remoteFileID string) ([]byte, error)
}
