# syntax=docker/dockerfile:1

# ---- Builder ----
FROM golang:alpine AS builder
WORKDIR /app
RUN apk add --no-cache git ca-certificates
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/server ./cmd/server

# ---- Runtime ----
FROM alpine:3.21
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /app/server /app/server

EXPOSE 8080
ENTRYPOINT ["/app/server"]
