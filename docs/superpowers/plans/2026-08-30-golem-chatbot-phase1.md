# Golem Chatbot Phase 1 (Chat) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A chat-only vertical slice: JWT auth against Postgres, a shared golem v0.7.1 agent over Gemini with SSE streaming, usage/cost metering, and a React frontend with beautifului-style AI states. RAG is Phase 2 (spec: `docs/superpowers/specs/2026-08-30-golem-chatbot-design.md`).

**Architecture:** Go backend (stdlib `net/http`, Go 1.22+ routing) with `internal/` packages: `config`, `auth` (primitives + service), `storage` (pgx + goose embedded migrations), `cost`, `chat` (shared golem agent + sink contract), `httpapi` (handlers, middleware, SSE). One shared agent built at startup; per-request events via `golem.WithRunObserver`; failed runs persist `RunError.Partial`. React 19 + Vite + TS strict + TanStack Query + Zustand + Tailwind v4 + shadcn/ui; SSE consumed with `fetch` + `ReadableStream`.

**Tech Stack:** golem v0.7.1 (`github.com/abubakarsiddik31/golem`), pgx/v5 + goose/v3 + golang-jwt/v5 + x/crypto, Docker Compose (postgres:17; weaviate/minio dormant behind the `rag` profile), Vite/React 19/TypeScript, Tailwind v4, shadcn/ui, vitest + RTL + MSW.

## Global Constraints

- Go 1.26.5; backend module `github.com/abubakarsiddik31/golem-chatbot` rooted at `backend/`.
- Golem: `go get github.com/abubakarsiddik31/golem@v0.7.1`; if the proxy lacks the tag, add `replace github.com/abubakarsiddik31/golem => ../golem-agent` to `backend/go.mod`.
- Backend tests are fully offline: golem models are `testmodel.Scripted`/`testmodel.Func` fakes; DB-dependent tests read `TEST_DATABASE_URL` and skip when absent. No sleeps in assertions.
- Env-only config; secrets never committed. `.env.example` lists every variable with a safe dev default.
- Required backend checks per task: `go fmt ./... && go build ./... && go vet ./... && go test ./...` (run inside `backend/`). Frontend: `npm run check` (lint + typecheck + test) inside `frontend/`.
- Conventional Commit subjects. One coherent verified change per commit.
- **This plan file is a local working document — do NOT commit it** (matches the golem-lab convention).
- Cost unit is integer micro-USD (`cost_micro_usd`); rates are USD per 1M tokens (`gemini-2.5-flash` 0.30 in / 2.50 out).
- Access JWT: HS256, 15 min, ≥32-byte secret. Refresh: 30 d, sha256-hashed at rest, rotated per use, reuse revokes the family, cookie `refresh_token` (HttpOnly, SameSite=Lax, Path=/api/auth).
- SSE events: `meta` (`{"type":"model_start"}` / `{"type":"model_end","inputTokens":N,"outputTokens":N}`), `delta` (`{"text":"…"}`), `done` (`{"messageId","inputTokens","outputTokens","requests","costUsd"}`), `error` (`{"stage","message"}`).
- Trim window: `golem.TrimHistory(40)`; usage guardrail `golem.UsageLimit{Requests: 12, TotalTokens: 100_000}`.

## File Structure

```text
backend/
├── cmd/server/main.go                  # wiring + graceful shutdown (Task 12)
├── internal/config/config.go           # env config + dev .env loader (Task 1)
├── internal/cost/rates.go              # rate card + micro-USD math (Task 4)
├── internal/auth/{password,jwt,refresh,service}.go  # primitives + Service (Tasks 5–6)
├── internal/storage/{db,migrations,users,refresh_tokens,conversations,messages,usage}.go
├── internal/storage/migrations/00001_init.sql
├── internal/chat/agent.go              # shared golem agent + Sink (Task 9)
├── internal/httpapi/{server,middleware,auth,conversations,chat,usage,sse}.go
├── go.mod
deploy/docker-compose.yml              # Task 2
Makefile, .env.example, README.md       # Tasks 2, 12, 16
frontend/                              # Tasks 13–16 (Vite scaffold)
└── src/{lib,stores,features,pages,components/{ui,ai}}
```

---

### Task 1: Backend scaffold, config, health endpoint

**Files:**
- Create: `backend/go.mod`, `backend/internal/config/config.go`, `backend/internal/config/config_test.go`, `backend/cmd/server/main.go`, `backend/internal/httpapi/server.go`, `backend/internal/httpapi/server_test.go`

**Interfaces:**
- Produces: `config.Config{Port, DatabaseURL, GeminiAPIKey, GeminiModel, GeminiBaseURL, JWTSecret, FrontendOrigin string; ChatInputRate, ChatOutputRate float64}`, `config.Load() (Config, error)`; `httpapi.NewServer(deps ServerDeps) http.Handler` (Task 1 version: routes only `GET /healthz`), `httpapi.ServerDeps{Cfg config.Config; Log *slog.Logger}`.

- [ ] **Step 1: Scaffold module and write the failing config test**

```bash
mkdir -p backend/internal/{config,httpapi} backend/cmd/server && cd backend
go mod init github.com/abubakarsiddik31/golem-chatbot
go get github.com/abubakarsiddik31/golem@v0.7.1
```

`backend/internal/config/config_test.go`:

```go
package config

import "testing"

func TestLoadRequiresSecrets(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/gc")
	t.Setenv("JWT_SECRET", "short")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for short JWT secret")
	}
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("GEMINI_API_KEY", "k")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != "8080" || cfg.GeminiModel != "gemini-2.5-flash" {
		t.Fatalf("defaults not applied: %+v", cfg)
	}
	if cfg.ChatInputRate != 0.30 || cfg.ChatOutputRate != 2.50 {
		t.Fatalf("rate defaults: %+v", cfg)
	}
	t.Setenv("CHAT_INPUT_USD_PER_MTOK", "0,5")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for malformed rate override")
	}
}
```

- [ ] **Step 2: Run to see it fail** — `cd backend && go test ./internal/config/` → FAIL (package has no non-test files / Load undefined).
- [ ] **Step 3: Implement `backend/internal/config/config.go`**

```go
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config carries every runtime setting. Values come from the environment;
// a .env file in the working directory is loaded first, dev convenience only.
type Config struct {
	Port           string
	DatabaseURL    string
	GeminiAPIKey   string
	GeminiModel    string
	GeminiBaseURL  string
	JWTSecret      string
	FrontendOrigin string
	ChatInputRate  float64 // USD per 1M input tokens
	ChatOutputRate float64 // USD per 1M output tokens
}

func Load() (Config, error) {
	loadDotenv(".env")
	inputRate, err := envFloat("CHAT_INPUT_USD_PER_MTOK", 0.30)
	if err != nil {
		return Config{}, err
	}
	outputRate, err := envFloat("CHAT_OUTPUT_USD_PER_MTOK", 2.50)
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		Port:           env("APP_PORT", "8080"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		GeminiAPIKey:   os.Getenv("GEMINI_API_KEY"),
		GeminiModel:    env("GEMINI_MODEL", "gemini-2.5-flash"),
		GeminiBaseURL:  os.Getenv("GEMINI_BASE_URL"),
		JWTSecret:      os.Getenv("JWT_SECRET"),
		FrontendOrigin: env("FRONTEND_ORIGIN", "http://localhost:5173"),
		ChatInputRate:  inputRate,
		ChatOutputRate: outputRate,
	}
	var errs []error
	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if cfg.GeminiAPIKey == "" {
		errs = append(errs, errors.New("GEMINI_API_KEY is required"))
	}
	if len(cfg.JWTSecret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET must be at least 32 bytes"))
	}
	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return cfg, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envFloat(key string, def float64) (float64, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %q", key, v)
	}
	return f, nil
}

// loadDotenv applies KEY=VALUE lines that are not already in the
// environment. Existing environment always wins.
func loadDotenv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}
```

- [ ] **Step 4: Health endpoint.** `backend/internal/httpapi/server.go`:

```go
package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
)

// ServerDeps carries the wired collaborators. Fields join as later tasks
// land (auth service, stores, agent).
type ServerDeps struct {
	Cfg config.Config
	Log *slog.Logger
}

func NewServer(deps ServerDeps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return logRequests(deps.Log, mux)
}
```

`backend/internal/httpapi/middleware.go`:

```go
package httpapi

import (
	"log/slog"
	"net/http"
	"time"
)

func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Info("request", "method", r.Method, "path", r.URL.Path, "dur", time.Since(start).String())
	})
}
```

`backend/cmd/server/main.go` (minimal now; wiring grows in later tasks):

```go
package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/httpapi"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: httpapi.NewServer(httpapi.ServerDeps{Cfg: cfg, Log: log})}
	log.Info("listening", "port", cfg.Port)
	if err := srv.ListenAndServe(); err != nil {
		log.Error("server", "err", err)
		os.Exit(1)
	}
}
```

`backend/internal/httpapi/server_test.go`:

```go
package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
)

func TestHealthz(t *testing.T) {
	h := NewServer(ServerDeps{Cfg: config.Config{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("healthz: %d %q", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 5: Run tests** — `go test ./...` → PASS. Then `go fmt ./... && go build ./... && go vet ./...`.
- [ ] **Step 6: Commit**

```bash
git add backend && git commit -m "feat(backend): module scaffold, env config, health endpoint"
```

---

### Task 2: Docker Compose infra, Makefile, .env.example

**Files:**
- Create: `deploy/docker-compose.yml`, `Makefile`, `.env.example`, `.gitignore`

**Interfaces:**
- Produces: `docker compose up -d` (postgres only), `docker compose --profile rag up -d` (adds weaviate+minio in Phase 2); `make check`, `make up`, `make backend`, `make frontend`.

- [ ] **Step 1: `deploy/docker-compose.yml`**

```yaml
services:
  postgres:
    image: postgres:17-alpine
    environment:
      POSTGRES_DB: ${POSTGRES_DB:-golem_chatbot}
      POSTGRES_USER: ${POSTGRES_USER:-golem}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-golem}
    ports:
      - "5433:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER:-golem}"]
      interval: 5s
      timeout: 3s
      retries: 10

  weaviate:
    image: semitechnologies/weaviate:1.28.4
    profiles: [rag]
    environment:
      PERSISTENCE_DATA_PATH: /var/lib/weaviate
      DEFAULT_VECTORIZER_MODULE: none
      ENABLE_MODULES: ""
      QUERY_DEFAULTS_LIMIT: 20
      CLUSTER_HOSTNAME: node1
    ports:
      - "8080:8080"
      - "50051:50051"
    volumes:
      - weaviate_data:/var/lib/weaviate

  minio:
    image: minio/minio:latest
    profiles: [rag]
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: ${MINIO_ROOT_USER:-golem}
      MINIO_ROOT_PASSWORD: ${MINIO_ROOT_PASSWORD:-golem1234}
    ports:
      - "9000:9000"
      - "9001:9001"
    volumes:
      - minio_data:/data

  minio-init:
    image: minio/mc:latest
    profiles: [rag]
    depends_on: [minio]
    entrypoint: >
      /bin/sh -c "
      mc alias set local http://minio:9000 ${MINIO_ROOT_USER:-golem} ${MINIO_ROOT_PASSWORD:-golem1234} &&
      mc mb --ignore-existing local/golem-chatbot-documents"

volumes:
  pgdata:
  weaviate_data:
  minio_data:
```

- [ ] **Step 2: `Makefile`**

```makefile
.PHONY: up down backend frontend check fe-check

up:
	docker compose -f deploy/docker-compose.yml up -d
down:
	docker compose -f deploy/docker-compose.yml down
backend:
	cd backend && go run ./cmd/server
frontend:
	cd frontend && npm run dev
check:
	cd backend && go fmt ./... && go build ./... && go vet ./... && go test ./...
fe-check:
	cd frontend && npm run check
```

- [ ] **Step 3: `.env.example` and `.gitignore`**

`.env.example`:

```bash
APP_PORT=8080
DATABASE_URL=postgres://golem:golem@localhost:5433/golem_chatbot
GEMINI_API_KEY=your-key-here
GEMINI_MODEL=gemini-2.5-flash
# GEMINI_BASE_URL=            # optional proxy
JWT_SECRET=0123456789abcdef0123456789abcdef
FRONTEND_ORIGIN=http://localhost:5173
CHAT_INPUT_USD_PER_MTOK=0.30
CHAT_OUTPUT_USD_PER_MTOK=2.50
```

`.gitignore`:

```gitignore
.env
node_modules/
backend/bin/
dist/
.superpowers/
```

- [ ] **Step 4: Verify** — `docker compose -f deploy/docker-compose.yml config` exits 0; `cp .env.example .env` and fill in a real `GEMINI_API_KEY` (local only, never committed).
- [ ] **Step 5: Commit**

```bash
git add deploy Makefile .env.example .gitignore && git commit -m "chore: compose infra (postgres core, rag profile), Makefile, env template"
```

---

### Task 3: Storage foundation — open, migrate, schema

**Files:**
- Create: `backend/internal/storage/db.go`, `backend/internal/storage/db_test.go`, `backend/internal/storage/migrations/00001_init.sql`
- Modify: `backend/go.mod` (deps: pgx, goose)

**Interfaces:**
- Produces: `storage.Open(ctx, url) (*pgxpool.Pool, error)`, `storage.Migrate(ctx, pool) error`; tables `users`, `refresh_tokens`, `conversations`, `messages`, `usage_events` (schema below — later tasks rely on these columns).

- [ ] **Step 1: Deps**

```bash
cd backend
go get github.com/jackc/pgx/v5 github.com/pressly/goose/v3
```

- [ ] **Step 2: Migration SQL** — `backend/internal/storage/migrations/00001_init.sql`:

```sql
-- +goose Up
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         citext NOT NULL UNIQUE,
    password_hash text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE refresh_tokens (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  text NOT NULL UNIQUE,
    expires_at  timestamptz NOT NULL,
    revoked_at  timestamptz,
    replaced_by text,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);

