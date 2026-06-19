-- ============================================================
-- Seed data for manual/local testing of catalog, player, purchase,
-- and analytics endpoints against a real database.
--
-- Safe to re-run: truncates all domain tables first (idempotent).
-- IDs are hardcoded UUIDs so rows can
-- reference each other correctly within plain SQL inserts.
-- ============================================================

BEGIN;

TRUNCATE TABLE
    event_log,
    access_check_log,
    access_grant,
    payment,
    progress_state,
    materials,
    media,
    lessons,
    courses,
    users
RESTART IDENTITY CASCADE;

-- ============================================================
-- Users: 4 teachers, 4 students
-- ============================================================
INSERT INTO users (id, email, password_hash, full_name, role) VALUES
    ('11111111-1111-4111-8111-000000000001', 'alice.teacher@example.com', '$2a$10$placeholderhashplaceholderhash1', 'Alice Johnson', 'teacher'),
    ('11111111-1111-4111-8111-000000000002', 'bob.teacher@example.com',   '$2a$10$placeholderhashplaceholderhash2', 'Bob Smith',     'teacher'),
    ('11111111-1111-4111-8111-000000000003', 'carol.teacher@example.com', '$2a$10$placeholderhashplaceholderhash3', 'Carol Lee',     'teacher'),
    ('11111111-1111-4111-8111-000000000004', 'dave.teacher@example.com',  '$2a$10$placeholderhashplaceholderhash4', 'Dave Patel',    'teacher'),
    ('22222222-2222-4222-8222-000000000001', 'eve.student@example.com',   '$2a$10$placeholderhashplaceholderhash5', 'Eve Davis',     'student'),
    ('22222222-2222-4222-8222-000000000002', 'frank.student@example.com', '$2a$10$placeholderhashplaceholderhash6', 'Frank Miller',  'student'),
    ('22222222-2222-4222-8222-000000000003', 'grace.student@example.com', '$2a$10$placeholderhashplaceholderhash7', 'Grace Wilson',  'student'),
    ('22222222-2222-4222-8222-000000000004', 'heidi.student@example.com', '$2a$10$placeholderhashplaceholderhash8', 'Heidi Clark',   'student');

-- ============================================================
-- Courses: mix of statuses and price points, across 4 teachers
-- ============================================================
INSERT INTO courses (id, teacher_id, title, description, price, currency, status) VALUES
    ('33333333-3333-4333-8333-000000000001', '11111111-1111-4111-8111-000000000001', 'Introduction to Go',           'Learn the basics of Go programming.',              49.99, 'USD', 'published'),
    ('33333333-3333-4333-8333-000000000002', '11111111-1111-4111-8111-000000000001', 'Advanced Go Concurrency',      'Master goroutines and channels.',                   79.99, 'USD', 'published'),
    ('33333333-3333-4333-8333-000000000003', '11111111-1111-4111-8111-000000000002', 'Fullstack React',              'Build modern web apps with React.',                 59.99, 'USD', 'published'),
    ('33333333-3333-4333-8333-000000000004', '11111111-1111-4111-8111-000000000003', 'Data Science with Pandas',     'Data analysis and visualization in Python.',        89.99, 'USD', 'draft'),
    ('33333333-3333-4333-8333-000000000005', '11111111-1111-4111-8111-000000000004', 'Kubernetes Mastery',          'Deploy and manage containers at scale.',            89.99, 'USD', 'published'),
    ('33333333-3333-4333-8333-000000000006', '11111111-1111-4111-8111-000000000004', 'Legacy Jenkins Pipelines',     'Retired course on Jenkins CI/CD pipelines.',        19.99, 'USD', 'archived');

