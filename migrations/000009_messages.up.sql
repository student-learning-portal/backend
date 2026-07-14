-- ============================================================
-- Messages (student <-> teacher chat, scoped to a course)
--
-- A conversation ("thread") is keyed by (course_id, student_id): each enrolled
-- student has their own thread with the course's teacher. A message may
-- optionally reference the lesson it is about (customer request: ask the teacher
-- about a specific assignment).
-- ============================================================
CREATE TABLE messages (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id   UUID        NOT NULL REFERENCES courses (id) ON DELETE CASCADE,
    student_id  UUID        NOT NULL REFERENCES users (id),
    lesson_id   UUID        REFERENCES lessons (id) ON DELETE SET NULL,
    sender_role TEXT        NOT NULL CHECK (sender_role IN ('student', 'teacher')),
    sender_id   UUID        NOT NULL REFERENCES users (id),
    body        TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ix_messages_thread ON messages (course_id, student_id, created_at);
