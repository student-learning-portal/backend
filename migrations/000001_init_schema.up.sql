-- ============================================================
-- Behavioral event log (hybrid: typed columns + JSONB payload)
-- ============================================================
CREATE TABLE event_log (
    event_id        UUID        PRIMARY KEY,
    event_name      TEXT        NOT NULL,
    schema_version  TEXT        NOT NULL,
    event_ts        TIMESTAMPTZ NOT NULL,
    ingest_ts       TIMESTAMPTZ NOT NULL DEFAULT now(),
    correlation_id  UUID,
    session_id      UUID,
    actor_id        TEXT,
    anonymous_id    UUID,
    role            TEXT        CHECK (role IN ('student', 'teacher', 'guest', 'system')),
    service         TEXT        NOT NULL CHECK (service IN ('catalog', 'access', 'player', 'analytics', 'gateway')),
    env             TEXT        NOT NULL CHECK (env IN ('dev', 'staging', 'prod')),
    course_id       TEXT,
    lesson_id       TEXT,
    payload         JSONB       NOT NULL DEFAULT '{}'::jsonb,
    pii_level       TEXT        NOT NULL DEFAULT 'none' CHECK (pii_level IN ('none', 'low', 'high'))
);

CREATE INDEX ix_event_name_ts ON event_log (event_name, event_ts DESC);
CREATE INDEX ix_event_actor   ON event_log (actor_id, event_ts DESC);
CREATE INDEX ix_event_course  ON event_log (course_id, event_ts DESC);
CREATE INDEX ix_event_session ON event_log (session_id);
CREATE INDEX ix_event_payload ON event_log USING GIN (payload);

-- ============================================================
-- Payment (audit-grade, append-only — never DELETE or UPDATE)
-- ============================================================
CREATE TABLE payment (
    txn_id          UUID          PRIMARY KEY,
    cart_id         UUID          NOT NULL,
    actor_id        TEXT          NOT NULL,
    course_id       TEXT          NOT NULL,
    amount          NUMERIC(12,2) NOT NULL,
    currency        TEXT          NOT NULL,
    status          TEXT          NOT NULL
        CHECK (status IN ('attempted', 'succeeded', 'failed', 'refunded')),
    sandbox         BOOLEAN       NOT NULL DEFAULT true,
    failure_code    TEXT,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX ix_payment_actor_course ON payment (actor_id, course_id);

-- ============================================================
-- Access grant (audit-grade; revocation = set revoked_at, never delete)
-- ============================================================
CREATE TABLE access_grant (
    grant_id        UUID        PRIMARY KEY,
    actor_id        TEXT        NOT NULL,
    course_id       TEXT        NOT NULL,
    txn_id          UUID        NOT NULL REFERENCES payment (txn_id),
    granted_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ,
    revoke_reason   TEXT
        CHECK (revoke_reason IN ('refund', 'expiry', 'chargeback', 'manual')),
    UNIQUE (actor_id, course_id, txn_id)
);

CREATE INDEX ix_grant_active ON access_grant (actor_id, course_id)
    WHERE revoked_at IS NULL;

-- ============================================================
-- Access check log (immutable audit trail of every authz decision)
-- ============================================================
CREATE TABLE access_check_log (
    event_id        UUID        PRIMARY KEY,
    actor_id        TEXT,
    course_id       TEXT        NOT NULL,
    lesson_id       TEXT,
    decision        TEXT        NOT NULL CHECK (decision IN ('allow', 'deny')),
    deny_reason     TEXT,
    checked_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ix_check_actor_course ON access_check_log (actor_id, course_id, checked_at DESC);

-- ============================================================
-- Progress state (mutable current resume point; history in event_log)
-- ============================================================
CREATE TABLE progress_state (
    actor_id         TEXT         NOT NULL,
    course_id        TEXT         NOT NULL,
    lesson_id        TEXT         NOT NULL,
    position_ms      INTEGER      NOT NULL DEFAULT 0,
    percent_complete NUMERIC(5,2) NOT NULL DEFAULT 0,
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    PRIMARY KEY (actor_id, course_id, lesson_id)
);
