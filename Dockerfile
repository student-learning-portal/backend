FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN go build -o server ./cmd/portal
RUN go build -o import-practicum-courses ./cmd/import-practicum-courses

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/server .
COPY --from=builder /app/import-practicum-courses .

EXPOSE 8080

CMD ["./server"]