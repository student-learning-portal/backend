-- ============================================================
-- Prevent more than one active access grant per (actor, course).
-- Closes the race where two concurrent checkouts for the same
-- course both pass the balance deduction before either commits
-- its grant, resulting in a double charge.
--
-- Existing data can already contain duplicate active grants from
-- before this fix (i.e. exactly the double-charge race this
-- migration closes), which would make the index creation below
-- fail. Revoke every duplicate but the earliest-granted row per
-- (actor_id, course_id) first so the index can be created.
-- ============================================================
WITH ranked AS (
    SELECT grant_id,
           row_number() OVER (
               PARTITION BY actor_id, course_id
               ORDER BY granted_at ASC, grant_id ASC
           ) AS rn
    FROM access_grant
    WHERE revoked_at IS NULL
)
UPDATE access_grant
SET revoked_at = now(),
    revoke_reason = 'manual'
WHERE grant_id IN (SELECT grant_id FROM ranked WHERE rn > 1);

CREATE UNIQUE INDEX ux_access_grant_one_active
    ON access_grant (actor_id, course_id)
    WHERE revoked_at IS NULL;