CREATE TABLE conversations (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title      text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_conversations_user ON conversations(user_id, updated_at DESC);

CREATE TABLE messages (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role            text NOT NULL,
    content         text NOT NULL,
    data            jsonb NOT NULL,
    input_tokens    int NOT NULL DEFAULT 0,
    output_tokens   int NOT NULL DEFAULT 0,
    requests        int NOT NULL DEFAULT 0,
    cost_micro_usd  bigint NOT NULL DEFAULT 0,
    truncated       boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_messages_conversation ON messages(conversation_id, created_at, id);

CREATE TABLE usage_events (
    id              bigserial PRIMARY KEY,
    user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind            text NOT NULL,
    model           text NOT NULL,
    conversation_id uuid REFERENCES conversations(id) ON DELETE SET NULL,
    input_tokens    int NOT NULL DEFAULT 0,
    output_tokens   int NOT NULL DEFAULT 0,
    requests        int NOT NULL DEFAULT 0,
    estimated       boolean NOT NULL DEFAULT false,
    cost_micro_usd  bigint NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_usage_events_user_time ON usage_events(user_id, created_at);

-- +goose Down
DROP TABLE usage_events, messages, conversations, refresh_tokens, users;
DROP EXTENSION IF EXISTS citext;
```

- [ ] **Step 3: `backend/internal/storage/db.go`**

```go
package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// Migrate applies embedded goose migrations; idempotent.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	dsn := pool.Config().ConnConfig.ConnString()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open migration db: %w", err)
	}
	defer db.Close()
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Test (skips without a database)** — `backend/internal/storage/db_test.go`:

```go
package storage

import (
	"context"
	"os"
	"testing"
)

func TestMigrate(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := Open(ctx, url)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.tables WHERE table_name = 'messages'`,
	).Scan(&n); err != nil || n != 1 {
		t.Fatalf("messages table missing: n=%d err=%v", n, err)
	}
}
```

- [ ] **Step 5: Run** — `go test ./internal/storage/ -v` → PASS with `SKIP` (offline). With `TEST_DATABASE_URL=postgres://golem:golem@localhost:5433/golem_chatbot?sslmode=disable` and `make up`, it PASSes against Postgres. Then the full gate: `go fmt ./... && go build ./... && go vet ./... && go test ./...`.
- [ ] **Step 6: Commit**

```bash
git add backend && git commit -m "feat(storage): pgx pool, goose embedded migrations, phase-1 schema"
```

---

### Task 4: Cost rate card

**Files:**
- Create: `backend/internal/cost/rates.go`, `backend/internal/cost/rates_test.go`

**Interfaces:**
- Produces: `cost.Rates{ChatInputPerM, ChatOutputPerM float64}`, `(r Rates) ChatCostMicros(inputTokens, outputTokens int) int64` — integer micro-USD.

- [ ] **Step 1: Failing test** — `backend/internal/cost/rates_test.go`:

```go
package cost

import "testing"

func TestChatCostMicros(t *testing.T) {
	r := Rates{ChatInputPerM: 0.30, ChatOutputPerM: 2.50}
	tests := []struct {
		name  string
		in    int
		out   int
		want  int64
	}{
		{"zero", 0, 0, 0},
		{"one million in", 1_000_000, 0, 300_000},   // $0.30
		{"one million out", 0, 1_000_000, 2_500_000}, // $2.50
		{"mixed", 1000, 500, 300 + 1250},             // 1000*0.3 + 500*2.5 micros
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.ChatCostMicros(tt.in, tt.out); got != tt.want {
				t.Fatalf("ChatCostMicros(%d,%d) = %d, want %d", tt.in, tt.out, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run to see it fail** — `go test ./internal/cost/` → FAIL (Rates undefined).
- [ ] **Step 3: Implement** — `backend/internal/cost/rates.go`:

```go
package cost

import "math"

// Rates holds USD prices per 1M tokens. Values come from config; the
// ledger stores raw tokens so historic rows reprice if rates change.
type Rates struct {
	ChatInputPerM  float64
	ChatOutputPerM float64
}

// ChatCostMicros returns the cost of one chat run in integer micro-USD.
// Per token, one micro-USD equals the rate itself (USD/1M tokens).
func (r Rates) ChatCostMicros(inputTokens, outputTokens int) int64 {
	return int64(math.Round(float64(inputTokens)*r.ChatInputPerM + float64(outputTokens)*r.ChatOutputPerM))
}
```

- [ ] **Step 4: Run** — `go test ./internal/cost/` → PASS. Gate + commit:

```bash
go fmt ./... && go build ./... && go vet ./... && go test ./...
git add backend && git commit -m "feat(cost): rate card with micro-USD chat cost math"
```

---

### Task 5: Auth primitives — password, JWT, refresh tokens

**Files:**
- Create: `backend/internal/auth/password.go`, `backend/internal/auth/jwt.go`, `backend/internal/auth/refresh.go`, plus tests `password_test.go`, `jwt_test.go`, `refresh_test.go`
- Modify: `backend/go.mod` (`golang-jwt/jwt/v5`, `golang.org/x/crypto`, `google/uuid`)

**Interfaces:**
- Produces: `auth.HashPassword(password) (string, error)`, `auth.CheckPassword(hash, password) error`; `auth.NewTokenMaker(secret) (*TokenMaker, error)`, `(*TokenMaker) Issue(userID string, now time.Time) (token string, exp time.Time, err error)`, `(*TokenMaker) Verify(token string) (userID string, err error)`; `auth.NewRefreshToken() (token, hash string, err error)`, `auth.HashRefreshToken(token string) string`, `const auth.RefreshTTL = 30 * 24 * time.Hour`.

- [ ] **Step 1: Deps**

```bash
go get github.com/golang-jwt/jwt/v5 golang.org/x/crypto google/uuid
```

- [ ] **Step 2: Failing tests**

`backend/internal/auth/jwt_test.go`:

```go
package auth

import (
	"testing"
	"time"
)

func TestTokenRoundTrip(t *testing.T) {
	tm, err := NewTokenMaker("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewTokenMaker: %v", err)
	}
	now := time.Now()
	token, exp, err := tm.Issue("u-1", now)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if !exp.After(now.Add(14 * time.Minute)) {
		t.Fatalf("exp too early: %v", exp)
	}
	got, err := tm.Verify(token)
	if err != nil || got != "u-1" {
		t.Fatalf("Verify = %q, %v", got, err)
	}
}

func TestTokenSecretTooShort(t *testing.T) {
	if _, err := NewTokenMaker("short"); err == nil {
		t.Fatal("expected error for short secret")
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	tm, _ := NewTokenMaker("0123456789abcdef0123456789abcdef")
	token, _, _ := tm.Issue("u-1", time.Now().Add(-16*time.Minute))
	if _, err := tm.Verify(token); err == nil {
		t.Fatal("expected expired token to fail")
	}
}
```

`backend/internal/auth/refresh_test.go`:

```go
package auth

import (
	"strings"
	"testing"
)

func TestNewRefreshToken(t *testing.T) {
	a, ha, err := NewRefreshToken()
	if err != nil {
		t.Fatalf("NewRefreshToken: %v", err)
	}
	b, hb, _ := NewRefreshToken()
	if a == b || ha == hb {
		t.Fatal("tokens must be unique")
	}
	if strings.ContainsAny(a, "+/=") {
		t.Fatalf("token must be URL-safe: %q", a)
	}
	if HashRefreshToken(a) != ha {
		t.Fatal("hash mismatch")
	}
}
```

`backend/internal/auth/password_test.go`:

```go
package auth

import "testing"

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("hunter2!")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := CheckPassword(hash, "hunter2!"); err != nil {
		t.Fatalf("CheckPassword: %v", err)
	}
	if err := CheckPassword(hash, "wrong"); err == nil {
		t.Fatal("expected wrong password to fail")
	}
}
```

- [ ] **Step 3: Run to see them fail** — `go test ./internal/auth/` → FAIL (undefined).
- [ ] **Step 4: Implement**

`backend/internal/auth/password.go`:

```go
package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(h), nil
}

func CheckPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
```

`backend/internal/auth/jwt.go`:

```go
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const accessTTL = 15 * time.Minute

type TokenMaker struct {
	secret []byte
}

func NewTokenMaker(secret string) (*TokenMaker, error) {
	if len(secret) < 32 {
		return nil, errors.New("auth: jwt secret must be at least 32 bytes")
	}
	return &TokenMaker{secret: []byte(secret)}, nil
}

// Issue returns a signed HS256 access token and its expiry.
func (t *TokenMaker) Issue(userID string, now time.Time) (string, time.Time, error) {
	exp := now.Add(accessTTL)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"iat": now.Unix(),
		"exp": exp.Unix(),
	})
	signed, err := token.SignedString(t.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return signed, exp, nil
}

func (t *TokenMaker) Verify(token string) (string, error) {
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return t.secret, nil
	})
	if err != nil || !parsed.Valid {
		return "", fmt.Errorf("invalid token: %w", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid claims")
	}
	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		return "", errors.New("missing subject")
	}
	return sub, nil
}
```

`backend/internal/auth/refresh.go`:

```go
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"
)

const RefreshTTL = 30 * 24 * time.Hour

// NewRefreshToken returns (opaque token, sha256 hash). Only the hash is
// stored; the token lives in the browser cookie.
func NewRefreshToken() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate refresh token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	return token, HashRefreshToken(token), nil
}

func HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
```

- [ ] **Step 5: Run** — `go test ./internal/auth/` → PASS. Gate + commit:

```bash
go fmt ./... && go build ./... && go vet ./... && go test ./...
git add backend && git commit -m "feat(auth): bcrypt passwords, HS256 access tokens, hashed refresh tokens"
```

---

### Task 6: Auth service — register, login, refresh rotation, reuse detection

**Files:**
- Create: `backend/internal/auth/service.go`, `backend/internal/auth/service_test.go`

**Interfaces:**
- Produces:
```go
type UserRecord struct { ID, Email, PasswordHash string }
type RefreshRecord struct { UserID string; ExpiresAt time.Time; Revoked bool }
type UserStore interface {
	Create(ctx context.Context, email, passwordHash string) (UserRecord, error)
	ByEmail(ctx context.Context, email string) (UserRecord, error)
}
type RefreshStore interface {
	Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	Get(ctx context.Context, tokenHash string) (RefreshRecord, error)
	Rotate(ctx context.Context, oldHash, newHash string, expiresAt time.Time) error
	RevokeFamily(ctx context.Context, userID string) error
}
func NewService(users UserStore, refresh RefreshStore, secret string) (*Service, error)
type AuthResult struct { AccessToken string; AccessExpiresAt time.Time; RefreshToken string; RefreshExpiresAt time.Time; User UserRecord }
func (*Service) Register(ctx, email, password string) (AuthResult, error)   // ErrEmailTaken, ErrWeakPassword
func (*Service) Login(ctx, email, password string) (AuthResult, error)      // ErrInvalidCredentials
func (*Service) Refresh(ctx, refreshToken string) (AuthResult, error)       // ErrInvalidRefresh; reuse revokes family
func (*Service) Logout(ctx, refreshToken string) error
var ErrEmailTaken, ErrWeakPassword, ErrInvalidCredentials, ErrInvalidRefresh error
```
- Consumes: Task 5 primitives.

- [ ] **Step 1: Failing tests** — `backend/internal/auth/service_test.go`:

```go
package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeUsers struct {
	byEmail map[string]UserRecord
	created []UserRecord
}

func (f *fakeUsers) Create(_ context.Context, email, hash string) (UserRecord, error) {
	if _, ok := f.byEmail[email]; ok {
		return UserRecord{}, ErrEmailTaken
	}
	rec := UserRecord{ID: "u-" + email, Email: email, PasswordHash: hash}
	f.byEmail[email] = rec
	f.created = append(f.created, rec)
	return rec, nil
}

func (f *fakeUsers) ByEmail(_ context.Context, email string) (UserRecord, error) {
	rec, ok := f.byEmail[email]
	if !ok {
		return UserRecord{}, ErrInvalidCredentials
	}
	return rec, nil
}

type fakeRefresh struct {
	tokens map[string]RefreshRecord
}

func (f *fakeRefresh) Create(_ context.Context, _, hash string, exp time.Time) error {
	f.tokens[hash] = RefreshRecord{UserID: "current", ExpiresAt: exp}
	return nil
}
func (f *fakeRefresh) Get(_ context.Context, hash string) (RefreshRecord, error) {
	rec, ok := f.tokens[hash]
	if !ok {
		return RefreshRecord{}, ErrInvalidRefresh
	}
	return rec, nil
}
func (f *fakeRefresh) Rotate(_ context.Context, oldHash, newHash string, exp time.Time) error {
	old := f.tokens[oldHash]
	old.Revoked = true
	f.tokens[oldHash] = old
	f.tokens[newHash] = RefreshRecord{UserID: old.UserID, ExpiresAt: exp}
	return nil
}
func (f *fakeRefresh) RevokeFamily(_ context.Context, _ string) error {
	for h, rec := range f.tokens {
		rec.Revoked = true
		f.tokens[h] = rec
	}
	return nil
}

