-- ============================================================
-- Administrator role + teacher approval queue
--
-- A teacher account is no longer usable the moment it registers: it lands in
-- 'pending' and an administrator has to confirm the role before the teacher
-- endpoints open up. The column is the state; the gate itself lives in
-- delivery/http.RequireApprovedTeacher, not in a DB constraint -- a pending
-- teacher is a perfectly valid row, it just can't author content yet.
--
-- teacher_status is NULL for every non-teacher row, so the partial index below
-- stays tiny and "which teachers are awaiting review" is a single index scan.
-- Teachers that already existed are grandfathered as 'approved': they were
-- created back when registration granted the role outright, and retroactively
-- locking them out of their own courses would be a regression, not a fix.
-- ============================================================
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check
    CHECK (role IN ('student', 'teacher', 'admin'));

ALTER TABLE users
    ADD COLUMN teacher_status            TEXT
        CHECK (teacher_status IS NULL OR teacher_status IN ('pending', 'approved', 'rejected')),
    ADD COLUMN teacher_status_updated_at TIMESTAMPTZ,
    ADD COLUMN teacher_reviewed_by       UUID REFERENCES users (id);

UPDATE users
   SET teacher_status = 'approved',
       teacher_status_updated_at = now()
 WHERE role = 'teacher';

CREATE INDEX ix_users_teacher_pending ON users (created_at)
    WHERE teacher_status = 'pending';

-- The analytics envelope's role enum has to learn 'admin' as well: otherwise
-- every event an administrator triggers -- starting with their own auth.login
-- -- trips this CHECK and is silently dropped by the best-effort event sink.
ALTER TABLE event_log DROP CONSTRAINT IF EXISTS event_log_role_check;
ALTER TABLE event_log ADD CONSTRAINT event_log_role_check
    CHECK (role IN ('student', 'teacher', 'admin', 'guest', 'system'));
