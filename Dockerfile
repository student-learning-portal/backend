# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o server ./cmd/portal && \
    go build -o import-practicum-courses ./cmd/import-practicum-courses

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/server .
COPY --from=builder /app/import-practicum-courses .

EXPOSE 8080

CMD ["./server"]
