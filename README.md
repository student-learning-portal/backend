# Student Learning Portal - Backend

This is the backend service for the Student Learning Portal, built with Go, OpenAPI, Swagger.

## Prerequisites
- [Go](https://golang.org/doc/install) (1.26+ recommended — matches the `go 1.26.4` directive in `go.mod`)
- [Docker](https://docs.docker.com/get-docker/) & Docker Compose (for running the PostgreSQL database)
- [golang-migrate](https://github.com/golang-migrate/migrate) (for running database migrations; no local install needed — used via Docker)
- [sqlc](https://sqlc.dev/) (optional, for code generation from SQL)
- [golangci-lint](https://golangci-lint.run/) v2.x built against a Go toolchain ≥ this module's `go` directive (an older build refuses to load the config — see Linting below)

## Project Structure
- `cmd/portal/main.go`: The main entry point to start the HTTP server.
- `cmd/analytics-loader/main.go`: Standalone binary that (re)computes the `analytics_student_course` rollup; see `analytics-ml-layer.md`.
- `configs/config.env`: Environment variables and secrets.
- `internal/`: Application code (`domain`, `usecase`, `database`, `delivery/http`, `security`, `logging`, `eventlog`, `practicum` — the course rating/review integration, see below).
- `api/openapi.yaml`: The OpenAPI/Swagger contract. `internal/delivery/http/router.go` hand-registers every route; the `api_types.gen.go` / `api_server.gen.go` files generated from this spec (see step 5 below) provide request/response types but their `ServerInterface` is **not** wired into the router — the hand-written mux is the actual source of truth for what's reachable.
- `migrations/`: Database schema migrations.
- `tools/sqlc/`: SQL queries for `sqlc` to generate database access code.
- `e2e/`: HTTP-level integration tests against a real Postgres (see "End-to-end / integration tests" below).
- `scripts/`: `seed.sql` / `seed_analytics.sql` fixtures and `integration-test.sh`, the one-command e2e runner.
- `deployments/`, `Dockerfile`, `docker-compose.yml`: container build/deploy config. `deployments/Dockerfile` and the root `docker-compose.yml` are currently empty placeholder files — the real dev stack (with its own `Dockerfile` build) lives in `../infra/docker-compose.yml`.
- `.golangci.yml`, `Makefile`: lint config and the `install-hooks` / `test` / `test-integration` targets.

## Getting Started

### 1. Environment Configuration
Define your environment variables. A template configuration can go into `configs/config.env`:
```env
PORT=8080
DATABASE_URL=postgres://user:password@localhost:5432/portal?sslmode=disable
JWT_SECRET=a-long-random-string
```
`JWT_SECRET` signs and verifies auth tokens; the server refuses to start without it. The Compose stack reads it from `infra/.env`.

### 2. Start the Database (Docker)
The PostgreSQL database (and the rest of the stack) is defined in `../infra/docker-compose.yml`. From the `infra/` directory:
```bash
docker compose up -d
```
Postgres is published on host port `5433`.

### 3. Run Database Migrations
Migrations live in `migrations/`. Apply them using the `migrate/migrate` Docker image so no local install is required — run from the `backend/` directory, attached to the compose network so it can resolve the `postgres` service by name:
```bash
docker run --rm -v "$(pwd)/migrations:/migrations" --network infra_default \
  migrate/migrate -path=/migrations -database "postgres://user:password@postgres:5432/database_name?sslmode=disable" up
```
Note: On Windows with Git Bash, prefix the command with `MSYS_NO_PATHCONV=1` if you get a "no such file or directory" error on `/migrations` — Git Bash otherwise mangles the Unix-style path.

Replace `user`/`password`/`database_name` with the values from your `infra/.env` (`POSTGRES_USER`/`POSTGRES_PASSWORD`/`POSTGRES_DB`), and the network name (`infra_default`) if your Compose project has a custom name. Useful variants:
```bash
# roll back the last migration
docker run --rm -v "$(pwd)/migrations:/migrations" --network infra_default \
  migrate/migrate -path=/migrations -database "postgres://user:password@postgres:5432/database_name?sslmode=disable" down 1

# check the currently applied migration version
docker run --rm -v "$(pwd)/migrations:/migrations" --network infra_default \
  migrate/migrate -path=/migrations -database "postgres://user:password@postgres:5432/database_name?sslmode=disable" version
```

### 4. Seed Test Data
`scripts/seed.sql` fills every table with a small set of related rows (teachers, students, courses across all statuses, lessons, media, materials, payments, access grants, access checks, progress, and events) so you can exercise endpoints against real data. Run it through the Postgres container started by Compose:
```bash
docker exec -i infra-postgres-1 psql -U user -d database_name < scripts/seed.sql
```
Replace the container name/credentials if they differ from your `infra/.env`.

### 5. Run the Server
Regenerate go-files
```bash
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.7.1 -generate types -package http -o internal/delivery/http/api_types.gen.go api/openapi.yaml
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.7.1 -generate std-http-server -package http -o internal/delivery/http/api_server.gen.go api/openapi.yaml
```

Run the HTTP server locally:
```bash
go build ./...
go run ./cmd/portal/main.go
```
The server will assemble its dependencies and start listening (default is `http://localhost:8080`).

## Testing the Endpoints

Once the server is running, you can test the basic endpoints using `curl` or your web browser.

**1. Hello World Endpoint**
Checks if the routing and HTTP delivery layer is functional.
```bash
curl -X GET http://localhost:8080/hello
```
*Expected Output:*
```json
{"message":"Hello, World!"}
```

**2. Database Health Check**
Verifies the database connection logic.
```bash
curl -X GET http://localhost:8080/api/v1/health/db
```
*Expected Output:*
```json
{"status":"connected"}
```

**3. Register**
Creates a new account (no email confirmation required) and returns a bearer token.
```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"alice@example.com","password":"password123","full_name":"Alice Johnson","role":"student"}'
```
*Expected Output:*
```json
{"token":"<jwt>","user":{"id":"...","email":"alice@example.com","full_name":"Alice Johnson","role":"student"}}
```

**4. Login**
```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"alice@example.com","password":"password123"}'
```

**5. Current User**
Requires the bearer token returned by register/login.
```bash
curl -X GET http://localhost:8080/api/v1/auth/me -H "Authorization: Bearer <jwt>"
```

**6. Player: lesson content (entitled users only)**
Returns the lesson's media URL, attachments, and the caller's last saved resume point.
The `RequireEntitlement` middleware first checks the caller holds an active access
grant for the course (seed a grant via `/purchase/checkout`), responding `403` otherwise.
```bash
curl -X GET "http://localhost:8080/api/v1/player/courses/<course_id>/lessons/<lesson_id>" \
  -H "Authorization: Bearer <jwt>"
```
*Example output:*
```json
{"lesson_id":"...","title":"Go Syntax Basics","content_url":"https://cdn.example.com/...","duration_seconds":347,"materials":[...],"last_progress_seconds":120,"percent_complete":35.5}
```

**7. Player: save progress**
Persists the caller's resume point for a lesson (upsert — re-saving overwrites in place).
`percent_complete` is derived server-side from the media duration (100 once `completed`).
```bash
curl -X POST "http://localhost:8080/api/v1/player/courses/<course_id>/lessons/<lesson_id>/progress" \
  -H "Authorization: Bearer <jwt>" -H "Content-Type: application/json" \
  -d '{"progress_seconds":120,"completed":false}'
```

**8. Player: resume progress**
Returns the saved resume point so playback can continue after re-login (`404` if never started).
```bash
curl -X GET "http://localhost:8080/api/v1/player/courses/<course_id>/lessons/<lesson_id>/progress" \
  -H "Authorization: Bearer <jwt>"
```

Note: the entitlement check above has one carve-out — a course's own teacher
always passes it, so they can preview their own lessons without a purchase.

### Full route reference

The examples above cover the core auth/player flow; the rest of the surface
(catalog, profile, purchasing, analytics, and teacher content authoring) is
documented in full in `api/openapi.yaml`. Quick index of every route
registered in `internal/delivery/http/router.go`:

| Method & Path | Auth | Notes |
|---|---|---|
| `GET /hello` | — | Liveness check |
| `GET /api/v1/health/db` | — | DB connectivity check |
| `GET /api/v1/catalog/courses` | — | Search/filter/sort/paginate published courses |
| `GET /api/v1/catalog/courses/{course_id}/lessons` | — | Lessons for a published course |
| `GET /api/v1/catalog/courses/{course_id}/rating` | — | Aggregate rating, proxied to the practicum-team service (see below) |
| `POST /api/v1/catalog/courses/{course_id}/comments` | JWT (student) | Leave a review, proxied to the practicum-team service (see below) |
| `POST /api/v1/auth/register` | — | Create account, returns JWT (a teacher lands in the approval queue — see below) |
| `POST /api/v1/auth/login` | — | Returns JWT |
| `GET /api/v1/auth/me` | JWT | Caller's profile (includes `teacher_status` for teachers) |
| `GET /api/v1/admin/teachers?status=` | JWT (admin) | Teacher approval queue (`pending` by default, `all` for every teacher) |
| `POST /api/v1/admin/teachers/{user_id}/approve` | JWT (admin) | Confirm a teacher registration |
| `POST /api/v1/admin/teachers/{user_id}/reject` | JWT (admin) | Decline it (reversible) |
| `GET /api/v1/teachers/{teacher_id}` | — | Public teacher profile (404s for non-teachers, avoids role enumeration) |
| `GET /api/v1/users/me/courses` | JWT | Caller's enrolled courses |
| `GET /api/v1/users/me/results` | JWT | Caller's per-course results |
| `PATCH /api/v1/users/me/{email,password,name}` | JWT | Profile edits |
| `POST /api/v1/users/me/avatar` | JWT | Upload avatar (served back from `/uploads/`) |
| `POST /api/v1/purchase/checkout` | JWT | Mock sandbox purchase |
| `POST /api/v1/purchase/refund` | JWT | Refund + revoke access |
| `POST /api/v1/purchase/webhook` | — | Gateway callback, idempotent on `transaction_id` |
| `GET /api/v1/purchase/history` | JWT | Caller's payment history |
| `GET /api/v1/player/courses/{course_id}/lessons/{lesson_id}` | JWT + entitlement | See item 6 above |
| `POST /api/v1/player/courses/{course_id}/lessons/{lesson_id}/progress` | JWT + entitlement | See item 7 above |
| `GET /api/v1/player/courses/{course_id}/lessons/{lesson_id}/progress` | JWT + entitlement | See item 8 above |
| `GET /api/v1/analytics/teacher/dashboard?course_id=` | JWT (approved teacher, owner) | AT_RISK/ON_TRACK per student |
| `GET /api/v1/analytics/student/me` | JWT | Caller's own progress across courses |
| `POST /api/v1/teacher/courses` | JWT (teacher) | Create draft course |
| `PATCH /api/v1/teacher/courses/{course_id}` | JWT (teacher, owner) | Update course / change status |
| `DELETE /api/v1/teacher/courses/{course_id}` | JWT (teacher, owner) | Draft-only (see `ErrCourseNotDraft`) |
| `POST /api/v1/teacher/courses/{course_id}/lessons` | JWT (teacher, owner) | Append lesson |
| `PUT /api/v1/teacher/courses/{course_id}/lessons/order` | JWT (teacher, owner) | Reorder lessons |
| `PATCH.../DELETE /api/v1/teacher/courses/{course_id}/lessons/{lesson_id}` | JWT (teacher, owner) | Edit / delete lesson |
| `PUT/DELETE .../lessons/{lesson_id}/media` | JWT (teacher, owner) | Set/remove the lesson's media asset |
| `POST/DELETE .../lessons/{lesson_id}/materials[/{material_id}]` | JWT (teacher, owner) | Add/remove attachments |

## End-to-end / integration tests

The `e2e/` package drives the **real** router — middleware, handlers, use cases,
and Postgres-backed repositories — over HTTP (`httptest`) against a real
database. No mocks: these tests confirm the pieces work *together*. Coverage:

- **Catalog** — published-only listing, search / subject / price filters, sort,
  pagination, lessons ordered by position.
- **Auth & profile** — register → login → `me` round-trip, duplicate-email
  conflict, generic login failure, teacher lookup hiding non-teachers.
- **Player & progress** — entitled access returns content, unentitled is `403`,
  a grant for one course can't read another's lesson, progress is scoped
  per-user (one learner never sees another's), and the audit log records every
  allow/deny.
- **The purchase chain** — `checkout → access → play → save progress → resume →
  refund → access revoked`, plus the gateway `webhook` SUCCESS/REFUNDED path,
  insufficient-funds (`402`), and unknown course (`404`).
- **Analytics** — teacher dashboard role + ownership gating, and AT_RISK /
  ON_TRACK classification + ordering over a seeded rollup.

### Running them

The tests **truncate every table**, so they must only run against a throwaway
database — never your dev DB. The one-command runner provisions a disposable
Postgres container, runs the suite, and tears it down:

```bash
# from backend/
make test-integration       # or: ./scripts/integration-test.sh
```

Schema migrations are applied automatically by the suite's `TestMain` the first
time it connects, so the throwaway database needs nothing but an empty Postgres.

**Safety / gating.** The suite only runs when `SLP_E2E=1` is set *and* a database
is reachable; otherwise every test **skips**. This means a bare `go test ./...`
never touches a database. To run against your own Postgres (must be disposable):

```bash
SLP_E2E=1 DB_HOST=localhost DB_PORT=5433 POSTGRES_USER=admin \
  POSTGRES_PASSWORD=qwerty POSTGRES_DB=db JWT_SECRET=any-local-secret \
  go test -v ./e2e/
```

## Administrator & Teacher Approval

Registering as a teacher no longer grants the role outright: the account is
created with `users.teacher_status = 'pending'` and an administrator has to
confirm it. Students are unaffected — they have no `teacher_status` at all.

**What a pending teacher can and cannot do.** They can sign in, read their own
profile, and use everything a signed-in user can; the `/api/v1/teacher/**`
routes and the teacher analytics dashboard answer `403` with
`{"error":"teacher account is awaiting administrator approval","status":"pending"}`
until the decision lands. The gate is the `RequireApprovedTeacher` middleware
(`internal/delivery/http/middleware.go`), which reads the status from the
database on every request — so an approval takes effect immediately, without
the teacher having to sign in again for a fresh token. Rejecting an already
approved teacher closes the endpoints again, and the decision stays reversible.

**The administrator account** is bootstrapped on startup
(`internal/app.go`'s `ensureAdminAccount`), because there is deliberately no way
to register one: `domain.Role.Valid` rejects `admin` at registration, so the
public endpoint can never mint a moderator. Defaults are `admin` / `admin111`:

| Var | Meaning | Default |
|---|---|---|
| `ADMIN_LOGIN` | Login for the admin account. Stored in the `email` column but not required to be an email address. | `admin` |
| `ADMIN_PASSWORD` | Password, hashed with bcrypt before storage (min 8 chars). | `admin111` |
| `ADMIN_FULL_NAME` | Display name. | `Администратор` |

The bootstrap only ever *creates* — an existing admin row is left untouched, so
a password changed on purpose survives restarts. To rotate the password back to
the configured one, delete the row (`DELETE FROM users WHERE role = 'admin'`)
and restart. If the login is already taken by a non-admin account, the server
logs the conflict and keeps serving without an administrator.

Walk through the flow with curl:
```bash
# 1. A teacher registers — note teacher_status in the response.
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"tess@example.com","password":"password123","full_name":"Tess","role":"teacher"}'

# 2. The administrator signs in and reads the queue.
ADMIN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin","password":"admin111"}' | jq -r .token)
curl -s http://localhost:8080/api/v1/admin/teachers -H "Authorization: Bearer $ADMIN"

# 3. And approves (or rejects) the application.
curl -X POST http://localhost:8080/api/v1/admin/teachers/<user_id>/approve \
  -H "Authorization: Bearer $ADMIN"
```

## Analytics Event Logging

The server emits a structured analytics event stream implementing
`logging-architecture.md`. Every event conforms to the envelope in §2 and fans
out to two sinks:
- **Raw NDJSON transport** — one JSON line per event on **stdout** (§1.1 / §5.1),
  ready to be tailed by a broker/loader.
- **Postgres `event_log`** — the hot envelope fields land in typed columns and the
  domain payload stays in the JSONB column (§5.2). Inserts are idempotent on
  `event_id` (§4 dedupe).

Audit-grade access state continues to live in the normalized `payment`,
`access_grant`, and `access_check_log` tables (§5.3); the event stream mirrors
those actions for analytics/replay without replacing the transactional source of
truth.

### Instrumented events
| Trigger | Events emitted |
|---|---|
| `POST /auth/register` | `auth.signup` |
| `POST /auth/login` | `auth.login` |
| `POST /admin/teachers/{id}/approve` \| `/reject` | `admin.teacher_approved` / `admin.teacher_rejected` |
| `POST /purchase/checkout` | `access.checkout_start`, `access.payment_succeeded`, `access.granted` |
| `POST /purchase/webhook` | `access.payment_succeeded` + `access.granted` (SUCCESS) / `access.refund_completed` + `access.revoked` (REFUNDED) |
| `RequireEntitlement` guard | `access.check` (+ `access.denied` on refusal) |
| `GET /player/.../lessons/{id}` | `player.lesson_open` |
| `POST /player/.../progress` | `player.progress_save` (+ `player.lesson_complete` when `completed`) |

### Configuration (env)
The per-instance `source` block (§2) is read from the environment:
| Var | Meaning | Default |
|---|---|---|
| `APP_ENV` | `dev` / `staging` / `prod` | `dev` |
| `HOSTNAME` | instance id | `local` |
| `RELEASE` | deployed git sha | `unknown` |

### Request headers honored
`WithLogContext` enriches each event from the request: `X-Correlation-ID`
(generated if absent), `X-Session-ID`, `X-Anonymous-ID`, W3C `traceparent`,
`Accept-Language`, `X-Forwarded-For` (IP is truncated to /24 or /48 for privacy,
§4), and consent via `X-Consent-Analytics` / `X-Consent-Marketing`. When analytics
consent is declined, only `pii_level=none` operational events are emitted.

### Observing events locally
Events print to stdout as NDJSON, e.g. after a login:
```json
{"schema_version":"1.0.0","event_id":"...","event_name":"auth.login","event_ts":"2026-06-23T12:00:00.000Z","actor":{"actor_id":"...","role":"student","auth_state":"authenticated"},"source":{"service":"gateway","env":"dev",...},"payload":{"method":"password"},"pii_level":"none","consent":{"analytics":true,"marketing":false}}
```
Inspect the Postgres load with:
```sql
SELECT event_name, actor_id, course_id, payload FROM event_log ORDER BY event_ts DESC LIMIT 20;
```

## Course Ratings & Reviews (practicum-team integration)

`GET /api/v1/catalog/courses/{course_id}/rating` and
`POST /api/v1/catalog/courses/{course_id}/comments` do not store anything in
our own database. They proxy to a separate team's already-running course
service (codename "SEHRIYO", `gitlab.pg.innopolis.university/practicum-team`)
rather than reimplementing their enrollment/progress-gated review logic —
see `internal/practicum`.

Because the two systems have completely independent Postgres databases with
independently-generated course IDs, a course is **mirrored** into their
system lazily, on the first rating/comment request for it
(`internal/practicum/review_repository.go`'s `ensureExternalCourse`); the
returned foreign ID is cached on our own `courses.external_course_id`
column (migration `000010`) so later requests don't mirror again.

### Setup

| Variable | Meaning |
|---|---|
| `PRACTICUM_API_URL` | Base URL of their API, e.g. `http://10.93.27.25:8000/api/v1`. |
| `PRACTICUM_JWT_SECRET` | Must equal *their* service's own `JWT_SECRET` — trust between the two independently-deployed services is a plain shared HMAC secret (their `pkg/jwt`), not our own session secret. |
| `PRACTICUM_INTEGRATION_TEACHER_ID` | A teacher account ID that already exists in *their* `teachers` table. Every mirrored course is created under this identity (their `POST /create-courses` requires teacher auth and FKs `teacher_id` to a real row) — register one once via their `POST /auth/register_teacher`, then set this to the returned `id`. |

Authentication between the two services works because
`internal/security`'s JWT now includes a `user_id` claim (in addition to the
standard `sub`) — that's the exact claim shape their `pkg/jwt.Claims`
expects, so a token minted by either service (given the shared secret) is
valid on both.

**Known limitation:** as of this writing the review/rating feature exists in
their repository but the instance at `10.93.27.25` predates it (its rating
endpoint 404s with a plain-text response, not their JSON error envelope) —
confirm with the practicum team that it's deployed before relying on this
integration.

## User Protection

- **Passwords** are hashed with bcrypt (`golang.org/x/crypto/bcrypt`, default cost) before storage — the `users.password_hash` column never holds plaintext, and only the hash is ever compared on login.
- **Sessions** are stateless HS256 JWTs (`github.com/golang-jwt/jwt/v5`) signed with `JWT_SECRET`, valid for 24 hours. The server refuses to boot without `JWT_SECRET` set.
- **Protected routes** go through the `RequireAuth` middleware (`internal/delivery/http/middleware.go`), which rejects requests with a missing, malformed, or expired bearer token before they reach the handler.
- **Input validation** on registration enforces a real email format, an 8-character password minimum, and a known role (`student`/`teacher`); duplicate emails are rejected with `409 Conflict` instead of leaking which check failed. `admin` is not an accepted role — see "Administrator & Teacher Approval" above.
- **Privilege escalation** is blocked on both ends: the public endpoint can't create an administrator, and the teacher role only becomes usable after an administrator approves it (`RequireApprovedTeacher`), which is re-checked against the database on every request rather than trusted from the token.
- **Login failures** (unknown email or wrong password) both return the same `401 Unauthorized` with a generic message, so the API doesn't reveal whether an email is registered.

## Running Unit Tests
To run tests across all internal packages:
```bash
go test ./... -v
```

## Linting
```bash
golangci-lint run ./...
```
Requires a golangci-lint build whose Go toolchain version is ≥ this module's
`go` directive (currently 1.26.4) — an older build refuses to load the config
with `can't load config: the Go language version ... is lower than the
targeted Go version`.

## Test Coverage & Lint Baseline

Snapshot from 2026-07-07 (`go test -short -cover ./...`, `golangci-lint v2.12.2`):

| Package | Coverage |
|---|---|
| `internal/security` | 92.3% |
| `internal/eventlog` | 83.3% |
| `internal/usecase` | 70.9% |
| `internal/delivery/http` | 40.5% |
| `internal/domain` | 29.6% |
| `internal/database`, `internal/logging`, `cmd/*` | 0% (no unit tests; `database` is exercised indirectly via `e2e/`) |

`golangci-lint run ./...` reports **24 issues**: 8 `lll` (line length), 5
`govet` (`err` shadowing in `internal/database/entitlement.go` /
`internal/database/lesson.go`), 3 `gofumpt` (formatting in test files), 3
`goconst` (repeated `"USD"` / `"video"` literals), 2 `dupl`
(`CourseStudentProgress` / `StudentCourseProgress` in
`internal/database/analytics.go` are near-identical queries), 1 `gocognit`
(`RequireEntitlement` middleware), 1 `contextcheck`, 1 `unparam`. None are
correctness bugs; re-run the command above for the current count before
relying on this table, since it will drift as the code changes.
