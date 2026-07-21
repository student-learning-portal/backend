ALTER TABLE event_log DROP CONSTRAINT IF EXISTS event_log_role_check;
ALTER TABLE event_log ADD CONSTRAINT event_log_role_check
    CHECK (role IN ('student', 'teacher', 'guest', 'system'));

DROP INDEX IF EXISTS ix_users_teacher_pending;

ALTER TABLE users
    DROP COLUMN IF EXISTS teacher_reviewed_by,
    DROP COLUMN IF EXISTS teacher_status_updated_at,
    DROP COLUMN IF EXISTS teacher_status;

-- Administrator accounts only exist because of this migration; the narrowed
-- CHECK below would reject them, so they go before the constraint comes back.
DELETE FROM users WHERE role = 'admin';

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check
    CHECK (role IN ('student', 'teacher'));