func newTestService() (*Service, *fakeRefresh) {
	users := &fakeUsers{byEmail: map[string]UserRecord{}}
	refresh := &fakeRefresh{tokens: map[string]RefreshRecord{}}
	svc, err := NewService(users, refresh, "0123456789abcdef0123456789abcdef")
	if err != nil {
		panic(err)
	}
	return svc, refresh
}

func TestRegisterLoginFlow(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	res, err := svc.Register(ctx, "a@b.co", "longenough1")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.AccessToken == "" || res.RefreshToken == "" {
		t.Fatal("expected both tokens")
	}
	if _, err := svc.Register(ctx, "a@b.co", "longenough1"); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
	if _, err := svc.Login(ctx, "a@b.co", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
	if _, err := svc.Login(ctx, "a@b.co", "longenough1"); err != nil {
		t.Fatalf("Login: %v", err)
	}
}

func TestRefreshRotationAndReuseDetection(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	reg, err := svc.Register(ctx, "a@b.co", "longenough1")
	if err != nil {
		t.Fatal(err)
	}

	second, err := svc.Refresh(ctx, reg.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// Presenting the rotated-out token again is reuse: the whole family dies.
	if _, err := svc.Refresh(ctx, reg.RefreshToken); !errors.Is(err, ErrInvalidRefresh) {
		t.Fatalf("expected ErrInvalidRefresh on reuse, got %v", err)
	}
	if _, err := svc.Refresh(ctx, second.RefreshToken); !errors.Is(err, ErrInvalidRefresh) {
		t.Fatalf("family should be revoked after reuse, got %v", err)
	}
}

func TestWeakPasswordRejected(t *testing.T) {
	svc, _ := newTestService()
	if _, err := svc.Register(context.Background(), "a@b.co", "short"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("expected ErrWeakPassword, got %v", err)
	}
}
```

- [ ] **Step 2: Run to see it fail** — `go test ./internal/auth/ -run TestRegister` → FAIL.
- [ ] **Step 3: Implement** — `backend/internal/auth/service.go`:

```go
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrWeakPassword       = errors.New("password must be at least 10 characters")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidRefresh     = errors.New("invalid refresh token")
)

type UserRecord struct {
	ID           string
	Email        string
	PasswordHash string
}

type RefreshRecord struct {
	UserID    string
	ExpiresAt time.Time
	Revoked   bool
}

type UserStore interface {
	Create(ctx context.Context, email, passwordHash string) (UserRecord, error)
	ByEmail(ctx context.Context, email string) (UserRecord, error)
}

type RefreshStore interface {
	Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	Get(ctx context.Context, tokenHash string) (RefreshRecord, error)
	Rotate(ctx context.Context, oldHash, newHash string, expiresAt time.Time) error
	RevokeFamily(ctx context.Context, userID string) error
}

type AuthResult struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
	User             UserRecord
}

type Service struct {
	users   UserStore
	refresh RefreshStore
	tokens  *TokenMaker
}

func NewService(users UserStore, refresh RefreshStore, secret string) (*Service, error) {
	tm, err := NewTokenMaker(secret)
	if err != nil {
		return nil, err
	}
	return &Service{users: users, refresh: refresh, tokens: tm}, nil
}

func (s *Service) issue(ctx context.Context, user UserRecord) (AuthResult, error) {
	access, exp, err := s.tokens.Issue(user.ID, time.Now())
	if err != nil {
		return AuthResult{}, err
	}
	refresh, refreshHash, err := NewRefreshToken()
	if err != nil {
		return AuthResult{}, err
	}
	refreshExp := time.Now().Add(RefreshTTL)
	if err := s.refresh.Create(ctx, user.ID, refreshHash, refreshExp); err != nil {
		return AuthResult{}, fmt.Errorf("store refresh token: %w", err)
	}
	return AuthResult{
		AccessToken: access, AccessExpiresAt: exp,
		RefreshToken: refresh, RefreshExpiresAt: refreshExp,
		User: UserRecord{ID: user.ID, Email: user.Email},
	}, nil
}

func (s *Service) Register(ctx context.Context, email, password string) (AuthResult, error) {
	if len(password) < 10 {
		return AuthResult{}, ErrWeakPassword
	}
	hash, err := HashPassword(password)
	if err != nil {
		return AuthResult{}, err
	}
	user, err := s.users.Create(ctx, email, hash)
	if err != nil {
		return AuthResult{}, err
	}
	return s.issue(ctx, user)
}

func (s *Service) Login(ctx context.Context, email, password string) (AuthResult, error) {
	user, err := s.users.ByEmail(ctx, email)
	if err != nil {
		return AuthResult{}, ErrInvalidCredentials
	}
	if err := CheckPassword(user.PasswordHash, password); err != nil {
		return AuthResult{}, ErrInvalidCredentials
	}
	return s.issue(ctx, user)
}

// Refresh rotates the presented token. A rotated-out or revoked token is
// reuse: the whole family is revoked and the caller gets ErrInvalidRefresh.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (AuthResult, error) {
	hash := HashRefreshToken(refreshToken)
	rec, err := s.refresh.Get(ctx, hash)
	if err != nil {
		return AuthResult{}, ErrInvalidRefresh
	}
	if rec.Revoked || time.Now().After(rec.ExpiresAt) {
		_ = s.refresh.RevokeFamily(ctx, rec.UserID)
		return AuthResult{}, ErrInvalidRefresh
	}
	user, err := s.users.ByIDForRefresh(ctx, rec.UserID)
	if err != nil {
		return AuthResult{}, ErrInvalidRefresh
	}
	newToken, newHash, err := NewRefreshToken()
	if err != nil {
		return AuthResult{}, err
	}
	newExp := time.Now().Add(RefreshTTL)
	if err := s.refresh.Rotate(ctx, hash, newHash, newExp); err != nil {
		return AuthResult{}, fmt.Errorf("rotate refresh token: %w", err)
	}
	access, exp, err := s.tokens.Issue(user.ID, time.Now())
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{
		AccessToken: access, AccessExpiresAt: exp,
		RefreshToken: newToken, RefreshExpiresAt: newExp,
		User: UserRecord{ID: user.ID, Email: user.Email},
	}, nil
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	rec, err := s.refresh.Get(ctx, HashRefreshToken(refreshToken))
	if err != nil {
		return nil // already invalid; logout is best-effort
	}
	return s.refresh.RevokeFamily(ctx, rec.UserID)
}
```

Note the interface drift: `Service.Refresh` needs `ByIDForRefresh`. Add to `UserStore`:

```go
type UserStore interface {
	Create(ctx context.Context, email, passwordHash string) (UserRecord, error)
	ByEmail(ctx context.Context, email string) (UserRecord, error)
	ByID(ctx context.Context, id string) (UserRecord, error)
}
```

and call `s.users.ByID(ctx, rec.UserID)`. Update the fake accordingly (`func (f *fakeUsers) ByID(_ context.Context, id string) (UserRecord, error)` — return the first created record whose ID matches, or `ErrInvalidRefresh`).

- [ ] **Step 4: Run** — `go test ./internal/auth/ -v` → PASS (all five tests). Gate + commit:

```bash
go fmt ./... && go build ./... && go vet ./... && go test ./...
git add backend && git commit -m "feat(auth): service with register, login, refresh rotation and reuse detection"
```

---

### Task 7: Storage repos for auth + conversations + messages + usage

**Files:**
- Create: `backend/internal/storage/users.go`, `refresh_tokens.go`, `conversations.go`, `messages.go`, `usage.go`, `errors.go`

**Interfaces:**
- Produces (satisfying Task 6 interfaces plus chat/usage needs):
```go
type User struct { ID, Email, PasswordHash string } // storage shape; auth.UserRecord conversion
func (*Users) Create(ctx, email, hash string) (auth.UserRecord, error)   // ErrEmailTaken on 23505
func (*Users) ByEmail(ctx, email string) (auth.UserRecord, error)        // auth.ErrInvalidCredentials on no rows
func (*Users) ByID(ctx, id string) (auth.UserRecord, error)              // auth.ErrInvalidRefresh on no rows
func NewRefreshTokens(pool) *RefreshTokens  // implements auth.RefreshStore
type Conversation struct { ID, UserID, Title string; CreatedAt, UpdatedAt time.Time }
func NewConversations(pool) *Conversations
func (*Conversations) Create(ctx, userID, title string) (Conversation, error)
func (*Conversations) List(ctx, userID string) ([]Conversation, error)
func (*Conversations) ByID(ctx, id, userID string) (Conversation, error)  // storage.ErrNotFound
func (*Conversations) SetTitle(ctx, id, userID, title string) error
func (*Conversations) Touch(ctx, id string) error
func (*Conversations) Delete(ctx, id, userID string) error
func (*Conversations) CountMessages(ctx, id, userID string) (int, error)
type Message struct { ID, ConversationID, UserID, Role, Content string; Data []byte; InputTokens, OutputTokens, Requests int; CostMicros int64; Truncated bool; CreatedAt time.Time }
func NewMessages(pool) *Messages
func (*Messages) Add(ctx, Message) error
func (*Messages) ForConversation(ctx, convID, userID string) ([]Message, error) // ordered
func NewUsage(pool) *Usage
func (*Usage) Add(ctx, Event) error // Event{UserID, Kind, Model, ConversationID *string, InputTokens, OutputTokens, Requests int, Estimated bool, CostMicros int64}
func (*Usage) Summary(ctx, userID string, days int) (Summary, error)
type Summary struct { Totals []KindModelRow; Daily []DailyRow }
type KindModelRow struct { Kind, Model string; InputTokens, OutputTokens, Requests int; CostMicros int64 }
type DailyRow struct { Day time.Time; InputTokens, OutputTokens int; CostMicros int64 }
var ErrNotFound = errors.New("storage: not found")
```

- [ ] **Step 1: Implement repos.** Keep each file small; queries are plain pgx. `errors.go`:

```go
package storage

import "errors"

var ErrNotFound = errors.New("storage: not found")
```

`users.go`:

```go
package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Users struct{ pool *pgxpool.Pool }

func NewUsers(pool *pgxpool.Pool) *Users { return &Users{pool: pool} }

func (u *Users) Create(ctx context.Context, email, passwordHash string) (auth.UserRecord, error) {
	row := u.pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash) VALUES ($1, $2)
		 RETURNING id, email::text`,
		email, passwordHash)
	var rec auth.UserRecord
	if err := row.Scan(&rec.ID, &rec.Email); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return auth.UserRecord{}, auth.ErrEmailTaken
		}
		return auth.UserRecord{}, fmt.Errorf("create user: %w", err)
	}
	return rec, nil
}

func (u *Users) ByEmail(ctx context.Context, email string) (auth.UserRecord, error) {
	row := u.pool.QueryRow(ctx,
		`SELECT id, email::text, password_hash FROM users WHERE email = $1`, email)
	var rec auth.UserRecord
	if err := row.Scan(&rec.ID, &rec.Email, &rec.PasswordHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.UserRecord{}, auth.ErrInvalidCredentials
		}
		return auth.UserRecord{}, fmt.Errorf("user by email: %w", err)
	}
	return rec, nil
}

func (u *Users) ByID(ctx context.Context, id string) (auth.UserRecord, error) {
	row := u.pool.QueryRow(ctx, `SELECT id, email::text, password_hash FROM users WHERE id = $1`, id)
	var rec auth.UserRecord
	if err := row.Scan(&rec.ID, &rec.Email, &rec.PasswordHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.UserRecord{}, auth.ErrInvalidRefresh
		}
		return auth.UserRecord{}, fmt.Errorf("user by id: %w", err)
	}
	return rec, nil
}
```

`refresh_tokens.go`:

```go
package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RefreshTokens struct{ pool *pgxpool.Pool }

func NewRefreshTokens(pool *pgxpool.Pool) *RefreshTokens { return &RefreshTokens{pool: pool} }

func (r *RefreshTokens) Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("insert refresh token: %w", err)
	}
	return nil
}

func (r *RefreshTokens) Get(ctx context.Context, tokenHash string) (auth.RefreshRecord, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT user_id, expires_at, revoked_at, replaced_by FROM refresh_tokens WHERE token_hash = $1`,
		tokenHash)
	var (
		rec                                auth.RefreshRecord
		revokedAt                          *time.Time
		replacedBy                         *string
	)
	if err := row.Scan(&rec.UserID, &rec.ExpiresAt, &revokedAt, &replacedBy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.RefreshRecord{}, auth.ErrInvalidRefresh
		}
		return auth.RefreshRecord{}, fmt.Errorf("get refresh token: %w", err)
	}
	rec.Revoked = revokedAt != nil || replacedBy != nil
	return rec, nil
}

