-- ============================================================
-- Course catalog extras.
--
-- difficulty / duration_minutes: plain course metadata, matching feature
-- parity with the practicum team's course API (see api/openapi.yaml Course
-- schema) — entirely local, no dependency on their service.
--
-- external_course_id: NOT a copy of their business logic. It is the
-- practicum-team course ID our course gets mirrored to (lazily, on first
-- rating/comment request — see internal/practicum), so we can proxy rating
-- and review requests to their already-running rating/comment feature
-- instead of reimplementing it. NULL until first mirrored.
-- ============================================================
ALTER TABLE courses
    ADD COLUMN difficulty TEXT NOT NULL DEFAULT 'all_levels'
        CHECK (difficulty IN ('beginner', 'intermediate', 'advanced', 'all_levels')),
    ADD COLUMN duration_minutes INTEGER NOT NULL DEFAULT 0
        CHECK (duration_minutes >= 0),
    ADD COLUMN external_course_id UUID;

CREATE INDEX ix_courses_difficulty ON courses (difficulty);
