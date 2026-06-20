# Student Learning Portal - Backend

This is the backend service for the Student Learning Portal, built with Go, OpenAPI, Swagger.

## Prerequisites
- [Go](https://golang.org/doc/install) (1.25+ recommended)
- [Docker](https://docs.docker.com/get-docker/) & Docker Compose (for running the PostgreSQL database)
- [golang-migrate](https://github.com/golang-migrate/migrate) (for running database migrations; no local install needed — used via Docker)
- [sqlc](https://sqlc.dev/) (optional, for code generation from SQL)

## Project Structure
- `cmd/portal/main.go`: The main entry point to start the server.
- `configs/config.env`: Environment variables and secrets.
- `internal/`: Application code.
- `migrations/`: Database schema migrations.
- `tools/sqlc/`: SQL queries for `sqlc` to generate database access code.

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

## User Protection

- **Passwords** are hashed with bcrypt (`golang.org/x/crypto/bcrypt`, default cost) before storage — the `users.password_hash` column never holds plaintext, and only the hash is ever compared on login.
- **Sessions** are stateless HS256 JWTs (`github.com/golang-jwt/jwt/v5`) signed with `JWT_SECRET`, valid for 24 hours. The server refuses to boot without `JWT_SECRET` set.
- **Protected routes** go through the `RequireAuth` middleware (`internal/delivery/http/middleware.go`), which rejects requests with a missing, malformed, or expired bearer token before they reach the handler.
- **Input validation** on registration enforces a real email format, an 8-character password minimum, and a known role (`student`/`teacher`); duplicate emails are rejected with `409 Conflict` instead of leaking which check failed.
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
