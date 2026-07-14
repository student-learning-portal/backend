ALTER TABLE courses
    DROP COLUMN IF EXISTS difficulty,
    DROP COLUMN IF EXISTS duration_minutes,
    DROP COLUMN IF EXISTS external_course_id;
