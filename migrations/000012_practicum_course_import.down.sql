DELETE FROM users WHERE id = '00000000-0000-0000-0000-000000000001';

DROP INDEX IF EXISTS ux_courses_external_id;
DROP INDEX IF EXISTS ux_lessons_external_id;

ALTER TABLE lessons
    DROP COLUMN external_lesson_id;
