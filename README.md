# Student Learning Portal - Backend

This is the backend service for the Student Learning Portal, built with Go, OpenAPI, Swagger.

## Prerequisites
- [Go](https://golang.org/doc/install) (1.20+ recommended)
- [Docker](https://docs.docker.com/get-docker/) & Docker Compose (for running the PostgreSQL database)
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
```

### 2. Start the Database (Docker)
To start the PostgreSQL database defined in `docker-compose.yml`:
```bash
docker-compose up -d
```

### 3. Run the Server
Run the HTTP server locally:
```bash
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
{"status":"connected (simulated)"}
```

## Running Unit Tests
To run tests across all internal packages:
```bash
go test ./... -v
```
