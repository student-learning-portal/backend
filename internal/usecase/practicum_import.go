package usecase

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/student-learning-portal/backend/internal/domain"
)

// importOwnerTeacherID is the system account every imported course is owned
// by (courses.teacher_id is NOT NULL, and there is no real teacher on our
// side to attribute these to) — created once by migration
// 000012_practicum_course_import.
const importOwnerTeacherID = "00000000-0000-0000-0000-000000000001"

const (
	importDirPerm  = 0o755
	importFilePerm = 0o644
)

// PracticumImportUseCase copies the practicum team's published course
// catalog into our own catalog, so students can browse, purchase, and play
// them entirely within our system — the reverse direction of
// internal/practicum.ReviewRepository (which proxies OUR courses' ratings to
// THEIR rating engine). It backs the one-shot cmd/import-practicum-courses
// command, not a background sync: re-running it is idempotent (courses and
// lessons already imported, tracked via external_course_id/
// external_lesson_id, are skipped) but it never picks up edits made to an
// already-imported course or lesson on their side.
type PracticumImportUseCase struct {
	remote     domain.CourseImportRepository
	catalog    domain.CatalogRepository
	lessons    domain.LessonRepository
	uploadsDir string
}

func NewPracticumImportUseCase(
	remote domain.CourseImportRepository,
	catalog domain.CatalogRepository,
	lessons domain.LessonRepository,
	uploadsDir string,
) *PracticumImportUseCase {
	return &PracticumImportUseCase{remote: remote, catalog: catalog, lessons: lessons, uploadsDir: uploadsDir}
}

// ImportSummary tallies what ImportAll did, for the CLI to report.
type ImportSummary struct {
	CoursesImported int
	CoursesSkipped  int
	LessonsImported int
	LessonsSkipped  int
	Errors          []string
}

func (s *ImportSummary) addError(format string, args ...any) {
	s.Errors = append(s.Errors, fmt.Sprintf(format, args...))
}

// ImportAll fetches every published course from the practicum team's
// service and copies each one — and its lessons' media/materials — into our
// catalog, skipping anything already imported. A failure on one course or
// lesson is recorded in the returned summary and does not stop the rest of
// the import.
func (uc *PracticumImportUseCase) ImportAll(ctx context.Context) (ImportSummary, error) {
	var summary ImportSummary

	remoteCourses, err := uc.remote.ListRemoteCourses(ctx)
	if err != nil {
		return summary, fmt.Errorf("list remote courses: %w", err)
	}

	for _, rc := range remoteCourses {
		localCourseID, imported, err := uc.importCourse(ctx, rc)
		if err != nil {
			summary.addError("course %s (%q): %v", rc.ID, rc.Title, err)
			continue
		}
		if imported {
			summary.CoursesImported++
		} else {
			summary.CoursesSkipped++
		}
		uc.importLessons(ctx, rc.ID, localCourseID, &summary)
	}

	return summary, nil
}

// importCourse returns the local course id rc maps to, creating it (as
// published, owned by importOwnerTeacherID) if it hasn't been imported yet.
// imported reports whether a new row was created.
func (uc *PracticumImportUseCase) importCourse(ctx context.Context, rc domain.RemoteCourse) (string, bool, error) {
	existing, ok, err := uc.catalog.FindByExternalCourseID(ctx, rc.ID)
	if err != nil {
		return "", false, fmt.Errorf("look up existing course: %w", err)
	}
	if ok {
		return existing.ID, false, nil
	}

	difficulty := rc.Difficulty
	if !difficulty.Valid() {
		difficulty = domain.DifficultyAllLevels
	}
	created, err := uc.catalog.Create(ctx, domain.Course{
		TeacherID:       importOwnerTeacherID,
		Title:           rc.Title,
		Description:     rc.Description,
		Subject:         rc.Subject,
		Price:           rc.Price,
		Currency:        "USD",
		Difficulty:      difficulty,
		DurationMinutes: rc.DurationMinutes,
	})
	if err != nil {
		return "", false, fmt.Errorf("create course: %w", err)
	}

	// Create always inserts as a draft (PostgresCatalogRepository.Create) —
	// their course is already published, so ours should be too.
	created.Status = "published"
	if _, err := uc.catalog.Update(ctx, created); err != nil {
		return "", false, fmt.Errorf("publish imported course: %w", err)
	}
	if err := uc.catalog.SetExternalCourseID(ctx, created.ID, rc.ID); err != nil {
		return "", false, fmt.Errorf("link external course id: %w", err)
	}
	return created.ID, true, nil
}

// importLessons copies every lesson of a remote course not already imported.
func (uc *PracticumImportUseCase) importLessons(ctx context.Context, remoteCourseID, localCourseID string, summary *ImportSummary) {
	remoteLessons, err := uc.remote.ListRemoteLessons(ctx, remoteCourseID)
	if err != nil {
		summary.addError("list lessons for course %s: %v", remoteCourseID, err)
		return
	}

	for _, rl := range remoteLessons {
		_, alreadyImported, err := uc.lessons.FindByExternalID(ctx, rl.ID)
		if err != nil {
			summary.addError("lesson %s (%q): look up existing: %v", rl.ID, rl.Title, err)
			continue
		}
		if alreadyImported {
			summary.LessonsSkipped++
			continue
		}

		if err := uc.importLesson(ctx, localCourseID, rl); err != nil {
			summary.addError("lesson %s (%q): %v", rl.ID, rl.Title, err)
			continue
		}
		summary.LessonsImported++
	}
}

const materialTypePDF = "pdf"

