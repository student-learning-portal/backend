-- ============================================================
-- Notifications (in-app "bell" feed)
--
-- One row per delivered notification, addressed to a single recipient
-- (user_id). Today the only producer is the course chat: when a message is
-- posted, the *other* participant in the thread gets a 'message' notification
-- (see usecase.ChatUseCase). The schema is deliberately generic (type/title/
-- body) so other producers can be added without a migration.
--
-- course_id is an optional deep-link target: the client opens the relevant
-- course chat when the notification is clicked. It is nullable because a future
-- notification type may have nothing to do with a course.
--
-- read_at NULL means "unread": the unread badge on the bell counts these, and
-- marking a notification (or all of them) read simply stamps this column.
-- ============================================================
CREATE TABLE notifications (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    type       TEXT        NOT NULL,
    title      TEXT        NOT NULL,
    body       TEXT        NOT NULL,
    course_id  UUID        REFERENCES courses (id) ON DELETE CASCADE,
    read_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The feed is always "this user's notifications, newest first".
CREATE INDEX ix_notifications_user ON notifications (user_id, created_at DESC);

-- The unread badge counts unread rows per user; a partial index keeps that
-- count cheap as read notifications accumulate.
CREATE INDEX ix_notifications_user_unread ON notifications (user_id) WHERE read_at IS NULL;
