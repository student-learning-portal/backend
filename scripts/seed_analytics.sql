-- ============================================================
-- Analytics mock-data generator (derived-layer demo)
--
-- Produces a realistic behavioral event_log so the derived analytics rollup and
-- the teacher at-risk dashboard have something to chew on. Everything is written
-- through event_log (the source of truth) + enrollment rows; the rollup is then
-- built by the loader:
--   psql -f internal/database/sql/refresh_student_course_rollup.sql
--   (or: go run ./cmd/analytics-loader)
--
-- Self-contained, idempotent, ADDITIVE: it does NOT TRUNCATE. It owns a fixed
-- UUID namespace (prefixes b0..b5) and deletes only its own rows before
-- re-inserting, so it coexists with scripts/seed.sql.
--
-- Run:
--   docker exec -i <postgres-container> psql -U admin -d db < scripts/seed_analytics.sql
--
-- Login for the dashboard:  analytics.teacher@example.com / password123
-- Target course_id:         b1000000-0000-4000-8000-000000000001
-- ============================================================

\set n_students 200

BEGIN;

-- bcrypt hashing for a real, loginable teacher password (no external tooling).
-- pgcrypto's crypt('..', gen_salt('bf')) emits a standard $2a$ bcrypt hash that
-- the Go bcrypt verifier accepts.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ------------------------------------------------------------
-- Idempotent cleanup (our namespace only; FK-safe order)
-- ------------------------------------------------------------
DELETE FROM event_log     WHERE event_id::text LIKE 'b5000000-%';
DELETE FROM access_grant  WHERE grant_id::text LIKE 'b4000000-%';
DELETE FROM payment       WHERE txn_id::text   LIKE 'b3000000-%';
DELETE FROM progress_state WHERE actor_id      LIKE 'b2000000-%';
DELETE FROM courses       WHERE id::text       LIKE 'b1000000-%';  -- cascades lessons
DELETE FROM users         WHERE id::text       LIKE 'b0000000-%' OR id::text LIKE 'b2000000-%';

-- ------------------------------------------------------------
-- Teacher (loginable) + two published courses
-- ------------------------------------------------------------
INSERT INTO users (id, email, password_hash, full_name, role) VALUES
    ('b0000000-0000-4000-8000-000000000001', 'analytics.teacher@example.com',
     crypt('password123', gen_salt('bf', 10)), 'Ada Analytics', 'teacher');

INSERT INTO courses (id, teacher_id, title, description, subject, price, currency, status) VALUES
    ('b1000000-0000-4000-8000-000000000001', 'b0000000-0000-4000-8000-000000000001',
     'Analytics 101', 'Seeded course with a cohort of mock learners.', 'Data Science', 49.99, 'USD', 'published'),
    ('b1000000-0000-4000-8000-000000000002', 'b0000000-0000-4000-8000-000000000001',
     'Analytics 201', 'Second seeded course (no cohort yet).', 'Data Science', 69.99, 'USD', 'published');

-- 6 lessons for the target course (Analytics 101)
INSERT INTO lessons (id, course_id, title, lesson_type, position)
SELECT ('b1a00000-0000-4000-8000-' || lpad(p::text, 12, '0'))::uuid,
       'b1000000-0000-4000-8000-000000000001', 'Lesson ' || p, 'video', p
FROM generate_series(1, 6) AS p;

-- ------------------------------------------------------------
-- Students + enrollment into Analytics 101
-- Student passwords are placeholders (login out of scope here).
-- ------------------------------------------------------------
INSERT INTO users (id, email, password_hash, full_name, role)
SELECT ('b2000000-0000-4000-8000-' || lpad(i::text, 12, '0'))::uuid,
       'seed.student' || i || '@example.com',
       '$2a$10$placeholderhashplaceholderhashplaceholderhashxx',
       'Seed Student ' || i, 'student'
FROM generate_series(1, :n_students) AS i;

INSERT INTO payment (txn_id, cart_id, actor_id, course_id, amount, currency, status, sandbox)
SELECT ('b3000000-0000-4000-8000-' || lpad(i::text, 12, '0'))::uuid,
       ('b3a00000-0000-4000-8000-' || lpad(i::text, 12, '0'))::uuid,
       (('b2000000-0000-4000-8000-' || lpad(i::text, 12, '0'))::uuid)::text,
       'b1000000-0000-4000-8000-000000000001',
       49.99, 'USD', 'succeeded', true