func (r *RefreshTokens) Rotate(ctx context.Context, oldHash, newHash string, expiresAt time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin rotate: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now(), replaced_by = $2 WHERE token_hash = $1`,
		oldHash, newHash); err != nil {
		return fmt.Errorf("revoke old token: %w", err)
	}
	var userID string
	if err := tx.QueryRow(ctx,
		`SELECT user_id FROM refresh_tokens WHERE token_hash = $1`, oldHash).Scan(&userID); err != nil {
		return fmt.Errorf("old token missing: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, newHash, expiresAt); err != nil {
		return fmt.Errorf("insert rotated token: %w", err)
	}
	return tx.Commit(ctx)
}

func (r *RefreshTokens) RevokeFamily(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	if err != nil {
		return fmt.Errorf("revoke family: %w", err)
	}
	return nil
}
```

`conversations.go`:

```go
package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Conversation struct {
	ID        string
	UserID    string
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Conversations struct{ pool *pgxpool.Pool }

func NewConversations(pool *pgxpool.Pool) *Conversations { return &Conversations{pool: pool} }

func (c *Conversations) Create(ctx context.Context, userID, title string) (Conversation, error) {
	row := c.pool.QueryRow(ctx,
		`INSERT INTO conversations (user_id, title) VALUES ($1, $2)
		 RETURNING id, user_id::text, title, created_at, updated_at`,
		userID, title)
	var conv Conversation
	err := row.Scan(&conv.ID, &conv.UserID, &conv.Title, &conv.CreatedAt, &conv.UpdatedAt)
	if err != nil {
		return Conversation{}, fmt.Errorf("create conversation: %w", err)
	}
	return conv, nil
}

func (c *Conversations) List(ctx context.Context, userID string) ([]Conversation, error) {
	rows, err := c.pool.Query(ctx,
		`SELECT id, user_id::text, title, created_at, updated_at
		 FROM conversations WHERE user_id = $1 ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()
	var out []Conversation
	for rows.Next() {
		var conv Conversation
		if err := rows.Scan(&conv.ID, &conv.UserID, &conv.Title, &conv.CreatedAt, &conv.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, conv)
	}
	return out, rows.Err()
}

func (c *Conversations) ByID(ctx context.Context, id, userID string) (Conversation, error) {
	row := c.pool.QueryRow(ctx,
		`SELECT id, user_id::text, title, created_at, updated_at
		 FROM conversations WHERE id = $1 AND user_id = $2`, id, userID)
	var conv Conversation
	if err := row.Scan(&conv.ID, &conv.UserID, &conv.Title, &conv.CreatedAt, &conv.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Conversation{}, ErrNotFound
		}
		return Conversation{}, fmt.Errorf("conversation by id: %w", err)
	}
	return conv, nil
}

func (c *Conversations) SetTitle(ctx context.Context, id, userID, title string) error {
	_, err := c.pool.Exec(ctx,
		`UPDATE conversations SET title = $3 WHERE id = $1 AND user_id = $2`, id, userID, title)
	return err
}

func (c *Conversations) Touch(ctx context.Context, id string) error {
	_, err := c.pool.Exec(ctx, `UPDATE conversations SET updated_at = now() WHERE id = $1`, id)
	return err
}

func (c *Conversations) Delete(ctx context.Context, id, userID string) error {
	tag, err := c.pool.Exec(ctx,
		`DELETE FROM conversations WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (c *Conversations) CountMessages(ctx context.Context, id, userID string) (int, error) {
	var n int
	err := c.pool.QueryRow(ctx,
		`SELECT count(*) FROM messages WHERE conversation_id = $1 AND user_id = $2`, id, userID).Scan(&n)
	return n, err
}
```

`messages.go`:

```go
package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Message struct {
	ID              string
	ConversationID  string
	UserID          string
	Role            string
	Content         string
	Data            []byte // golem model.Message JSON (durable additive-only)
	InputTokens     int
	OutputTokens    int
	Requests        int
	CostMicros      int64
	Truncated       bool
	CreatedAt       time.Time
}

type Messages struct{ pool *pgxpool.Pool }

func NewMessages(pool *pgxpool.Pool) *Messages { return &Messages{pool: pool} }

func (m *Messages) Add(ctx context.Context, msg Message) error {
	_, err := m.pool.Exec(ctx,
		`INSERT INTO messages
		 (conversation_id, user_id, role, content, data, input_tokens, output_tokens, requests, cost_micro_usd, truncated)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		msg.ConversationID, msg.UserID, msg.Role, msg.Content, msg.Data,
		msg.InputTokens, msg.OutputTokens, msg.Requests, msg.CostMicros, msg.Truncated)
	if err != nil {
		return fmt.Errorf("add message: %w", err)
	}
	return nil
}

func (m *Messages) ForConversation(ctx context.Context, convID, userID string) ([]Message, error) {
	rows, err := m.pool.Query(ctx,
		`SELECT id, conversation_id::text, user_id::text, role, content, data,
		        input_tokens, output_tokens, requests, cost_micro_usd, truncated, created_at
		 FROM messages WHERE conversation_id = $1 AND user_id = $2
		 ORDER BY created_at, id`, convID, userID)
	if err != nil {
		return nil, fmt.Errorf("messages for conversation: %w", err)
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.ID, &msg.ConversationID, &msg.UserID, &msg.Role, &msg.Content,
			&msg.Data, &msg.InputTokens, &msg.OutputTokens, &msg.Requests, &msg.CostMicros,
			&msg.Truncated, &msg.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}
```

`usage.go`:

```go
package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type UsageEvent struct {
	UserID         string
	Kind           string
	Model          string
	ConversationID *string
	InputTokens    int
	OutputTokens   int
	Requests       int
	Estimated      bool
	CostMicros     int64
}

type Usage struct{ pool *pgxpool.Pool }

func NewUsage(pool *pgxpool.Pool) *Usage { return &Usage{pool: pool} }

func (u *Usage) Add(ctx context.Context, e UsageEvent) error {
	_, err := u.pool.Exec(ctx,
		`INSERT INTO usage_events
		 (user_id, kind, model, conversation_id, input_tokens, output_tokens, requests, estimated, cost_micro_usd)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		e.UserID, e.Kind, e.Model, e.ConversationID,
		e.InputTokens, e.OutputTokens, e.Requests, e.Estimated, e.CostMicros)
	if err != nil {
		return fmt.Errorf("add usage event: %w", err)
	}
	return nil
}

type KindModelRow struct {
	Kind         string
	Model        string
	InputTokens  int
	OutputTokens int
	Requests     int
	CostMicros   int64
}

type DailyRow struct {
	Day          time.Time
	InputTokens  int
	OutputTokens int
	CostMicros   int64
}

type Summary struct {
	Totals []KindModelRow
	Daily  []DailyRow
}

func (u *Usage) Summary(ctx context.Context, userID string, days int) (Summary, error) {
	var sum Summary
	rows, err := u.pool.Query(ctx,
		`SELECT kind, model, SUM(input_tokens)::int, SUM(output_tokens)::int,
		        COALESCE(SUM(requests),0)::int, SUM(cost_micro_usd)
		 FROM usage_events
		 WHERE user_id = $1 AND created_at > now() - make_interval(days => $2)
		 GROUP BY kind, model ORDER BY kind, model`, userID, days)
	if err != nil {
		return sum, fmt.Errorf("usage totals: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var r KindModelRow
		if err := rows.Scan(&r.Kind, &r.Model, &r.InputTokens, &r.OutputTokens, &r.Requests, &r.CostMicros); err != nil {
			return sum, err
		}
		sum.Totals = append(sum.Totals, r)
	}
	if err := rows.Err(); err != nil {
		return sum, err
	}
	rows2, err := u.pool.Query(ctx,
		`SELECT date_trunc('day', created_at)::date, COALESCE(SUM(input_tokens),0)::int,
		        COALESCE(SUM(output_tokens),0)::int, COALESCE(SUM(cost_micro_usd),0)
		 FROM usage_events
		 WHERE user_id = $1 AND created_at > now() - make_interval(days => $2)
		 GROUP BY 1 ORDER BY 1`, userID, days)
	if err != nil {
		return sum, fmt.Errorf("usage daily: %w", err)
	}
	defer rows2.Close()
	for rows2.Next() {
		var r DailyRow
		if err := rows2.Scan(&r.Day, &r.InputTokens, &r.OutputTokens, &r.CostMicros); err != nil {
			return sum, err
		}
		sum.Daily = append(sum.Daily, r)
	}
	return sum, rows2.Err()
}
```

- [ ] **Step 2: Gate + integration check.** `go fmt ./... && go build ./... && go vet ./... && go test ./...` → PASS. With `TEST_DATABASE_URL` set: write a quick throwaway check via `go run` or trust the auth handler tests in Task 8 to exercise these through the real pool (Task 8 uses fakes; the DB path is covered by the offline-skipped pattern — add one integration test file `storage_integration_test.go` with the same skip guard inserting a user + refresh token and exercising `Rotate` + `Get`).

`backend/internal/storage/storage_integration_test.go`:

```go
package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	pool, err := Open(context.Background(), url)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

func TestRefreshRotationPersistence(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := NewUsers(pool)
	user, err := users.Create(ctx, "rotate@test.dev", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	rt := NewRefreshTokens(pool)
	oldTok, oldHash, _ := auth.NewRefreshToken()
	if err := rt.Create(ctx, user.ID, oldHash, time.Now().Add(auth.RefreshTTL)); err != nil {
		t.Fatalf("create: %v", err)
	}
	newTok, newHash, _ := auth.NewRefreshToken()
	if err := rt.Rotate(ctx, oldHash, newHash, time.Now().Add(auth.RefreshTTL)); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if _, err := rt.Get(ctx, oldHash); err != nil {
		t.Fatalf("old hash unreadable: %v", err)
	}
	rec, err := rt.Get(ctx, oldHash)
	if err != nil || !rec.Revoked {
		t.Fatalf("old token should read back revoked: %+v %v", rec, err)
	}
	if rec2, err := rt.Get(ctx, auth.HashRefreshToken(newTok)); err != nil || rec2.Revoked {
		t.Fatalf("new token should be active: %+v %v", rec2, err)
	}
}
```

- [ ] **Step 3: Commit**

```bash
git add backend && git commit -m "feat(storage): users, refresh tokens, conversations, messages, usage repos"
```

---

### Task 8: HTTP auth endpoints + middleware + CORS

**Files:**
- Create: `backend/internal/httpapi/auth.go`, `backend/internal/httpapi/auth_test.go`
- Modify: `backend/internal/httpapi/server.go` (wire deps + routes + CORS), `backend/internal/httpapi/middleware.go` (auth middleware), `backend/go.mod` (none)

**Interfaces:**
- Consumes: Task 6 `*auth.Service`, Task 5 `*auth.TokenMaker`.
- Produces: routes `POST /api/auth/register|login|refresh|logout`, `GET /api/me`; `auth.ContextWithUser(ctx, userID) context.Context`, `auth.UserIDFromContext(ctx) (string, bool)` — **note: put these two in package `httpapi`, not `auth`**, names `ctxUserKey`, `withUser`, `userIDFrom`. Middleware `requireAuth(tokens)`; CORS wrapper `withCORS(origin string)`; refresh cookie name `const refreshCookie = "refresh_token"`.
- Server gains fields: `Auth *auth.Service`, `Tokens *auth.TokenMaker` in `ServerDeps`.

- [ ] **Step 1: Middleware** — append to `backend/internal/httpapi/middleware.go`:

```go
type ctxKey int

const userKey ctxKey = 1

func withUser(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userKey, userID)
}

func userIDFrom(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userKey).(string)
	return id, ok
}

func requireAuth(tokens *auth.TokenMaker, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
			return
		}
		userID, err := tokens.Verify(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid token")
			return
		}
		next.ServeHTTP(w, r.WithContext(withUser(r.Context(), userID)))
	})
}

func withCORS(origin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

Add `backend/internal/httpapi/respond.go`:

```go
package httpapi

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
```

- [ ] **Step 2: Handlers** — `backend/internal/httpapi/auth.go`:

```go
package httpapi

import (
	"errors"
	"net/http"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
)

const refreshCookie = "refresh_token"

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	res, err := s.Auth.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrEmailTaken):
			writeError(w, http.StatusConflict, "email_taken", "that email is already registered")
		case errors.Is(err, auth.ErrWeakPassword):
			writeError(w, http.StatusBadRequest, "weak_password", "password must be at least 10 characters")
		default:
			writeError(w, http.StatusInternalServerError, "internal", "registration failed")
		}
		return
	}
	s.setRefreshCookie(w, res)
	writeJSON(w, http.StatusCreated, map[string]any{
		"accessToken": res.AccessToken,
		"user":        map[string]string{"id": res.User.ID, "email": res.User.Email},
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	res, err := s.Auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	}
	s.setRefreshCookie(w, res)
	writeJSON(w, http.StatusOK, map[string]any{
		"accessToken": res.AccessToken,
		"user":        map[string]string{"id": res.User.ID, "email": res.User.Email},
	})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshCookie)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing refresh cookie")
		return
	}
	res, err := s.Auth.Refresh(r.Context(), cookie.Value)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "refresh rejected")
		return
	}
	s.setRefreshCookie(w, res)
	writeJSON(w, http.StatusOK, map[string]any{"accessToken": res.AccessToken})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(refreshCookie); err == nil {
		_ = s.Auth.Logout(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: refreshCookie, Value: "", Path: "/api/auth",
		MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	id, _ := userIDFrom(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

func (s *Server) setRefreshCookie(w http.ResponseWriter, res auth.AuthResult) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookie,
		Value:    res.RefreshToken,
		Path:     "/api/auth",
		Expires:  res.RefreshExpiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	return dec.Decode(v)
}
```

(`decodeJSON` needs the `encoding/json` import; `MaxBytesReader(nil, …)` — pass `w` where available or drop the limit wrapper if awkward; simplest correct: `dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))` with `io` imported.)

- [ ] **Step 3: Wire routes** — extend `ServerDeps` and `NewServer` in `server.go`:

```go
type ServerDeps struct {
	Cfg    config.Config
	Log    *slog.Logger
	Auth   *auth.Service
	Tokens *auth.TokenMaker
}

