-- ============================================================
-- Catalog browse/search is unindexed for its two hottest predicates:
--
-- 1. search ILIKE '%term%' against title/description forces a sequential
--    scan of courses (a leading wildcard defeats any plain btree index).
--    pg_trgm GIN indexes let Postgres use a trigram similarity scan instead.
-- 2. Every browse query filters status = 'published' and commonly narrows
--    further by subject and sorts by price; a partial composite index on
--    the published subset avoids scanning draft/archived rows.
-- ============================================================
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS ix_courses_title_trgm
    ON courses USING GIN (title gin_trgm_ops);

CREATE INDEX IF NOT EXISTS ix_courses_description_trgm
    ON courses USING GIN (description gin_trgm_ops);

CREATE INDEX IF NOT EXISTS ix_courses_subject_trgm
    ON courses USING GIN (subject gin_trgm_ops);

CREATE INDEX IF NOT EXISTS ix_courses_published_subject_price
    ON courses (subject, price)
    WHERE status = 'published';