-- ============================================================
-- Lessons: 3 per active course, 2 for the draft, 1 for the archived one
-- ============================================================
INSERT INTO lessons (id, course_id, title, lesson_type, position) VALUES
    ('44444444-4444-4444-8444-000000000001', '33333333-3333-4333-8333-000000000001', 'Go Syntax Basics',          'video', 1),
    ('44444444-4444-4444-8444-000000000002', '33333333-3333-4333-8333-000000000001', 'Types and Interfaces',      'video', 2),
    ('44444444-4444-4444-8444-000000000003', '33333333-3333-4333-8333-000000000001', 'Module Quiz',               'quiz',  3),

    ('44444444-4444-4444-8444-000000000004', '33333333-3333-4333-8333-000000000002', 'Goroutines 101',            'video', 1),
    ('44444444-4444-4444-8444-000000000005', '33333333-3333-4333-8333-000000000002', 'Channels and Select',       'video', 2),
    ('44444444-4444-4444-8444-000000000006', '33333333-3333-4333-8333-000000000002', 'Race Conditions Lab',       'mixed', 3),

    ('44444444-4444-4444-8444-000000000007', '33333333-3333-4333-8333-000000000003', 'Components and Props',      'video', 1),
    ('44444444-4444-4444-8444-000000000008', '33333333-3333-4333-8333-000000000003', 'State and Hooks',           'video', 2),
    ('44444444-4444-4444-8444-000000000009', '33333333-3333-4333-8333-000000000003', 'Routing Basics',            'text',  3),

    ('44444444-4444-4444-8444-000000000010', '33333333-3333-4333-8333-000000000004', 'Loading Data with Pandas',  'video', 1),
    ('44444444-4444-4444-8444-000000000011', '33333333-3333-4333-8333-000000000004', 'Cleaning and Reshaping',    'text',  2),

    ('44444444-4444-4444-8444-000000000012', '33333333-3333-4333-8333-000000000005', 'Pods and Deployments',      'video', 1),
    ('44444444-4444-4444-8444-000000000013', '33333333-3333-4333-8333-000000000005', 'Services and Ingress',      'video', 2),
    ('44444444-4444-4444-8444-000000000014', '33333333-3333-4333-8333-000000000005', 'Cluster Troubleshooting',   'mixed', 3),

    ('44444444-4444-4444-8444-000000000015', '33333333-3333-4333-8333-000000000006', 'Freestyle vs Declarative Pipelines', 'text', 1);

-- ============================================================
-- Media: one video asset per lesson
-- ============================================================
INSERT INTO media (id, lesson_id, url, duration_ms, media_type)
SELECT
    ('55555555-5555-4555-8555-' || lpad((row_number() over (order by id))::text, 12, '0'))::uuid,
    id,
    'https://cdn.example.com/lessons/' || id || '/video.mp4',
    (300 + (row_number() over (order by id)) * 47) * 1000,
    'video'
FROM lessons;

-- ============================================================
-- Materials: one attachment per lesson
-- ============================================================
INSERT INTO materials (id, lesson_id, title, url, material_type)
SELECT
    ('66666666-6666-4666-8666-' || lpad((row_number() over (order by id))::text, 12, '0'))::uuid,
    id,
    title || ' - Slides',
    'https://cdn.example.com/lessons/' || id || '/slides.pdf',
    'pdf'
FROM lessons;

-- ============================================================
-- Payments: a mix of succeeded, failed, and refunded sandbox transactions
-- ============================================================
INSERT INTO payment (txn_id, cart_id, actor_id, course_id, amount, currency, status, sandbox, failure_code) VALUES
    ('77777777-7777-4777-8777-000000000001', '77777777-aaaa-4aaa-8aaa-000000000001', '22222222-2222-4222-8222-000000000001', '33333333-3333-4333-8333-000000000001', 49.99, 'USD', 'succeeded', true, NULL),
    ('77777777-7777-4777-8777-000000000002', '77777777-aaaa-4aaa-8aaa-000000000002', '22222222-2222-4222-8222-000000000002', '33333333-3333-4333-8333-000000000001', 49.99, 'USD', 'succeeded', true, NULL),
    ('77777777-7777-4777-8777-000000000003', '77777777-aaaa-4aaa-8aaa-000000000003', '22222222-2222-4222-8222-000000000001', '33333333-3333-4333-8333-000000000003', 59.99, 'USD', 'succeeded', true, NULL),
    ('77777777-7777-4777-8777-000000000004', '77777777-aaaa-4aaa-8aaa-000000000004', '22222222-2222-4222-8222-000000000003', '33333333-3333-4333-8333-000000000005', 89.99, 'USD', 'failed',    true, 'card_declined'),
    ('77777777-7777-4777-8777-000000000005', '77777777-aaaa-4aaa-8aaa-000000000005', '22222222-2222-4222-8222-000000000004', '33333333-3333-4333-8333-000000000002', 79.99, 'USD', 'refunded',  true, NULL);

-- ============================================================
-- Access grants: one per succeeded payment; the refunded one is revoked
-- ============================================================
INSERT INTO access_grant (grant_id, actor_id, course_id, txn_id, granted_at, revoked_at, revoke_reason) VALUES
    ('88888888-8888-4888-8888-000000000001', '22222222-2222-4222-8222-000000000001', '33333333-3333-4333-8333-000000000001', '77777777-7777-4777-8777-000000000001', now() - interval '14 days', NULL, NULL),
    ('88888888-8888-4888-8888-000000000002', '22222222-2222-4222-8222-000000000002', '33333333-3333-4333-8333-000000000001', '77777777-7777-4777-8777-000000000002', now() - interval '10 days', NULL, NULL),
    ('88888888-8888-4888-8888-000000000003', '22222222-2222-4222-8222-000000000001', '33333333-3333-4333-8333-000000000003', '77777777-7777-4777-8777-000000000003', now() - interval '7 days',  NULL, NULL),
    ('88888888-8888-4888-8888-000000000004', '22222222-2222-4222-8222-000000000004', '33333333-3333-4333-8333-000000000002', '77777777-7777-4777-8777-000000000005', now() - interval '5 days',  now() - interval '1 day', 'refund');

