-- ============================================================
-- Users
-- ============================================================
CREATE TABLE users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    full_name     TEXT        NOT NULL,
    role          TEXT        NOT NULL CHECK (role IN ('student', 'teacher')),
    anonymous_id  UUID,           -- links pre-auth anonymous_id from event_log on signup
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ix_users_email ON users (email);

-- ============================================================
-- Courses
-- ============================================================
CREATE TABLE courses (
    id          UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    teacher_id  UUID          NOT NULL REFERENCES users (id),
    title       TEXT          NOT NULL,
    description TEXT,
    price       NUMERIC(12,2) NOT NULL DEFAULT 0,
    currency    TEXT          NOT NULL DEFAULT 'USD',
    status      TEXT          NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'published', 'archived')),
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX ix_courses_teacher ON courses (teacher_id);
CREATE INDEX ix_courses_status  ON courses (status);

-- ============================================================
-- Lessons (ordered within a course)
-- ============================================================
CREATE TABLE lessons (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id   UUID        NOT NULL REFERENCES courses (id) ON DELETE CASCADE,
    title       TEXT        NOT NULL,
    lesson_type TEXT        NOT NULL CHECK (lesson_type IN ('video', 'text', 'quiz', 'mixed')),
    position    INTEGER     NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (course_id, position)
);

CREATE INDEX ix_lessons_course ON lessons (course_id, position);

-- ============================================================
-- Media (video/audio files attached to a lesson)
-- ============================================================
CREATE TABLE media (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    lesson_id   UUID        NOT NULL REFERENCES lessons (id) ON DELETE CASCADE,
    url         TEXT        NOT NULL,
    duration_ms INTEGER,
    media_type  TEXT        NOT NULL DEFAULT 'video' CHECK (media_type IN ('video', 'audio')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ix_media_lesson ON media (lesson_id);

-- ============================================================
-- Materials (attachments: pdf, zip, link, etc.)
-- ============================================================
CREATE TABLE materials (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    lesson_id     UUID        NOT NULL REFERENCES lessons (id) ON DELETE CASCADE,
    title         TEXT        NOT NULL,
    url           TEXT        NOT NULL,
    material_type TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ix_materials_lesson ON materials (lesson_id);
