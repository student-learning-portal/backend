-- ============================================================
-- Prevent more than one active access grant per (actor, course).
-- Closes the race where two concurrent checkouts for the same
-- course both pass the balance deduction before either commits
-- its grant, resulting in a double charge.
-- ============================================================
CREATE UNIQUE INDEX ux_access_grant_one_active
    ON access_grant (actor_id, course_id)
    WHERE revoked_at IS NULL;