type Server struct {
	deps ServerDeps
	mux  *http.ServeMux
}

func NewServer(deps ServerDeps) http.Handler {
	s := &Server{deps: deps, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	s.mux.HandleFunc("POST /api/auth/register", s.handleRegister)
	s.mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/auth/refresh", s.handleRefresh)
	s.mux.HandleFunc("POST /api/auth/logout", s.handleLogout)

	authed := http.NewServeMux()
	authed.HandleFunc("GET /api/me", s.handleMe)
	// later tasks mount more authed routes here
	s.mux.Handle("/api/", requireAuth(deps.Tokens, authed))

	return withCORS(deps.Cfg.FrontendOrigin, logRequests(deps.Log, s.mux))
}
```

Careful: `POST /api/auth/*` are registered on `s.mux` (unauthenticated); the catch-all `/api/` mounts the authed submux — Go's precedence rules pick the more specific pattern, so this composes.

- [ ] **Step 4: Test** — `backend/internal/httpapi/auth_test.go`, using the fakes from Task 6. Export nothing new; duplicate the small fakes in this package (or move them to an `internal/auth/authtest` helper package — choose the helper package `backend/internal/auth/authtest/fakes.go` exposing `NewFakeService() (*auth.Service, *auth.FakeRefreshStore)`; simpler for reuse across handler tests):

`backend/internal/auth/authtest/fakes.go`: move `fakeUsers`/`fakeRefresh` from `service_test.go` here as exported `FakeUsers`, `FakeRefreshStore` with `NewService(secret string) *auth.Service` constructor; `service_test.go` switches to using `authtest` (keep its behavior).

`backend/internal/httpapi/auth_test.go`:

```go
package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth/authtest"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	svc := authtest.NewService("0123456789abcdef0123456789abcdef")
	tm, err := auth.NewTokenMaker("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	return NewServer(ServerDeps{Cfg: config.Config{FrontendOrigin: "http://localhost:5173"}, Log: slog.Default(), Auth: svc, Tokens: tm})
}

func TestRegisterLoginMe(t *testing.T) {
	h := newTestServer(t)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, reqJSON(http.MethodPost, "/api/auth/register",
		map[string]string{"email": "a@b.co", "password": "longenough1"}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", rec.Code, rec.Body.String())
	}
	var reg struct {
		AccessToken string `json:"accessToken"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &reg)

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+reg.AccessToken)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("me: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated me should 401, got %d", rec.Code)
	}
}

func TestRefreshSetsRotatedCookie(t *testing.T) {
	h := newTestServer(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, reqJSON(http.MethodPost, "/api/auth/register",
		map[string]string{"email": "a@b.co", "password": "longenough1"}))
	cookie := rec.Result().Cookies()[0]

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
	req.AddCookie(cookie)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh: %d %s", rec.Code, rec.Body.String())
	}
	if c := rec.Result().Cookies()[0]; c.Value == cookie.Value {
		t.Fatal("refresh cookie must rotate")
	}
}

func reqJSON(method, path string, body any) *http.Request {
	b, _ := json.Marshal(body)
	return httptest.NewRequest(method, path, bytes.NewReader(b))
}
```

- [ ] **Step 5: Run** — `go test ./internal/httpapi/ -v` → PASS. Gate + commit:

```bash
go fmt ./... && go build ./... && go vet ./... && go test ./...
git add backend && git commit -m "feat(httpapi): auth endpoints, bearer middleware, CORS, refresh cookie"
```

---

### Task 9: Shared golem chat agent + sink contract

**Files:**
- Create: `backend/internal/chat/agent.go`, `backend/internal/chat/agent_test.go`

**Interfaces:**
- Consumes: golem `golem.Agent[Deps, string]`, `model.StreamingModel`, `testmodel.Scripted`, `golem.WithRunObserver`, `golem.PartialResult` (via `RunError`).
- Produces:
```go
type Deps struct { UserID, ConversationID string }
type Sink interface {
	Delta(text string) error
	ModelStart()
	ModelEnd(inputTokens, outputTokens int)
}
func New(client model.StreamingModel, usageLimit golem.UsageLimit) (*Agent, error)
type Outcome struct { Output string; Messages []model.Message; Usage model.Usage; Requests int }
func (a *Agent) Run(ctx context.Context, deps Deps, history []model.Message, prompt string, sink Sink) (Outcome, error)
```
Errors pass through as golem's (`*golem.RunError`, cancellation) — the handler classifies.

- [ ] **Step 1: Failing tests** — `backend/internal/chat/agent_test.go`:

```go
package chat

import (
	"context"
	"errors"
	"testing"

	"github.com/abubakarsiddik31/golem"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/testmodel"
)

type recordingSink struct {
	deltas []string
	ends   int
	in, out int
}

func (s *recordingSink) Delta(text string) error { s.deltas = append(s.deltas, text); return nil }
func (s *recordingSink) ModelStart()             {}
func (s *recordingSink) ModelEnd(in, out int)    { s.ends++; s.in += in; s.out += out }

func respond(content string) model.Response {
	return model.Response{Message: model.Message{Role: model.RoleAssistant, Content: content},
		Usage: model.Usage{InputTokens: 10, OutputTokens: 5}}
}

func TestRunStreamsAndCounts(t *testing.T) {
	m := testmodel.New().Respond(respond("hello "))
	a, err := New(m, golem.UsageLimit{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sink := &recordingSink{}
	out, err := a.Run(context.Background(), Deps{UserID: "u", ConversationID: "c"}, nil, "hi", sink)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Output != "hello " || out.Usage.InputTokens != 10 || out.Usage.OutputTokens != 5 {
		t.Fatalf("outcome: %+v", out)
	}
	if len(sink.deltas) == 0 || sink.ends != 1 || sink.in != 10 {
		t.Fatalf("sink not fed: %+v", sink)
	}
	if out.Requests != 1 {
		t.Fatalf("requests = %d, want 1", out.Requests)
	}
}

func TestUsageLimitSurfacesStage(t *testing.T) {
	m := testmodel.New().Respond(respond("big"))
	a, _ := New(m, golem.UsageLimit{TotalTokens: 3})
	sink := &recordingSink{}
	_, err := a.Run(context.Background(), Deps{}, nil, "hi", sink)
	var runErr *golem.RunError
	if !errors.As(err, &runErr) || runErr.Stage != golem.StageUsage {
		t.Fatalf("expected usage-stage RunError, got %v", err)
	}
}
```

- [ ] **Step 2: Run to see it fail** — `go test ./internal/chat/` → FAIL (New undefined).
- [ ] **Step 3: Implement** — `backend/internal/chat/agent.go`:

```go
package chat

import (
	"context"
	"fmt"

	"github.com/abubakarsiddik31/golem"
	"github.com/abubakarsiddik31/golem/model"
)

// Deps is the per-run identity flowing to every tool (search joins in Phase 2).
type Deps struct {
	UserID         string
	ConversationID string
}

// Sink receives run progress. Implemented by the httpapi SSE writer; tests
// use recording fakes.
type Sink interface {
	Delta(text string) error
	ModelStart()
	ModelEnd(inputTokens, outputTokens int)
}

const systemPrompt = `You are a helpful assistant in a local chat app.
Answer clearly and concisely in markdown.`

// Agent is the shared, concurrency-safe chat agent. One instance per process.
type Agent struct {
	agent *golem.Agent[Deps, string]
}

func New(client model.StreamingModel, usageLimit golem.UsageLimit) (*Agent, error) {
	passthrough := golem.DecodeFunc[string](func(_ context.Context, r model.Response) (string, error) {
		return r.Message.Content, nil
	})
	a, err := golem.New[Deps, string](client, passthrough,
		golem.WithInstructions[Deps, string](systemPrompt),
		golem.WithHistoryProcessor[Deps, string](golem.TrimHistory(40)),
		golem.WithMaxAttempts[Deps, string](2),
		golem.WithUsageLimit[Deps, string](usageLimit),
	)
	if err != nil {
		return nil, fmt.Errorf("build chat agent: %w", err)
	}
	return &Agent{agent: a}, nil
}

type Outcome struct {
	Output   string
	Messages []model.Message
	Usage    model.Usage
	Requests int
}

// Run streams one turn. Progress goes to sink; errors are returned as-is
// (golem.RunError with Partial evidence, or context cancellation) for the
// caller to classify and persist.
func (a *Agent) Run(ctx context.Context, deps Deps, history []model.Message, prompt string, sink Sink) (Outcome, error) {
	var requests int
	res, err := a.agent.RunStreamWithHistory(ctx, golem.RunContext[Deps]{Deps: deps},
		history, prompt,
		func(d model.Delta) error {
			if d.Content == "" {
				return nil
			}
			return sink.Delta(d.Content)
		},
		golem.WithRunObserver(func(e golem.RunEvent) {
			switch e.Kind {
			case golem.EventModelStart:
				sink.ModelStart()
			case golem.EventModelEnd:
				requests++
				sink.ModelEnd(e.Usage.InputTokens, e.Usage.OutputTokens)
			}
		}),
	)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{Output: res.Output, Messages: res.Messages, Usage: res.Usage, Requests: requests}, nil
}
```

- [ ] **Step 4: Run** — `go test ./internal/chat/ -v` → PASS. Gate + commit:

```bash
go fmt ./... && go build ./... && go vet ./... && go test ./...
git add backend && git commit -m "feat(chat): shared golem agent with run-scoped observer and sink contract"
```

---

### Task 10: Conversations CRUD endpoints

**Files:**
- Create: `backend/internal/httpapi/conversations.go`
- Modify: `backend/internal/httpapi/server.go` (mount authed routes), `backend/go.mod` (`google/uuid` already present)

**Interfaces:**
- Consumes: `*storage.Conversations`, `*storage.Messages` (Task 7).
- Produces: `GET /api/conversations`, `POST /api/conversations` `{title?}`, `GET /api/conversations/{id}` (with `messages` array), `PATCH /api/conversations/{id}` `{title}`, `DELETE /api/conversations/{id}`; JSON wire shapes `{id, title, createdAt, updatedAt}` and message `{id, role, content, truncated, createdAt}`.
- Also extend `ServerDeps` with `Convos *storage.Conversations`, `Msgs *storage.Messages`.

- [ ] **Step 1: Implement** — `backend/internal/httpapi/conversations.go`:

```go
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

type conversationDTO struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func toConversationDTO(c storage.Conversation) conversationDTO {
	return conversationDTO{ID: c.ID, Title: c.Title, CreatedAt: c.CreatedAt.UTC().Format(timeRFC3339), UpdatedAt: c.UpdatedAt.UTC().Format(timeRFC3339)}
}

const timeRFC3339 = "2006-01-02T15:04:05.000Z07:00"

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	convs, err := s.deps.Convos.List(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list conversations")
		return
	}
	if convs == nil {
		convs = []storage.Conversation{}
	}
	out := make([]conversationDTO, len(convs))
	for i, c := range convs {
		out[i] = toConversationDTO(c)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateConversation(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	var req struct {
		Title string `json:"title"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req) // optional body
	}
	conv, err := s.deps.Convos.Create(r.Context(), userID, req.Title)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not create conversation")
		return
	}
	writeJSON(w, http.StatusCreated, toConversationDTO(conv))
}

func (s *Server) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	conv, err := s.deps.Convos.ByID(r.Context(), r.PathValue("id"), userID)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "conversation not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load conversation")
		return
	}
	msgs, err := s.deps.Msgs.ForConversation(r.Context(), conv.ID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load messages")
		return
	}
	type msgDTO struct {
		ID        string `json:"id"`
		Role      string `json:"role"`
		Content   string `json:"content"`
		Truncated bool   `json:"truncated"`
		CreatedAt string `json:"createdAt"`
	}
	out := make([]msgDTO, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, msgDTO{ID: m.ID, Role: m.Role, Content: m.Content, Truncated: m.Truncated, CreatedAt: m.CreatedAt.UTC().Format(timeRFC3339)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversation": toConversationDTO(conv), "messages": out})
}

func (s *Server) handlePatchConversation(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	var req struct {
		Title string `json:"title"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Title == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "title is required")
		return
	}
	if err := s.deps.Convos.SetTitle(r.Context(), r.PathValue("id"), userID, req.Title); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not rename")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	if err := s.deps.Convos.Delete(r.Context(), r.PathValue("id"), userID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not delete")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

Mount in `NewServer` inside the `authed` mux:

```go
	authed.HandleFunc("GET /api/conversations", s.handleListConversations)
	authed.HandleFunc("POST /api/conversations", s.handleCreateConversation)
	authed.HandleFunc("GET /api/conversations/{id}", s.handleGetConversation)
	authed.HandleFunc("PATCH /api/conversations/{id}", s.handlePatchConversation)
	authed.HandleFunc("DELETE /api/conversations/{id}", s.handleDeleteConversation)
```

Add the `io` import; `decodeJSON` already exists (Task 8). Handler tests for this task come via Task 11's SSE test (which exercises persistence) plus one wire-up check here — keep it lean: extend `auth_test.go`'s server builder in Task 11.

- [ ] **Step 2: Gate + commit**

```bash
go fmt ./... && go build ./... && go vet ./... && go test ./...
git add backend && git commit -m "feat(httpapi): conversations CRUD endpoints"
```

---

### Task 11: Chat SSE endpoint — streaming, persistence, partial evidence, cost

**Files:**
- Create: `backend/internal/httpapi/sse.go`, `backend/internal/httpapi/chat.go`, `backend/internal/httpapi/chat_test.go`
- Modify: `backend/internal/httpapi/server.go` (mount + `Agent *chat.Agent`, `Usage *storage.Usage`, `Rates cost.Rates` in deps), `backend/cmd/server/main.go` (build the real agent — final wiring happens in Task 12; leave main untouched here)

**Interfaces:**
- Consumes: Task 9 `chat.Agent.Run(ctx, deps, history, prompt, sink)`; Task 7 stores; Task 4 rates.
- Produces: `POST /api/conversations/{id}/messages` `{content}` → SSE stream; `sseSink` implementing `chat.Sink` with `done(messageID, usage, costMicros)` and `error(stage, message)`; history decoding helper `messagesToHistory([]storage.Message) []model.Message` (all but the last message, `json.Unmarshal` of `Data`).

- [ ] **Step 1: SSE writer** — `backend/internal/httpapi/sse.go`:

```go
package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// sseSink writes the chat SSE protocol and implements chat.Sink.
type sseSink struct {
	w        http.ResponseWriter
	flush    http.Flusher
	requests int
}

func newSSESink(w http.ResponseWriter) (*sseSink, bool) {
	flush, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flush.Flush()
	return &sseSink{w: w, flush: flush}, true
}

func (s *sseSink) event(name string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, b); err != nil {
		return err
	}
	s.flush.Flush()
	return nil
}

func (s *sseSink) Delta(text string) error { return s.event("delta", map[string]string{"text": text}) }
func (s *sseSink) ModelStart()             { _ = s.event("meta", map[string]string{"type": "model_start"}) }
func (s *sseSink) ModelEnd(in, out int) {
	s.requests++
	_ = s.event("meta", map[string]any{"type": "model_end", "inputTokens": in, "outputTokens": out})
}
```

- [ ] **Step 2: Handler** — `backend/internal/httpapi/chat.go`:

```go
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"

	"github.com/abubakarsiddik31/golem"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/cost"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

const maxPromptChars = 8000

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	convID := r.PathValue("id")
	var req struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Content == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "content is required")
		return
	}
	if len([]rune(req.Content)) > maxPromptChars {
		writeError(w, http.StatusBadRequest, "bad_request", "message too long")
		return
	}

	ctx := r.Context()
	if _, err := s.deps.Convos.ByID(ctx, convID, userID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not load conversation")
		return
	}

	// Auto-title from the first user message.
	if n, err := s.deps.Convos.CountMessages(ctx, convID, userID); err == nil && n == 0 {
		_ = s.deps.Convos.SetTitle(ctx, convID, userID, truncateRunes(req.Content, 48))
	}

	// Persist the user message first, then build history from prior turns.
	if err := s.deps.Msgs.Add(ctx, storage.Message{
		ConversationID: convID, UserID: userID, Role: model.RoleUser,
		Content: req.Content,
		Data:    mustJSON(model.Message{Role: model.RoleUser, Content: req.Content}),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not store message")
		return
	}
	msgs, err := s.deps.Msgs.ForConversation(ctx, convID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load history")
		return
	}
	history := messagesToHistory(msgs)

	// SSE headers go out only after validation; from here the response is a stream.
	sink, ok := newSSESink(w)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal", "streaming unsupported")
		return
	}

	outcome, err := s.deps.Agent.Run(ctx, chat.Deps{UserID: userID, ConversationID: convID}, history, req.Content, sink)
	if err != nil {
		s.persistFailure(ctx, userID, convID, err, sink, r)
		return
	}
	s.finishRun(ctx, userID, convID, outcome, sink)
}

// persistFailure uses golem v0.7.1 partial evidence: whatever completed is
// kept, marked truncated.
func (s *Server) persistFailure(ctx context.Context, userID, convID string, runErr error, sink *sseSink, r *http.Request) {
	clientGone := errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded)
	var runError *golem.RunError
	_ = errors.As(runErr, &runError)

	var partial *golem.PartialResult
	if runError != nil {
		partial = runError.Partial
	}
	if partial != nil && len(partial.Messages) > 0 {
		content := partialTailText(partial.Messages)
		if content != "" {
			if err := s.deps.Msgs.Add(ctx, storage.Message{
				ConversationID: convID, UserID: userID, Role: model.RoleAssistant,
				Content: content, Data: mustJSON(partial.Messages[len(partial.Messages)-1]),
				InputTokens: partial.Usage.InputTokens, OutputTokens: partial.Usage.OutputTokens,
				Requests: partial.Requests, Truncated: true,
			}); err != nil {
				s.deps.Log.Error("persist partial", "err", err)
			}
			_ = s.deps.Usage.Add(ctx, usageEventFor(userID, convID, s.deps.Cfg.GeminiModel, partial.Usage, partial.Requests, s.deps.Rates))
		}
	}
	_ = s.deps.Convos.Touch(ctx, convID)
	if clientGone && r.Context().Err() != nil {
		return // client is gone; writing would fail anyway
	}
	if clientGone {
		return // quiet stop for user-initiated cancels
	}
	stage := "model"
	if runError != nil {
		stage = string(runError.Stage)
	}
	_ = sink.event("error", map[string]string{"stage": stage, "message": userMessage(runErr)})
	s.deps.Log.Error("chat run failed", "err", runErr)
}

func (s *Server) finishRun(ctx context.Context, userID, convID string, outcome chat.Outcome, sink *sseSink) {
	cost := s.deps.Rates.ChatCostMicros(outcome.Usage.InputTokens, outcome.Usage.OutputTokens)
	var data []byte
	if len(outcome.Messages) > 0 {
		data = mustJSON(outcome.Messages[len(outcome.Messages)-1])
	}
	if err := s.deps.Msgs.Add(ctx, storage.Message{
		ConversationID: convID, UserID: userID, Role: model.RoleAssistant,
		Content: outcome.Output, Data: data,
		InputTokens: outcome.Usage.InputTokens, OutputTokens: outcome.Usage.OutputTokens,
		Requests: outcome.Requests, CostMicros: cost,
	}); err != nil {
		s.deps.Log.Error("persist assistant message", "err", err)
	}
	if err := s.deps.Usage.Add(ctx, usageEventFor(userID, convID, s.deps.Cfg.GeminiModel, outcome.Usage, outcome.Requests, s.deps.Rates)); err != nil {
		s.deps.Log.Error("record usage", "err", err)
	}
	_ = s.deps.Convos.Touch(ctx, convID)

	// The stored message id is needed for `done`; fetch the latest assistant row.
	msgID := ""
	if msgs, err := s.deps.Msgs.ForConversation(ctx, convID, userID); err == nil && len(msgs) > 0 {
		msgID = msgs[len(msgs)-1].ID
	}
	_ = sink.event("done", map[string]any{
		"messageId":    msgID,
		"inputTokens":  outcome.Usage.InputTokens,
		"outputTokens": outcome.Usage.OutputTokens,
		"requests":     outcome.Requests,
		"costUsd":      math.Round(float64(cost)/10) / 1e5, // 4 decimal places
	})
}

func usageEventFor(userID, convID, mdl string, usage model.Usage, requests int, rates cost.Rates) storage.UsageEvent {
	cid := convID
	return storage.UsageEvent{
		UserID: userID, Kind: "chat", Model: mdl, ConversationID: &cid,
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		Requests: requests, CostMicros: rates.ChatCostMicros(usage.InputTokens, usage.OutputTokens),
	}
}

// messagesToHistory decodes every stored message except the last (the fresh
// user prompt, which golem appends itself).
func messagesToHistory(msgs []storage.Message) []model.Message {
	if len(msgs) <= 1 {
		return nil
	}
	var history []model.Message
	for _, m := range msgs[:len(msgs)-1] {
		var mm model.Message
		if err := json.Unmarshal(m.Data, &mm); err != nil {
			continue // damaged row: skip rather than fail the run
		}
		history = append(history, mm)
	}
	return history
}

func partialTailText(msgs []model.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == model.RoleAssistant {
			return msgs[i].Content
		}
	}
	return ""
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func userMessage(err error) string {
	var runErr *golem.RunError
	if errors.As(err, &runErr) {
		return "the model run failed at the " + string(runErr.Stage) + " stage"
	}
	return "the model run failed"
}
```

Cleanups while implementing: the store interfaces (`ConvoStore`, `MsgStore`, `UsageStore`) must be defined in `chat.go` with exactly the methods used; `userMessage` and `usageEventFor` are file-local helpers.

Mount in `NewServer` (authed mux):

```go
	authed.HandleFunc("POST /api/conversations/{id}/messages", s.handleSendMessage)
```

Extend `ServerDeps`: `Agent *chat.Agent`, `Usage *storage.Usage`, `Rates cost.Rates`.

- [ ] **Step 3: Offline test** — `backend/internal/httpapi/chat_test.go` (golem `testmodel.Scripted` feeds the agent; storage is nil-safe fakes? No — storage needs a DB. Use a tiny in-memory fake for `Msgs`/`Convos`/`Usage`? They are concrete `*storage.X` types, not interfaces. To keep handler tests offline, define handler-scoped interfaces instead: `type convoStore interface{...}`, `type msgStore interface{...}`, `type usageStore interface{...}` in `httpapi`, satisfied by both `*storage.*` and test fakes. Do that: change `Server` deps fields to the interfaces `ConvoStore`, `MsgStore`, `UsageStore` (defined in `chat.go` with exactly the methods used: Create/List/ByID/SetTitle/Touch/Delete/CountMessages; Add/ForConversation; Add/Summary). Storage types satisfy them implicitly.)

```go
package httpapi

import (
	"strings"
	"testing"

	"github.com/abubakarsiddik31/golem"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/testmodel"
	"github.com/abubakarsiddik31/golem-chatbot/internal/cost"
)

func TestSendMessageStreamsAndPersists(t *testing.T) {
	m := testmodel.New().Respond(model.Response{
		Message: model.Message{Role: model.RoleAssistant, Content: "Hi there!"},
		Usage:   model.Usage{InputTokens: 12, OutputTokens: 7},
	})
	agent, err := chat.New(m, golem.UsageLimit{})
	if err != nil {
		t.Fatal(err)
	}
	convs := newFakeConvos()
	msgs := newFakeMsgs()
	usage := newFakeUsage()
	s := newHandlerServer(t, agent, convs, msgs, usage)

	conv := convs.mustCreate("u-1", "")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, reqJSON(http.MethodPost, "/api/conversations/"+conv.ID+"/messages",
		map[string]string{"content": "hello"}))

	body := rr.Body.String()
	for _, want := range []string{"event: delta", `"text":"Hi` , "event: done", `"requests":1`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in SSE:\n%s", want, body)
		}
	}
	if msgs.count() != 2 { // user + assistant
		t.Fatalf("messages persisted: %d", msgs.count())
	}
	if usage.events[0].InputTokens != 12 || usage.events[0].CostMicros != 0 {
		t.Fatalf("usage not recorded: %+v", usage.events)
	}
	if convs.convs[conv.ID].Title == "" {
		t.Fatal("conversation should be auto-titled")
	}
}
```

The fakes (`newFakeConvos`/`newFakeMsgs`/`newFakeUsage`/`newHandlerServer`) are small structs in the same test file implementing the three interfaces; `newHandlerServer` wires `NewServer(ServerDeps{..., Agent: agent, Convos: convs, Msgs: msgs, Usage: usage, Rates: cost.Rates{ChatInputPerM: 0.3, ChatOutputPerM: 2.5}})` with the authtest service and issues a token for `u-1`. Keep them under 80 lines total.

Add one more test here:

```go
func TestCancelPersistsPartial(t *testing.T) {
	// Scripted stream that errors mid-stream: use a testmodel.StreamFunc
	// emitting one delta then returning a fake error wrapped so the run
	// fails at the model stage with partial evidence.
	m := testmodel.StreamFunc(func(ctx context.Context, _ model.Request, onDelta func(model.Delta) error) (model.Response, error) {
		_ = onDelta(model.Delta{Content: "partial "})
		return model.Response{}, errors.New("boom")
	})
	agent, _ := chat.New(m, golem.UsageLimit{})
	// ... wire server, send message, expect: no `done`, one delta, assistant
	// row persisted with Truncated=true (Scripted Func path returns a plain
	// error; golem wraps it as RunError{Stage: model}; Partial may be nil for
	// pre-evidence failures — assert only that the error event is emitted and
	// no assistant row exists when Partial is nil).
}
```

Implement this test against actual golem behavior: with a failing first attempt and no completed turn, `Partial` is nil → expect `event: error` with `"stage":"model"`, no persisted assistant row. Note that expectation in the test name: `TestFailedRunEmitsErrorEvent`.

- [ ] **Step 4: Run** — `go test ./internal/httpapi/ -v` → PASS. Gate + commit:

```bash
go fmt ./... && go build ./... && go vet ./... && go test ./...
git add backend && git commit -m "feat(httpapi): SSE chat endpoint with persistence, partial evidence, cost ledger"
```

---

### Task 12: Usage summary endpoint + main wiring + graceful shutdown

**Files:**
- Create: `backend/internal/httpapi/usage.go`
- Modify: `backend/internal/httpapi/server.go` (route + `Usage` dep already there), `backend/cmd/server/main.go` (full wiring)

**Interfaces:**
- Produces: `GET /api/usage/summary?days=30` → `{"totals":[{kind,model,inputTokens,outputTokens,requests,costUsd}],"daily":[{day,inputTokens,outputTokens,costUsd}]}`; `main.go` boots config → pool → migrate → stores → gemini client → chat agent → server with graceful shutdown.

- [ ] **Step 1: Handler** — `backend/internal/httpapi/usage.go`:

```go
package httpapi

import (
	"math"
	"net/http"
	"strconv"
)

func microsToUSD(m int64) float64 { return math.Round(float64(m)/10) / 1e5 }

func (s *Server) handleUsageSummary(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	days := 30
	if d, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && d > 0 && d <= 365 {
		days = d
	}
	sum, err := s.deps.Usage.Summary(r.Context(), userID, days)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load usage")
		return
	}
	type row struct {
		Kind         string  `json:"kind"`
		Model        string  `json:"model"`
		InputTokens  int     `json:"inputTokens"`
		OutputTokens int     `json:"outputTokens"`
		Requests     int     `json:"requests"`
		CostUsd      float64 `json:"costUsd"`
	}
	totals := make([]row, 0, len(sum.Totals))
	for _, t := range sum.Totals {
		totals = append(totals, row{t.Kind, t.Model, t.InputTokens, t.OutputTokens, t.Requests, microsToUSD(t.CostMicros)})
	}
	type daily struct {
		Day          string  `json:"day"`
		InputTokens  int     `json:"inputTokens"`
		OutputTokens int     `json:"outputTokens"`
		CostUsd      float64 `json:"costUsd"`
	}
	dailies := make([]daily, 0, len(sum.Daily))
	for _, d := range sum.Daily {
		dailies = append(dailies, daily{d.Day.Format("2006-01-02"), d.InputTokens, d.OutputTokens, microsToUSD(d.CostMicros)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"totals": totals, "daily": dailies})
}
```

Mount: `authed.HandleFunc("GET /api/usage/summary", s.handleUsageSummary)`.

- [ ] **Step 2: Full wiring** — replace `backend/cmd/server/main.go`:

```go
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	golemgemini "github.com/abubakarsiddik31/golem/providers/gemini"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/cost"
	"github.com/abubakarsiddik31/golem-chatbot/internal/httpapi"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := storage.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := storage.Migrate(ctx, pool); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}

	users := storage.NewUsers(pool)
	refresh := storage.NewRefreshTokens(pool)
	svc, err := auth.NewService(users, refresh, cfg.JWTSecret)
	if err != nil {
		log.Error("auth", "err", err)
		os.Exit(1)
	}
	tokens, err := auth.NewTokenMaker(cfg.JWTSecret)
	if err != nil {
		log.Error("auth", "err", err)
		os.Exit(1)
	}

	gemini, err := golemgemini.New(golemgemini.Config{
		APIKey: cfg.GeminiAPIKey, Model: cfg.GeminiModel, BaseURL: cfg.GeminiBaseURL,
	})
	if err != nil {
		log.Error("gemini", "err", err)
		os.Exit(1)
	}
	agent, err := chat.New(gemini, golem.UsageLimit{Requests: 12, TotalTokens: 100_000})
	if err != nil {
		log.Error("agent", "err", err)
		os.Exit(1)
	}

	handler := httpapi.NewServer(httpapi.ServerDeps{
		Cfg:    cfg,
		Log:    log,
		Auth:   svc,
		Tokens: tokens,
		Convos: storage.NewConversations(pool),
		Msgs:   storage.NewMessages(pool),
		Usage:  storage.NewUsage(pool),
		Agent:  agent,
		Rates:  cost.Rates{ChatInputPerM: cfg.ChatInputRate, ChatOutputPerM: cfg.ChatOutputRate},
	})
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: handler, ReadHeaderTimeout: 10 * time.Second}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	log.Info("listening", "port", cfg.Port)

	select {
	case err := <-errCh:
		log.Error("server", "err", err)
		os.Exit(1)
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("shutdown", "err", err)
	}
	log.Info("stopped")
}
```

- [ ] **Step 3: Smoke test with real infra**

```bash
make up
cd backend && DATABASE_URL=$(grep DATABASE_URL ../.env | cut -d= -f2-) GEMINI_API_KEY=$(grep GEMINI_API_KEY ../.env | cut -d= -f2-) JWT_SECRET=0123456789abcdef0123456789abcdef go run ./cmd/server &
curl -s localhost:8080/healthz   # ok
curl -s -X POST localhost:8080/api/auth/register -H 'Content-Type: application/json' \
  -d '{"email":"a@b.co","password":"longenough1"}'   # accessToken present
```

Then `go test ./...` once more (all offline green) and kill the server. Commit:

```bash
git add backend && git commit -m "feat(backend): usage summary endpoint and full server wiring with graceful shutdown"
```

---

### Task 13: Frontend scaffold — Vite, TS strict, Tailwind v4, shadcn/ui, quality gates

**Files:**
- Create: `frontend/` (Vite scaffold), `frontend/src/lib/api.ts`, `frontend/src/lib/types.ts`, `frontend/src/stores/auth.ts`
- Test: `frontend/src/lib/api.test.ts`

**Interfaces:**
- Produces: `apiFetch<T>(path: string, opts?: RequestInit & {json?: unknown}): Promise<T>` — attaches `Authorization`, retries once through `/api/auth/refresh` on 401, throws `ApiError{status, code, message}`; `useAuth` Zustand store `{user: {id,email}|null, accessToken: string|null, setAuth, clear}`; `VITE_API_URL` env (default `http://localhost:8080`).

- [ ] **Step 1: Scaffold**

```bash
npm create vite@latest frontend -- --template react-ts
cd frontend
npm i
npm i react-router @tanstack/react-query zustand react-markdown remark-gfm recharts sonner
npm i -D tailwindcss @tailwindcss/vite vitest jsdom @testing-library/react @testing-library/user-event @testing-library/jest-dom msw prettier
npx shadcn@latest init -y   # choose neutral base color; accepts Tailwind v4
npx shadcn@latest add button input card label sonner scroll-area badge skeleton dialog
```

In `vite.config.ts`: add `plugins: [react(), tailwindcss()]`, `resolve: { alias: { "@": "/src" } }`, `server: { port: 5173 }`, and `test:` config (vitest): `environment: "jsdom"`, `globals: true`, `setupFiles: "./src/setupTests.ts"`. Set `tsconfig.app.json` `compilerOptions.strict: true` (already) and paths `{"@/*": ["./src/*"]}`. Add `"test": "vitest run", "check": "npm run lint && tsc -b && npm run test"` to scripts. `src/index.css` starts with `@import "tailwindcss";`. `src/setupTests.ts`: `import "@testing-library/jest-dom/vitest";`.

- [ ] **Step 2: Types + store + client** — `frontend/src/lib/types.ts`:

```ts
export interface User { id: string; email: string }
export interface Conversation { id: string; title: string; createdAt: string; updatedAt: string }
export interface ChatMessage {
  id: string; role: "user" | "assistant"; content: string;
  truncated: boolean; createdAt: string;
  streaming?: boolean; error?: string;
}
export interface UsageTotalsRow { kind: string; model: string; inputTokens: number; outputTokens: number; requests: number; costUsd: number }
export interface UsageDailyRow { day: string; inputTokens: number; outputTokens: number; costUsd: number }
export interface UsageSummary { totals: UsageTotalsRow[]; daily: UsageDailyRow[] }
```

`frontend/src/stores/auth.ts`:

```ts
import { create } from "zustand";
import type { User } from "@/lib/types";

interface AuthState {
  user: User | null;
  accessToken: string | null;
  setAuth: (user: User, accessToken: string) => void;
  clear: () => void;
}

// Access token lives in memory only; the refresh cookie is HttpOnly.
export const useAuth = create<AuthState>((set) => ({
  user: null,
  accessToken: null,
  setAuth: (user, accessToken) => set({ user, accessToken }),
  clear: () => set({ user: null, accessToken: null }),
}));
```

`frontend/src/lib/api.ts`:

```ts
import { useAuth } from "@/stores/auth";

const BASE = import.meta.env.VITE_API_URL ?? "http://localhost:8080";

export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string) {
    super(message);
  }
}

let refreshing: Promise<boolean> | null = null;

async function tryRefresh(): Promise<boolean> {
  refreshing ??= fetch(`${BASE}/api/auth/refresh`, {
    method: "POST",
    credentials: "include",
  }).then(async (res) => {
    if (!res.ok) return false;
    const data = (await res.json()) as { accessToken: string };
    const { user, setAuth } = useAuth.getState();
    if (user) setAuth(user, data.accessToken);
    return true;
  }).finally(() => { refreshing = null; });
  return refreshing;
}

export async function apiFetch<T>(path: string, opts: RequestInit & { json?: unknown } = {}): Promise<T> {
  const doFetch = () => {
    const headers = new Headers(opts.headers);
    if (opts.json !== undefined) headers.set("Content-Type", "application/json");
    const token = useAuth.getState().accessToken;
    if (token) headers.set("Authorization", `Bearer ${token}`);
    return fetch(`${BASE}${path}`, {
      ...opts,
      headers,
      body: opts.json !== undefined ? JSON.stringify(opts.json) : opts.body,
      credentials: "include",
    });
  };

  let res = await doFetch();
  if (res.status === 401 && (await tryRefresh())) {
    res = await doFetch();
  }
  if (!res.ok) {
    let code = "error", message = `request failed (${res.status})`;
    try {
      const body = (await res.json()) as { error?: { code: string; message: string } };
      if (body.error) { code = body.error.code; message = body.error.message; }
    } catch { /* non-JSON error body */ }
    if (res.status === 401) useAuth.getState().clear();
    throw new ApiError(res.status, code, message);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export { BASE };
