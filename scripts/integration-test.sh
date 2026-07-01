#!/usr/bin/env bash
# Run the end-to-end / integration tests (./e2e) against a throwaway Postgres.
#
# These tests TRUNCATE every table, so they must never run against a real
# database. This script starts a dedicated, disposable Postgres container on an
# unused host port, points the suite at it (SLP_E2E=1 + DB_* env), runs the
# tests, and tears the container down afterwards — your dev database on :5433 is
# never touched.
#
# Usage:
#   scripts/integration-test.sh            # run the suite
#   scripts/integration-test.sh -run Chain # pass extra `go test` flags through
set -euo pipefail

CONTAINER="slp-e2e-pg-$$"
HOST_PORT="${E2E_DB_PORT:-55432}"
PG_IMAGE="${E2E_PG_IMAGE:-postgres:16-alpine}"

cleanup() { docker rm -f "$CONTAINER" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "==> starting throwaway Postgres ($PG_IMAGE) on host port $HOST_PORT"
docker run -d --name "$CONTAINER" \
  -e POSTGRES_USER=admin -e POSTGRES_PASSWORD=qwerty -e POSTGRES_DB=db \
  -p "${HOST_PORT}:5432" "$PG_IMAGE" >/dev/null

echo -n "==> waiting for Postgres to accept connections"
for _ in $(seq 1 30); do
  if docker exec "$CONTAINER" pg_isready -U admin -d db >/dev/null 2>&1; then
    echo " ready"; break
  fi
  echo -n "."; sleep 1
done

# Migrations are applied automatically by the suite's TestMain on first run.
export SLP_E2E=1
export DB_HOST=localhost DB_PORT="$HOST_PORT"
export POSTGRES_USER=admin POSTGRES_PASSWORD=qwerty POSTGRES_DB=db

echo "==> running e2e tests"
go test ./e2e/... -count=1 -v "$@"