-- ============================================================
-- Access check log: a handful of allow/deny authorization decisions
-- ============================================================
INSERT INTO access_check_log (event_id, actor_id, course_id, lesson_id, decision, deny_reason, checked_at) VALUES
    ('99999999-9999-4999-8999-000000000001', '22222222-2222-4222-8222-000000000001', '33333333-3333-4333-8333-000000000001', '44444444-4444-4444-8444-000000000001', 'allow', NULL,                    now() - interval '13 days'),
    ('99999999-9999-4999-8999-000000000002', '22222222-2222-4222-8222-000000000002', '33333333-3333-4333-8333-000000000001', '44444444-4444-4444-8444-000000000002', 'allow', NULL,                    now() - interval '9 days'),
    ('99999999-9999-4999-8999-000000000003', '22222222-2222-4222-8222-000000000004', '33333333-3333-4333-8333-000000000002', '44444444-4444-4444-8444-000000000004', 'deny',  'access_revoked',        now() - interval '1 day'),
    ('99999999-9999-4999-8999-000000000004', '22222222-2222-4222-8222-000000000003', '33333333-3333-4333-8333-000000000005', '44444444-4444-4444-8444-000000000012', 'deny',  'no_active_entitlement', now() - interval '2 days');

-- ============================================================
-- Progress state: current resume point for active learners
-- ============================================================
INSERT INTO progress_state (actor_id, course_id, lesson_id, position_ms, percent_complete, updated_at) VALUES
    ('22222222-2222-4222-8222-000000000001', '33333333-3333-4333-8333-000000000001', '44444444-4444-4444-8444-000000000001', 347000, 100.00, now() - interval '13 days'),
    ('22222222-2222-4222-8222-000000000001', '33333333-3333-4333-8333-000000000001', '44444444-4444-4444-8444-000000000002', 120000, 35.50,  now() - interval '12 days'),
    ('22222222-2222-4222-8222-000000000002', '33333333-3333-4333-8333-000000000001', '44444444-4444-4444-8444-000000000001', 394000, 100.00, now() - interval '8 days'),
    ('22222222-2222-4222-8222-000000000001', '33333333-3333-4333-8333-000000000003', '44444444-4444-4444-8444-000000000007', 200000, 60.00,  now() - interval '6 days');

-- ============================================================
-- Event log: a few representative behavioral events
-- ============================================================
INSERT INTO event_log (event_id, event_name, schema_version, event_ts, actor_id, role, service, env, course_id, lesson_id, payload) VALUES
    ('aaaaaaaa-aaaa-4aaa-8aaa-000000000001', 'course_purchased', '1.0', now() - interval '14 days', '22222222-2222-4222-8222-000000000001', 'student', 'catalog', 'dev', '33333333-3333-4333-8333-000000000001', NULL,                                     '{"amount": 49.99, "currency": "USD"}'),
    ('aaaaaaaa-aaaa-4aaa-8aaa-000000000002', 'lesson_started',   '1.0', now() - interval '13 days', '22222222-2222-4222-8222-000000000001', 'student', 'player',  'dev', '33333333-3333-4333-8333-000000000001', '44444444-4444-4444-8444-000000000001', '{}'),
    ('aaaaaaaa-aaaa-4aaa-8aaa-000000000003', 'lesson_completed', '1.0', now() - interval '13 days', '22222222-2222-4222-8222-000000000001', 'student', 'player',  'dev', '33333333-3333-4333-8333-000000000001', '44444444-4444-4444-8444-000000000001', '{"percent_complete": 100}'),
    ('aaaaaaaa-aaaa-4aaa-8aaa-000000000004', 'payment_refunded', '1.0', now() - interval '1 day',   '22222222-2222-4222-8222-000000000004', 'student', 'gateway', 'dev', '33333333-3333-4333-8333-000000000002', NULL,                                     '{"reason": "refund"}'),
    ('aaaaaaaa-aaaa-4aaa-8aaa-000000000005', 'access_denied',    '1.0', now() - interval '2 days',  '22222222-2222-4222-8222-000000000003', 'student', 'access',  'dev', '33333333-3333-4333-8333-000000000005', '44444444-4444-4444-8444-000000000012', '{"reason": "no_active_entitlement"}'),
    ('aaaaaaaa-aaaa-4aaa-8aaa-000000000006', 'catalog_search',   '1.0', now() - interval '3 days',  NULL,                                    'guest',   'catalog', 'dev', NULL,                                   NULL,                                     '{"query": "go", "results": 3}');

COMMIT;