```

- [ ] **Step 3: Test the refresh retry** — `frontend/src/lib/api.test.ts` with MSW:

```ts
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { beforeEach, describe, expect, it } from "vitest";
import { ApiError, apiFetch } from "./api";
import { useAuth } from "@/stores/auth";

const handlers = [
  http.post("*/api/auth/refresh", () =>
    HttpResponse.json({ accessToken: "fresh" })),
  http.get("*/api/things", ({ request }) => {
    if (request.headers.get("Authorization") === "Bearer fresh") {
      return HttpResponse.json({ ok: true });
    }
    return HttpResponse.json(
      { error: { code: "unauthorized", message: "no" } }, { status: 401 });
  }),
];
const server = setupServer(...handlers);
beforeEach(() => {
  server.listen({ onUnhandledRequest: "error" });
  useAuth.setState({ user: { id: "u", email: "a@b.co" }, accessToken: "stale" });
});

describe("apiFetch", () => {
  it("refreshes once on 401 and retries with the new token", async () => {
    const res = await apiFetch<{ ok: boolean }>("/api/things");
    expect(res.ok).toBe(true);
    expect(useAuth.getState().accessToken).toBe("fresh");
  });
});

server.close();
```

(`server.close()` belongs in `afterAll`; adjust while implementing.)

- [ ] **Step 4: Run** — `npm run check` → PASS. Commit:

```bash
cd .. && git add frontend && git commit -m "feat(frontend): vite scaffold, api client with silent refresh, auth store"
```

---

### Task 14: Router, auth pages, guard

**Files:**
- Create: `frontend/src/App.tsx` (router + providers), `frontend/src/pages/LoginPage.tsx`, `frontend/src/pages/RegisterPage.tsx`, `frontend/src/components/RequireAuth.tsx`, `frontend/src/features/auth/AuthForm.tsx`
- Modify: `frontend/src/main.tsx`

**Interfaces:**
- Consumes: Task 13 `apiFetch`, `useAuth`.
- Produces: routes `/login`, `/register`, `/` (chat, guarded), `/usage` (guarded). `useAuth` populated after login.

- [ ] **Step 1: `AuthForm` + pages.** `frontend/src/features/auth/AuthForm.tsx` — controlled email/password inputs (shadcn `Input`, `Button`, `Card`), submit calls `apiFetch<{accessToken: string; user: User}>("/api/auth/login" | "/api/auth/register", {method: "POST", json: {...}})`, then `setAuth(user, accessToken)` and `navigate("/")`. Errors surface via `sonner`'s `toast.error(err.message)`. Show the full code inline in the final implementation (≈60 lines; two pages are thin wrappers setting `mode: "login" | "register"` and the heading).

`frontend/src/App.tsx`:

```tsx
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Navigate, Route, Routes } from "react-router";
import { Toaster } from "@/components/ui/sonner";
import { RequireAuth } from "@/components/RequireAuth";
import { LoginPage } from "@/pages/LoginPage";
import { RegisterPage } from "@/pages/RegisterPage";
import { ChatPage } from "@/pages/ChatPage";
import { UsagePage } from "@/pages/UsagePage";

