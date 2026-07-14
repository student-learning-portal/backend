-- ============================================================
-- Ratings (local, 1-10 scale) for courses and teachers.
--
-- Separate from the practicum-team proxied course review/comment system
-- (courses.external_course_id, internal/practicum): that integration only
-- covers courses, so teacher ratings need a local table regardless. Kept
-- alongside the practicum proxy rather than replacing it.
--
-- Two dedicated tables (rather than one polymorphic table) since courses and
-- teachers are rated independently and rater_id always references the
-- students table's role -- gating that only students may rate is enforced in
-- usecase.RatingUseCase, not by a DB constraint (role is mutable and lives on
-- the shared users row).
--
-- One row per (student, target): resubmitting a rating overwrites the score
-- (see Upsert) instead of accumulating duplicates, and the average/count
-- shown to users is always computed from these rows, never stored directly.
-- ============================================================
CREATE TABLE course_ratings (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    course_id  UUID        NOT NULL REFERENCES courses (id) ON DELETE CASCADE,
    score      SMALLINT    NOT NULL CHECK (score BETWEEN 1 AND 10),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (student_id, course_id)
);

CREATE INDEX ix_course_ratings_course ON course_ratings (course_id);

CREATE TABLE teacher_ratings (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    teacher_id UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    score      SMALLINT    NOT NULL CHECK (score BETWEEN 1 AND 10),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (student_id, teacher_id)
);

CREATE INDEX ix_teacher_ratings_teacher ON teacher_ratings (teacher_id);