// mediaMimeInfo / materialMimeInfo classify the practicum team's closed
// MimeType enum (video/mp4, audio/mpeg, image/jpeg, image/png,
// application/pdf — see their api/swagger.json) into our media-vs-material
// distinction, matching the extension/type conventions
// internal/delivery/http's teacher upload handlers already use.
var (
	mediaMimeInfo = map[string]struct{ ext, mediaType string }{
		"video/mp4":  {".mp4", videoType},
		"audio/mpeg": {".mp3", "audio"},
	}
	materialMimeInfo = map[string]struct{ ext, materialType string }{
		"image/jpeg":      {".jpg", "image"},
		"image/png":       {".png", "image"},
		"application/pdf": {".pdf", materialTypePDF},
	}
)

// importLesson creates the lesson, then downloads and attaches its files:
// the chosen primary file (see choosePrimaryMediaID) becomes the lesson's
// single playable media asset (SetLessonMedia only ever keeps one — see
// domain.LessonRepository), every other recognized file becomes a
// downloadable material. A lesson with no recognized files (e.g. quiz-only
// content, which our player has no equivalent for) is imported as a plain
// text lesson with no attachments — still visible and markable complete,
// just without any playable content.
func (uc *PracticumImportUseCase) importLesson(ctx context.Context, courseID string, rl domain.RemoteLesson) error {
	files, err := uc.remote.ListRemoteLessonFiles(ctx, rl.ID)
	if err != nil {
		return fmt.Errorf("list files: %w", err)
	}

	lessonType := "text"
	for _, f := range files {
		if _, ok := mediaMimeInfo[f.MimeType]; ok {
			lessonType = videoType
			break
		}
	}

	lesson, err := uc.lessons.CreateLesson(ctx, courseID, rl.Title, lessonType)
	if err != nil {
		return fmt.Errorf("create lesson: %w", err)
	}
	if err := uc.lessons.SetExternalLessonID(ctx, lesson.ID, rl.ID); err != nil {
		return fmt.Errorf("link external lesson id: %w", err)
	}

	primaryMediaID := uc.choosePrimaryMediaID(ctx, rl.ID, files)

	haveMedia := false
	for _, f := range files {
		if info, ok := mediaMimeInfo[f.MimeType]; ok && !haveMedia && f.ID == primaryMediaID {
			if err := uc.copyMedia(ctx, lesson.ID, f, info.ext, info.mediaType); err != nil {
				return fmt.Errorf("copy media %s: %w", f.ID, err)
			}
			haveMedia = true
			continue
		}
		if info, ok := materialMimeInfo[f.MimeType]; ok {
			if err := uc.copyMaterial(ctx, lesson.ID, f, info.ext, info.materialType); err != nil {
				return fmt.Errorf("copy material %s: %w", f.ID, err)
			}
		}
	}
	return nil
}

// choosePrimaryMediaID picks which file becomes the lesson's single playable
// media asset: the file their own course builder recorded as
// content.mediaFileId when the lesson was authored (a de-facto convention —
// see internal/practicum's lessonContent), falling back to the first
// recognized video/audio file when that hint is missing, doesn't resolve to
// one of files, or isn't itself a playable type. The remote lookup is
// best-effort: an error here just falls back to the heuristic instead of
// failing the whole lesson import over a refinement.
func (uc *PracticumImportUseCase) choosePrimaryMediaID(ctx context.Context, remoteLessonID string, files []domain.RemoteFile) string {
	if preferred, ok, err := uc.remote.GetRemotePrimaryMediaFileID(ctx, remoteLessonID); err == nil && ok {
		for _, f := range files {
			if f.ID != preferred {
				continue
			}
			if _, isMedia := mediaMimeInfo[f.MimeType]; isMedia {
				return preferred
			}
			break
		}
	}
	for _, f := range files {
		if _, ok := mediaMimeInfo[f.MimeType]; ok {
			return f.ID
		}
	}
	return ""
}

// saveFile downloads-then-writes a file under the same
// uploadsDir/lessons/{lessonID}/{uuid}{ext} layout (and /uploads/ URL shape)
// the teacher-authoring upload endpoints use, so it's served by the same
// static file route with no extra wiring.
func (uc *PracticumImportUseCase) saveFile(lessonID string, data []byte, ext string) (string, error) {
	destDir := filepath.Join(uc.uploadsDir, "lessons", lessonID)
	if err := os.MkdirAll(destDir, importDirPerm); err != nil {
		return "", fmt.Errorf("prepare directory: %w", err)
	}
	filename := uuid.NewString() + ext
	destPath := filepath.Join(destDir, filename)
	if err := os.WriteFile(destPath, data, importFilePerm); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return fmt.Sprintf("/uploads/lessons/%s/%s", lessonID, filename), nil
}

func (uc *PracticumImportUseCase) copyMedia(ctx context.Context, lessonID string, f domain.RemoteFile, ext, mediaType string) error {
	data, err := uc.remote.DownloadRemoteFile(ctx, f.ID)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	url, err := uc.saveFile(lessonID, data, ext)
	if err != nil {
		return err
	}
	if _, err := uc.lessons.SetLessonMedia(ctx, lessonID, url, 0, mediaType); err != nil {
		return fmt.Errorf("set lesson media: %w", err)
	}
	return nil
}

func (uc *PracticumImportUseCase) copyMaterial(ctx context.Context, lessonID string, f domain.RemoteFile, ext, materialType string) error {
	data, err := uc.remote.DownloadRemoteFile(ctx, f.ID)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	url, err := uc.saveFile(lessonID, data, ext)
	if err != nil {
		return err
	}
	title := f.OriginalFilename
	if title == "" {
		title = "material" + ext
	}
	if _, err := uc.lessons.AddMaterial(ctx, lessonID, title, url, materialType); err != nil {
		return fmt.Errorf("add material: %w", err)
	}
	return nil
}
