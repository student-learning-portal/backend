-- ============================================================
-- Reverse course import (see internal/practicum + cmd/import-practicum-courses).
--
-- This is the opposite direction from migration 000010's external_course_id:
-- there, external_course_id is *our* course mirrored into their system.
-- Here, an imported course is *their* course copied into ours, so its
-- external_course_id holds their original course id instead. Either way the
-- column means "the matching course id on the other team's service", so it
-- is safe to reuse for both directions — a lookup by external_course_id is
-- always scoped to one direction by which side initiated it.
--
-- external_lesson_id is the equivalent link for lessons, used to make the
-- one-shot import command idempotent (re-running it skips lessons already
-- imported instead of duplicating them).
-- ============================================================
ALTER TABLE lessons
    ADD COLUMN external_lesson_id UUID;

CREATE UNIQUE INDEX ux_lessons_external_id ON lessons (external_lesson_id)
    WHERE external_lesson_id IS NOT NULL;

CREATE UNIQUE INDEX ux_courses_external_id ON courses (external_course_id)
    WHERE external_course_id IS NOT NULL;

-- System account that owns every course imported from the practicum team's
-- service — a real users row is required by courses.teacher_id's FK, but
-- there is no actual teacher on our side to attribute these to.
INSERT INTO users (id, email, password_hash, full_name, role)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'practicum-import@system.local',
    -- Not a usable password hash (system account, never logs in) — just a
    -- fixed placeholder so the NOT NULL constraint is satisfied.
    'system-account-no-login',
    'Практикум (импортированные курсы)',
    'teacher'
)
ON CONFLICT (id) DO NOTHING;
