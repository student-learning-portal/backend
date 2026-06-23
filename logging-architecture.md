# Logging Architecture — Student & Sales Portal

> Machine-readable spec for raw event log collection.
> Scope: Catalog, Payment/Access, Player/Progress, Analytics surface. Excludes Lesson Studio.
> Derived/analytics layer is specified separately in `analytics-ml-layer.md`.

---

## 1. Conventions

### 1.1 Transport & Format
- **Format:** newline-delimited JSON (NDJSON), one event per line.
- **Encoding:** UTF-8.
- **Timestamps:** ISO 8601, UTC, millisecond precision (`2026-06-12T14:03:55.812Z`).
- **Naming:** `snake_case` for all keys; `domain.action` for `event_name`.
- **Versioning:** every event carries `schema_version` (semver). Breaking change → major bump.

### 1.2 Identifiers
| Field | Type | Description |
|---|---|---|
| `event_id` | uuid v4 | Unique per event. Idempotency key. |
| `correlation_id` | uuid v4 | Spans one logical user action across services. |
| `session_id` | uuid v4 | Client session, rotates on login/logout. |
| `actor_id` | string | Authenticated user id (student/teacher). Null if anonymous. |
| `anonymous_id` | uuid v4 | Stable pre-auth visitor id (cookie/device). |
| `trace_id` / `span_id` | string | Distributed tracing (W3C traceparent). |

---

## 2. Envelope Schema

Every event MUST conform to this envelope. Domain payload lives under `payload`.

```json
{
  "schema_version": "1.0.0",
  "event_id": "uuid",
  "event_name": "domain.action",
  "event_ts": "2026-06-12T14:03:55.812Z",
  "ingest_ts": "2026-06-12T14:03:56.001Z",
  "correlation_id": "uuid",
  "session_id": "uuid",
  "trace_id": "string",
  "span_id": "string",
  "actor": {
    "actor_id": "string|null",
    "anonymous_id": "uuid",
    "role": "student|teacher|guest|system",
    "auth_state": "authenticated|anonymous"
  },
  "source": {
    "service": "catalog|access|player|analytics|gateway",
    "env": "dev|staging|prod",
    "instance": "string",
    "release": "git_sha"
  },
  "context": {
    "ip_trunc": "string",
    "user_agent": "string",
    "device_type": "desktop|mobile|tablet",
    "locale": "string",
    "referrer": "string|null",
    "page_url": "string|null"
  },
  "payload": {},
  "pii_level": "none|low|high",
  "consent": { "analytics": true, "marketing": false }
}
```

---

## 3. Event Taxonomy

### 3.1 Catalog (`catalog.*`)
| event_name | Trigger | Key payload fields |
|---|---|---|
| `catalog.view_listing` | Catalog page rendered | `query`, `filters[]`, `sort`, `result_count`, `page` |
| `catalog.search` | Search executed | `query`, `result_count`, `latency_ms`, `zero_results` |
| `catalog.filter_apply` | Filter changed | `filter_field`, `filter_value`, `result_count` |
| `catalog.view_course_card` | Card impression | `course_id`, `position`, `surface` |
| `catalog.click_course` | Card clicked | `course_id`, `position`, `from_surface` |
| `catalog.view_course_detail` | Detail page open | `course_id`, `price`, `currency` |

### 3.2 Payment & Access (`access.*`) — HIGH PRIORITY, AUDIT-GRADE
> Append-only, immutable. Must reconstruct full access state at any timestamp.

| event_name | Trigger | Key payload fields |
|---|---|---|
| `access.checkout_start` | Checkout initiated | `course_id`, `price`, `currency`, `cart_id` |
| `access.payment_attempt` | Payment submitted | `cart_id`, `course_id`, `amount`, `method`, `sandbox` |
| `access.payment_succeeded` | Payment cleared | `cart_id`, `course_id`, `txn_id`, `amount` |
| `access.payment_failed` | Payment rejected | `cart_id`, `course_id`, `failure_code`, `failure_reason` |
| `access.granted` | Access opened | `course_id`, `grant_id`, `reason: purchase`, `effective_at` |
| `access.revoked` | Access closed | `course_id`, `grant_id`, `reason: refund|expiry|chargeback|manual`, `effective_at` |
| `access.refund_requested` | Refund opened | `txn_id`, `course_id`, `reason` |
| `access.refund_completed` | Refund settled | `txn_id`, `course_id`, `grant_id` |
| `access.check` | Authorization gate hit | `course_id`, `lesson_id`, `decision: allow|deny`, `deny_reason` |
| `access.denied` | Unauthorized attempt | `course_id`, `lesson_id`, `deny_reason`, `attempted_via` |