const queryClient = new QueryClient();

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />
          <Route element={<RequireAuth />}>
            <Route path="/" element={<ChatPage />} />
            <Route path="/usage" element={<UsagePage />} />
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
        <Toaster position="top-center" />
      </BrowserRouter>
    </QueryClientProvider>
  );
}
```

`frontend/src/components/RequireAuth.tsx`:

```tsx
import { Navigate, Outlet } from "react-router";
import { useAuth } from "@/stores/auth";

export function RequireAuth() {
  const token = useAuth((s) => s.accessToken);
  return token ? <Outlet /> : <Navigate to="/login" replace />;
}
```

`main.tsx` renders `<App />`.

- [ ] **Step 2: Gate + commit**

```bash
npm run check && cd .. && git add frontend && git commit -m "feat(frontend): router, auth pages with silent-refresh-backed guard"
```

---

### Task 15: Chat UI — conversations, thread, SSE streaming, beautifului AI states

**Files:**
- Create: `frontend/src/lib/sse.ts`, `frontend/src/lib/sse.test.ts`, `frontend/src/features/chat/useChat.ts`, `frontend/src/features/chat/useConversations.ts`, `frontend/src/pages/ChatPage.tsx`, `frontend/src/components/ai/StreamingText.tsx`, `frontend/src/components/ai/ThinkingTrace.tsx`, `frontend/src/components/ai/RunLoader.tsx`
- Modify: `frontend/src/App.tsx` (already routes `/`)

**Interfaces:**
- Consumes: `apiFetch`, SSE protocol from Task 11.
- Produces: `parseSSE(res: Response): AsyncGenerator<{event: string; data: string}>`; `useChat(convId)` → `{messages, status, send(content, {signal}), stop()}`; `useConversations()` TanStack Query list + create + invalidate.

- [ ] **Step 1: SSE parser with failing test** — `frontend/src/lib/sse.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { parseSSE } from "./sse";