FROM generate_series(1, :n_students) AS i;

INSERT INTO access_grant (grant_id, actor_id, course_id, txn_id, granted_at)
SELECT ('b4000000-0000-4000-8000-' || lpad(i::text, 12, '0'))::uuid,
       (('b2000000-0000-4000-8000-' || lpad(i::text, 12, '0'))::uuid)::text,
       'b1000000-0000-4000-8000-000000000001',
       ('b3000000-0000-4000-8000-' || lpad(i::text, 12, '0'))::uuid,
       now() - (interval '1 day' * (25 + (i % 10)))
FROM generate_series(1, :n_students) AS i;

-- ------------------------------------------------------------
-- Behavioral events (player.*) driving progress + recency.
-- Deterministic profile bucket per student (Knuth multiplicative hash):
--   < 25  low progress (recent)        -> AT_RISK (progress)
--   25-39 high progress but stale      -> AT_RISK (inactivity)
--   40-94 healthy + recent             -> ON_TRACK
--   >= 95 enrolled, no activity        -> AT_RISK (never active)
-- Expected at-risk share ~45%.
-- ------------------------------------------------------------
WITH params AS (
    SELECT i,
           (('b2000000-0000-4000-8000-' || lpad(i::text, 12, '0'))::uuid)::text AS actor_id,
           ((i::bigint * 2654435761) % 100)::int AS bucket
    FROM generate_series(1, :n_students) AS i
),
profile AS (
    SELECT i, actor_id, bucket,
        CASE
            WHEN bucket < 25 THEN 1   -- low: only 1 lesson touched
            WHEN bucket < 40 THEN 5   -- inactive: most lessons done
            WHEN bucket < 95 THEN 5   -- on track: most lessons done
            ELSE 0                    -- none
        END AS active_lessons,
        CASE
            WHEN bucket < 25 THEN (bucket % 5)            -- recent 0-4d
            WHEN bucket < 40 THEN 14 + (bucket % 27)      -- stale 14-40d
            WHEN bucket < 95 THEN (bucket % 6)            -- recent 0-5d
            ELSE 0
        END AS days_ago,
        CASE
            WHEN bucket < 25 THEN 10 + (bucket % 15)      -- 10-24%
            WHEN bucket < 40 THEN 80 + (bucket % 21)      -- 80-100%
            WHEN bucket < 95 THEN 70 + (bucket % 31)      -- 70-100%
            ELSE 0
        END AS pct
    FROM params
),
active AS (
    SELECT pr.i, pr.actor_id, pr.pct, pr.days_ago, p
    FROM profile pr
    CROSS JOIN generate_series(1, 6) AS p
    WHERE p <= pr.active_lessons
)
INSERT INTO event_log
    (event_id, event_name, schema_version, event_ts, actor_id, role, service, env, course_id, lesson_id, payload)
SELECT ('b5000000-0000-4000-8000-' || lpad((i * 1000 + p * 10 + 1)::text, 12, '0'))::uuid,
       'player.lesson_open', '1.0.0',
       now() - (interval '1 day' * days_ago) - (interval '1 hour' * (7 - p)),
       actor_id, 'student', 'player', 'dev',
       'b1000000-0000-4000-8000-000000000001',
       (('b1a00000-0000-4000-8000-' || lpad(p::text, 12, '0'))::uuid)::text,
       '{}'::jsonb
FROM active
UNION ALL
SELECT ('b5000000-0000-4000-8000-' || lpad((i * 1000 + p * 10 + 2)::text, 12, '0'))::uuid,
       'player.progress_save', '1.0.0',
       now() - (interval '1 day' * days_ago) - (interval '1 hour' * (6 - p)),
       actor_id, 'student', 'player', 'dev',
       'b1000000-0000-4000-8000-000000000001',
       (('b1a00000-0000-4000-8000-' || lpad(p::text, 12, '0'))::uuid)::text,
       jsonb_build_object('percent_complete', pct, 'position_ms', 0)
FROM active;

COMMIT;

-- Quick sanity echo (counts), printed after commit.
SELECT
    'b1000000-0000-4000-8000-000000000001' AS target_course_id,
    (SELECT count(*) FROM access_grant WHERE course_id = 'b1000000-0000-4000-8000-000000000001' AND revoked_at IS NULL) AS enrolled,
    (SELECT count(*) FROM event_log WHERE event_id::text LIKE 'b5000000-%') AS seeded_events;