**Invariants (must hold in log):**
- Every `access.granted` has a preceding `access.payment_succeeded` with matching `course_id` + `actor_id`.
- Every `access.revoked` references an existing `grant_id`.
- `access.check` with `decision=allow` requires an active (granted, not revoked) grant at `event_ts`.

### 3.3 Player & Progress (`player.*`)
| event_name | Trigger | Key payload fields |
|---|---|---|
| `player.lesson_open` | Lesson opened | `course_id`, `lesson_id`, `lesson_type`, `resumed_from_ms` |
| `player.media_play` | Playback start/resume | `lesson_id`, `media_id`, `position_ms` |
| `player.media_pause` | Playback paused | `lesson_id`, `media_id`, `position_ms` |
| `player.media_seek` | Scrub | `lesson_id`, `media_id`, `from_ms`, `to_ms` |
| `player.media_rate_change` | Speed change | `lesson_id`, `playback_rate` |
| `player.heartbeat` | Periodic (e.g. 15s) | `lesson_id`, `media_id`, `position_ms`, `watched_delta_ms` |
| `player.media_complete` | Media finished | `lesson_id`, `media_id`, `duration_ms` |
| `player.progress_save` | Resume point persisted | `course_id`, `lesson_id`, `position_ms`, `percent_complete` |
| `player.lesson_complete` | Lesson done | `course_id`, `lesson_id`, `completion_pct` |
| `player.material_open` | Attachment/material viewed | `lesson_id`, `material_id`, `material_type` |
| `player.error` | Playback error | `lesson_id`, `media_id`, `error_code`, `position_ms` |

### 3.4 Assessment (`assessment.*`)
| event_name | Trigger | Key payload fields |
|---|---|---|
| `assessment.start` | Quiz/task started | `lesson_id`, `assessment_id`, `attempt_no` |
| `assessment.answer_submit` | Answer submitted | `assessment_id`, `question_id`, `is_correct`, `time_ms` |
| `assessment.submit` | Assessment finished | `assessment_id`, `score`, `max_score`, `passed` |

### 3.5 Analytics Surface (`report.*`)
> Logging consumption of dashboards (meta-analytics).

| event_name | Trigger | Key payload fields |
|---|---|---|
| `report.teacher_dashboard_view` | Teacher dashboard open | `course_id`, `cohort_id`, `widgets[]` |
| `report.student_progress_view` | Student views own progress | `course_id` |
| `report.export` | Data exported | `report_type`, `format`, `row_count` |

### 3.6 Lifecycle & System (`session.*`, `auth.*`)
| event_name | Trigger | Key payload fields |
|---|---|---|
| `session.start` | Session created | `entry_url`, `referrer` |
| `session.end` | Session closed | `duration_ms` |
| `auth.login` | Login success | `method` |
| `auth.logout` | Logout | — |
| `auth.signup` | Account created | `role` |

---

## 4. Data Governance

- **PII:** tag with `pii_level`; `ip_trunc` only (last octet zeroed); never log raw payment instruments (sandbox tokens only).
- **Consent:** honor `consent.analytics`; events from non-consenting actors limited to `pii_level=none` operational logging.
- **Retention:** `access.*` audit events — long retention / immutable store. Behavioral events — configurable TTL.
- **Idempotency:** dedupe on `event_id` at ingest.
- **Ordering:** never trust client clock for state; reconcile via `ingest_ts` + sequence; access state derived server-side.

---

## 5. Storage Schema (PostgreSQL)

> Two-tier model. NDJSON (Section 1–2) is the raw transport/append-only log.
> Events are loaded into Postgres for querying. The `access_*` tables are
> normalized & audit-grade; behavioral events use a hybrid JSONB table.

### 5.1 Pipeline
```
service → NDJSON log / broker → loader (dedupe on event_id) → Postgres
```
Raw log is retained for replay (re-deriving tables on schema change) and audit.

### 5.2 Behavioral events (hybrid: typed columns + JSONB)

Hot-filtered fields are promoted to real columns and indexed; the full
domain payload stays in `payload JSONB` for flexibility across event types.