function sseResponse(frames: string): Response {
  const stream = new ReadableStream({
    start(controller) {
      controller.enqueue(new TextEncoder().encode(frames));
      controller.close();
    },
  });
  return new Response(stream, { headers: { "Content-Type": "text/event-stream" } });
}

describe("parseSSE", () => {
  it("yields named events with data payloads", async () => {
    const res = sseResponse('event: delta\ndata: {"text":"Hi"}\n\nevent: done\ndata: {"requests":1}\n\n');
    const events: { event: string; data: string }[] = [];
    for await (const e of parseSSE(res)) events.push(e);
    expect(events).toEqual([
      { event: "delta", data: '{"text":"Hi"}' },
      { event: "done", data: '{"requests":1}' },
    ]);
  });
});
```

`frontend/src/lib/sse.ts`:

```ts
// Minimal SSE frame parser over fetch's ReadableStream — EventSource cannot
// send the Authorization header, so the chat stream uses POST + fetch.
export async function* parseSSE(res: Response): AsyncGenerator<{ event: string; data: string }> {
  const reader = res.body!.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    let idx: number;
    while ((idx = buffer.indexOf("\n\n")) !== -1) {
      const frame = buffer.slice(0, idx);
      buffer = buffer.slice(idx + 2);
      let event = "message";
      let data = "";
      for (const line of frame.split("\n")) {
        if (line.startsWith("event: ")) event = line.slice(7);
        else if (line.startsWith("data: ")) data += line.slice(6);
      }
      if (data) yield { event, data };
    }
  }
}
```

- [ ] **Step 2: Run test** — `npx vitest run src/lib/sse.test.ts` → PASS after implementing.

- [ ] **Step 3: `useChat` hook** — `frontend/src/features/chat/useChat.ts`:

```ts
import { useCallback, useRef, useState } from "react";
import { BASE, ApiError } from "@/lib/api";
import { parseSSE } from "@/lib/sse";
import { useAuth } from "@/stores/auth";
import type { ChatMessage } from "@/lib/types";

export type RunStatus = "idle" | "running" | "error";

interface DonePayload { messageId: string; inputTokens: number; outputTokens: number; requests: number; costUsd: number }

export function useChat(onDone?: () => void) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [status, setStatus] = useState<RunStatus>("idle");
  const abortRef = useRef<AbortController | null>(null);

  const stop = useCallback(() => abortRef.current?.abort(), []);

  const send = useCallback(async (content: string, conversationId: string) => {
    const controller = new AbortController();
    abortRef.current = controller;
    const userMsg: ChatMessage = { id: crypto.randomUUID(), role: "user", content, truncated: false, createdAt: new Date().toISOString() };
    const assistantId = crypto.randomUUID();
    setMessages((m) => [
      ...m, userMsg,
      { id: assistantId, role: "assistant", content: "", truncated: false, createdAt: new Date().toISOString(), streaming: true },
    ]);
    setStatus("running");
    let hadError = false;
    try {
      const token = useAuth.getState().accessToken;
      const res = await fetch(`${BASE}/api/conversations/${conversationId}/messages`, {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
        body: JSON.stringify({ content }),
        credentials: "include",
        signal: controller.signal,
      });
      if (!res.ok || !res.body) {
        if (res.status === 401) useAuth.getState().clear();
        throw new ApiError(res.status, "error", `stream failed (${res.status})`);
      }
      let received = false;
      for await (const frame of parseSSE(res)) {
        const payload = JSON.parse(frame.data) as any;
        if (frame.event === "delta") {
          received = true;
          setMessages((m) => m.map((msg) => msg.id === assistantId ? { ...msg, content: msg.content + payload.text } : msg));
        } else if (frame.event === "done") {
          const done = payload as DonePayload;
          setMessages((m) => m.map((msg) => msg.id === assistantId
            ? { ...msg, id: done.messageId || msg.id, streaming: false } : msg));
        } else if (frame.event === "error") {
          hadError = true;
          setMessages((m) => m.map((msg) => msg.id === assistantId
            ? { ...msg, streaming: false, error: payload.message, truncated: received } : msg));
        }
      }
      setMessages((m) => m.map((msg) => msg.id === assistantId ? { ...msg, streaming: false } : msg));
      setStatus(hadError ? "error" : "idle");
      onDone?.();
    } catch (err) {
      if ((err as Error).name === "AbortError") {
        setMessages((m) => m.map((msg) => msg.id === assistantId ? { ...msg, streaming: false, truncated: true } : msg));
        setStatus("idle");
      } else {
        setStatus("error");
        throw err;
      }
    }
  }, [onDone]);

  return { messages, setMessages, status, send, stop };
}
```

- [ ] **Step 4: beautifului-adapted AI components** (`src/components/ai/`, each with a header comment `// Adapted from beautifului.dev (MIT) by TurboProduct`):

`RunLoader.tsx` — three pulsing dots + elapsed seconds (adapted from their Loading State):

```tsx
import { useEffect, useState } from "react";

export function RunLoader() {
  const [elapsed, setElapsed] = useState(0);
  useEffect(() => {
    const t = setInterval(() => setElapsed((s) => s + 1), 1000);
    return () => clearInterval(t);
  }, []);
  return (
    <div className="flex items-center gap-2 text-muted-foreground text-sm">
      <span className="flex gap-1">
        {[0, 1, 2].map((i) => (
          <span key={i} className="size-1.5 rounded-full bg-current animate-bounce"
            style={{ animationDelay: `${i * 150}ms` }} />
        ))}
      </span>
      thinking{elapsed > 2 ? ` · ${elapsed}s` : ""}
    </div>
  );
}
```

`ThinkingTrace.tsx` — collapsible run-events trace fed by `meta` events (adapted from Thinking); ChatPage collects meta rows into `trace: string[]` state per assistant message ("model call #1 (214 in / 87 out)"). `StreamingText.tsx` — wraps `react-markdown` + `remark-gfm` over the accumulating content; while `streaming`, appends a blinking caret span.

- [ ] **Step 5: ChatPage** — `frontend/src/pages/ChatPage.tsx`:

Layout: shadcn `Sidebar`-style flex — left column (w-72): "New chat" `Button` + `useConversations()` list (`QueryClient` `useQuery(["conversations"])`, click selects, `PATCH`/`DELETE` via dropdown); right column: `ScrollArea` thread rendering `useChat().messages` (user right-aligned bubble, assistant via `StreamingText`, `RunLoader` while streaming with empty content, `ThinkingTrace` above the assistant bubble while `trace.length > 0`, red error line under `error`), composer at bottom (`Textarea` + send `Button`, Enter submits, disabled while `status === "running"`, Stop button calls `stop()` when running). Selecting a conversation loads history via `useQuery(["conversation", id])` → `GET /api/conversations/{id}` and seeds `setMessages`. "New chat" `POST /api/conversations` then selects it. Nav header: link to `/usage`, logout `Button` → `POST /api/auth/logout` + `clear()` + navigate `/login`. Write the full component in the implementation (≈180 lines) following this spec exactly.

- [ ] **Step 6: Gate + commit**

```bash
npm run check && cd .. && git add frontend && git commit -m "feat(frontend): chat UI with SSE streaming, beautifului-style AI states"
```

---

### Task 16: Usage dashboard + README + final verification

**Files:**
- Create: `frontend/src/pages/UsagePage.tsx`, `scripts/smoke.sh`
- Modify: `README.md` (new, repo root)

**Interfaces:**
- Consumes: `GET /api/usage/summary` shape from Task 12.

- [ ] **Step 1: UsagePage** — TanStack Query `useQuery({queryKey: ["usage"], queryFn: () => apiFetch<UsageSummary>("/api/usage/summary?days=30")})`; totals `Card` grid (total cost, input tokens, output tokens, requests — summed from `totals` rows); `recharts` `BarChart` of `daily` (X `day`, Y `costUsd`); table of per-model rows. ≈90 lines, written in full during implementation.

- [ ] **Step 2: `scripts/smoke.sh`** — end-to-end smoke against a running stack:

```bash
#!/usr/bin/env bash
# Smoke test: register, create a conversation, stream one message.
set -euo pipefail
BASE=${BASE:-http://localhost:8080}
EMAIL=${EMAIL:-smoke@test.dev}
res=$(curl -sf -X POST "$BASE/api/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"longenough1\"}")
token=$(echo "$res" | python3 -c 'import json,sys; print(json.load(sys.stdin)["accessToken"])')
conv=$(curl -sf -X POST "$BASE/api/conversations" -H "Authorization: Bearer $token" -H 'Content-Type: application/json' -d '{}' \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')
curl -sf -N -X POST "$BASE/api/conversations/$conv/messages" \
  -H "Authorization: Bearer $token" -H 'Content-Type: application/json' \
  -d '{"content":"Reply with exactly: pong"}' | head -40
echo "smoke ok"
```

(`chmod +x scripts/smoke.sh`; a repeat run re-registers → expect the script to handle 409 by falling back to `/api/auth/login`. Implement that fallback.)

- [ ] **Step 3: README** — repo root: what it is, architecture diagram (copy from spec), `make up` / `make backend` / `make frontend` quickstart, `.env` setup, cost model note (estimates flagged), Phase 2 pointer to the spec, attribution note for beautifului.dev-adapted components.

- [ ] **Step 4: Final gates**

```bash
cd backend && go fmt ./... && go build ./... && go vet ./... && go test ./... && cd ..
cd frontend && npm run check && cd ..
bash scripts/smoke.sh   # with stack running and GEMINI_API_KEY set
```

- [ ] **Step 5: Commit**

```bash
git add . && git commit -m "feat: usage dashboard, smoke script, README — phase 1 complete"
```

---

## Task Dependency Notes

- Tasks 1→2→3 are foundations; 4 and 5 are independent leaves (parallelizable).
- Task 6 needs 5; Task 7 needs 3 + 6's interfaces; Task 8 needs 6 (fakes via `authtest`); Task 9 needs 1 (golem dep); Task 10 needs 7; Task 11 needs 7 + 9 + 10 + 4; Task 12 needs everything backend.
- Tasks 13→14→15→16 are frontend-serial; 13 can start any time after Task 2.

## Self-Review Notes

- **Spec coverage:** [P1] success criteria map to Tasks 2 (compose up), 8+14 (register/login), 9–11+15 (streamed chat, Stop → `RunError.Partial` persisted truncated), 12+16 (usage dashboard with requests), offline tests throughout (3, 5, 6, 9, 11, 13, 15), quality gates in every task. SSE protocol constants in Global Constraints match Task 11's writer.
- **Type consistency:** `chat.Sink` (Task 9) ↔ `sseSink` methods (Task 11); `auth.UserStore`/`RefreshStore` (Task 6) ↔ storage methods (Task 7); `ServerDeps` fields grow per task (8: Auth/Tokens; 10: Convos/Msgs; 11: Agent/Usage/Rates) with final shape in Task 12's wiring — implementers must apply all fields to one struct. Handler stores are interfaces (`ConvoStore`, `MsgStore`, `UsageStore`) so Task 11's fakes satisfy the same surface Task 12 wires to concrete `*storage.*` types.
- **Known risks:** (1) golem v0.7.1 proxy availability — fallback `replace` directive documented in Global Constraints; (2) `testmodel.Scripted` streaming replays `Respond` as deltas — Task 9's delta assertion accepts any fragment count ≥ 1; (3) Task 11's `TestCancelPersistsPartial` expectations were corrected during planning to `TestFailedRunEmitsErrorEvent` (nil `Partial` for pre-evidence failures) — implement the corrected version; (4) shadcn CLI output varies by version — Task 13 treats generated files as scaffold, only hand-written code is normative.