```sql
CREATE TABLE event_log (
    event_id        UUID PRIMARY KEY,
    event_name      TEXT        NOT NULL,
    schema_version  TEXT        NOT NULL,
    event_ts        TIMESTAMPTZ NOT NULL,
    ingest_ts       TIMESTAMPTZ NOT NULL DEFAULT now(),
    correlation_id  UUID,
    session_id      UUID,
    actor_id        TEXT,
    anonymous_id    UUID,
    role            TEXT,
    service         TEXT        NOT NULL,
    env             TEXT        NOT NULL,
    course_id       TEXT,
    lesson_id       TEXT,
    payload         JSONB       NOT NULL DEFAULT '{}'::jsonb,
    pii_level       TEXT        NOT NULL DEFAULT 'none'
);

CREATE INDEX ix_event_name_ts ON event_log (event_name, event_ts DESC);
CREATE INDEX ix_event_actor   ON event_log (actor_id, event_ts DESC);
CREATE INDEX ix_event_course  ON event_log (course_id, event_ts DESC);
CREATE INDEX ix_event_session ON event_log (session_id);
CREATE INDEX ix_event_payload ON event_log USING GIN (payload);
-- optional: monthly partitioning on event_ts when volume grows
```

### 5.3 Access & payments (normalized, audit-grade)

Append-only, immutable. Enforces the invariants from Section 3.2 with
foreign keys instead of trusting application code.

```sql
CREATE TABLE payment (
    txn_id          UUID PRIMARY KEY,
    cart_id         UUID        NOT NULL,
    actor_id        TEXT        NOT NULL,
    course_id       TEXT        NOT NULL,
    amount          NUMERIC(12,2) NOT NULL,
    currency        TEXT        NOT NULL,
    status          TEXT        NOT NULL
        CHECK (status IN ('attempted','succeeded','failed','refunded')),
    sandbox         BOOLEAN     NOT NULL DEFAULT true,
    failure_code    TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE access_grant (
    grant_id        UUID PRIMARY KEY,
    actor_id        TEXT        NOT NULL,
    course_id       TEXT        NOT NULL,
    txn_id          UUID        NOT NULL REFERENCES payment (txn_id),
    granted_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ,                      -- NULL = active
    revoke_reason   TEXT
        CHECK (revoke_reason IN ('refund','expiry','chargeback','manual')),
    UNIQUE (actor_id, course_id, txn_id)
);

-- active access = grant exists AND revoked_at IS NULL AND now() within window
CREATE INDEX ix_grant_active
    ON access_grant (actor_id, course_id)
    WHERE revoked_at IS NULL;

-- immutable audit trail of every authorization decision
CREATE TABLE access_check_log (
    event_id        UUID PRIMARY KEY,
    actor_id        TEXT,
    course_id       TEXT        NOT NULL,
    lesson_id       TEXT,
    decision        TEXT        NOT NULL CHECK (decision IN ('allow','deny')),
    deny_reason     TEXT,
    checked_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ix_check_actor_course ON access_check_log (actor_id, course_id, checked_at DESC);
```

**Enforced invariants:**
- `access_grant.txn_id` → `payment.txn_id` (no grant without a payment row).
- An `allow` decision requires an active row in `access_grant` at check time.
- Tables are append-only; revocation = setting `revoked_at`, never deleting.

### 5.4 Progress (mutable current-state + history)

Resume point is a mutable upsert; the full history lives in `event_log`
(`player.progress_save`) for curve analysis.

```sql
CREATE TABLE progress_state (
    actor_id        TEXT        NOT NULL,
    course_id       TEXT        NOT NULL,
    lesson_id       TEXT        NOT NULL,
    position_ms     INTEGER     NOT NULL DEFAULT 0,
    percent_complete NUMERIC(5,2) NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (actor_id, course_id, lesson_id)
);
```

### 5.5 Engine notes
- `JSONB` (not `JSON`): binary, indexable, supports GIN + `@>` containment.
- Keep tables raw: any aggregation/derivation lives in a separate layer over
  `event_log` + the normalized tables, so the log stays replayable.
- If behavioral volume outgrows Postgres (heavy `player.heartbeat` / impressions),
  move `event_log` to a columnar store (ClickHouse / DuckDB); keep `access_*` in
  Postgres for transactional integrity.
