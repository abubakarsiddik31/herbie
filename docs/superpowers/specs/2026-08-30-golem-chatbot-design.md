# Golem Chatbot — Design

- **Date:** 2026-08-30
- **Status:** Draft — pending review by Abu Bakar Siddik
- **Scope:** Chat + RAG web application built on golem (`github.com/abubakarsiddik31/golem` v0.7.1): Go backend, React frontend, JWT auth, local Postgres, local Weaviate, Google Gemini chat + embeddings, token/cost tracking, file upload to local MinIO.

## Goal

A local-first chat application where signed-in users upload documents (txt/md/pdf/docx), and chat with an agent that answers questions grounded in those documents via a retrieval tool. Every model and embedding call is metered and priced so users can see what they spent, per conversation and in total.

**Success criteria**

- `docker compose up` brings up Postgres, Weaviate, MinIO; two commands start backend and frontend.
- Register → login → upload a PDF → ask a question about it → streamed answer cites the document.
- Usage dashboard shows tokens and USD cost per day and per model, chat and embeddings separated.
- Backend test suite is fully offline (golem `testmodel` fakes, `httptest`); frontend tests run under vitest/MSW.
- `gofmt`, `go build ./...`, `go vet ./...`, `go test ./...` clean; TypeScript strict, ESLint clean.

## Decisions made (with Abu)

1. **RAG style: agentic tool-based.** The agent gets a `search_documents` tool and decides when to retrieve. Golem's tool loop and run events drive the UI ("searching documents…").
2. **Auth: JWT.** Short-lived access JWT (15 min, HS256) held in memory; rotating opaque refresh token (30 d) in an HttpOnly cookie, hashed at rest in Postgres with reuse detection.
3. **Frontend:** Vite + React 19 + TypeScript strict + react-router + TanStack Query + Zustand + Tailwind v4 + shadcn/ui.
4. **Ingestion formats:** txt, md, pdf (`ledongthuc/pdf`), docx (zip + `word/document.xml`). 20 MB upload cap.

## Architecture

```text
┌────────────┐  SSE over fetch (JWT header)   ┌──────────────────────────────┐
│ React SPA  │◄──────────────────────────────►│ Go backend (stdlib net/http) │
│ Vite + TS  │   REST /api/*                  │                              │
└────────────┘                                │  httpapi   handlers/mw       │
                                              │  auth      jwt, refresh, bcrypt
                                              │  chat      golem agent + tools
                                              │  rag       extract→chunk→embed→retrieve
                                              │  storage   pgx repos, minio
                                              │  cost      rate card, ledger
                                              └──────┬───────┬───────┬───────┘
                                                     │       │       │
                                               Postgres  Weaviate  MinIO
                                               (docker)  (docker)  (docker)
                                                     │
                                                     └── Gemini API (chat via golem,
                                                         embeddings direct REST)
```

- **Monorepo:** `backend/` (Go module `github.com/abubakarsiddik31/golem-chatbot`), `frontend/` (Vite), `deploy/docker-compose.yml`, `docs/`.
- **Golem dependency:** `go get github.com/abubakarsiddik31/golem@v0.7.1`. If the tag isn't reachable when we start, use `replace github.com/abubakarsiddik31/golem => ../golem-agent` (the golem-lab pattern) until it is.
- **Backend style:** stdlib `net/http` with Go 1.22+ method/path routing, `log/slog` JSON logging, env-only config (no config files), `context.Context` propagation, wrapped errors, no globals — matching golem's house rules (AGENTS.md).
- **Golem is used for chat only.** Embeddings are an explicit golem non-goal (docs/ROADMAP.md), so `internal/rag/embed.go` calls `models/{model}:batchEmbedContents` directly with stdlib `net/http`, mirroring golem's adapter conventions (validated Config, classified errors implementing `Retryable() bool`).

## Backend layout

```text
backend/
├── cmd/server/main.go           # config → pools → mux → ListenAndServe, graceful shutdown
└── internal/
    ├── config/config.go         # env parsing, validation, defaults
    ├── httpapi/                 # mux, handlers, middleware, SSE protocol
    │   ├── server.go            # route table
    │   ├── middleware.go        # request id, slog, recover, CORS, auth
    │   ├── auth.go              # register/login/refresh/logout/me
    │   ├── conversations.go     # CRUD + message history
    │   ├── chat.go              # POST message → SSE stream (agent run)
    │   ├── documents.go         # upload/list/delete, ingest trigger
    │   └── usage.go             # cost summary
    ├── auth/                    # jwt.go, password.go, refresh.go (rotation + reuse detection)
    ├── chat/                    # agent.go: golem.New construction, Deps, search tool, SSE bridge
    ├── rag/
    │   ├── extract/             # txt/md/pdf/docx → plain text
    │   ├── chunk.go             # ~1000 chars, ~150 overlap, paragraph/heading aware
    │   ├── embed.go             # Gemini batchEmbedContents client (768-dim, task types)
    │   ├── retrieve.go          # Weaviate hybrid search filtered by user_id
    │   └── ingest.go            # orchestration: extract→chunk→embed→upsert→status
    ├── storage/
    │   ├── db.go                # pgxpool + goose embedded migrations
    │   ├── migrations/*.sql
    │   ├── users.go, refresh_tokens.go, conversations.go, messages.go,
    │   ├── documents.go, usage.go
    │   └── objectstore.go       # minio-go wrapper (bucket ensure, put/get/delete)
    ├── cost/rates.go            # per-model USD rates, env-overridable, ledger math
    └── weaviate/client.go       # weaviate-go-client v4 wrapper (schema ensure, upsert, query)
```

## Data model

Postgres (goose migrations, embedded):

- `users` — `id uuid pk`, `email citext unique`, `password_hash`, `created_at`.
- `refresh_tokens` — `id uuid pk`, `user_id fk`, `token_hash sha256 unique`, `expires_at`, `revoked_at`, `replaced_by`. Rotation: presenting a revoked token revokes the whole family (reuse detection).
- `conversations` — `id uuid pk`, `user_id fk`, `title`, `created_at`, `updated_at`.
- `messages` — `id uuid pk`, `conversation_id fk`, `user_id fk`, `role`, `content text` (what the UI renders), `data jsonb` (the full golem `model.Message`, additive-only JSON — round-trips via `json.Marshal`/`json.Unmarshal`), `input_tokens`, `output_tokens`, `cost_micro_usd bigint` (same unit as the ledger), `truncated bool` (partial stream on client disconnect), `created_at`.
  - **History replay:** `RunWithHistory(history, prompt)` where `history` = `data` of all prior messages, ordered. Golem auto-repairs orphaned tool calls/results, so interrupted runs replay safely.
- `documents` — `id uuid pk`, `user_id fk`, `object_key` (MinIO), `filename`, `mime`, `size_bytes`, `status` (`processing|ready|failed`), `error`, `chunk_count`, `created_at`.
- `usage_events` — `id bigserial pk`, `user_id fk`, `kind` (`chat|embedding`), `model`, `conversation_id nullable`, `document_id nullable`, `input_tokens`, `output_tokens`, `estimated bool`, `cost_micro_usd bigint`, `created_at`.

Weaviate: single collection `DocumentChunk`, BYO vectors (no vectorizer module), properties: `user_id`, `document_id`, `doc_title`, `chunk_index`, `content`, `created_at`; 768-dim vectors; **hybrid search** (BM25 + vector) filtered by `user_id`. (Native multi-tenancy is the scale-up path; an equality filter is sufficient and simpler here.)

MinIO: bucket `golem-chatbot-documents`, object key `{user_id}/{document_id}/{filename}`. Originals are kept so re-ingestion (new chunker/embedder) never needs a re-upload.

## Chat pipeline (the golem core)

One **shared agent** is built at startup (golem agents are safe for concurrent use; `Deps` flow per run through `RunContext`, so user scoping needs no per-request construction). Each chat request runs through it, wiring that request's SSE sink via the v0.7.1 run-scoped observer:

```go
type Deps struct {
    UserID         string
    ConversationID string
    Search         func(ctx context.Context, query string, k int) ([]rag.Chunk, error)
}

// built once at startup
agent := golem.New[Deps, string](
    geminiClient,                       // providers/gemini, StreamingModel
    golem.DecodeFunc[string](passthrough),
    golem.WithInstructionsFunc[Deps, string](systemPrompt),   // persona + citation rules
    golem.WithTools[Deps, string](searchTool()),
    golem.WithHistoryProcessor[Deps, string](golem.TrimHistory(40)),
    golem.WithMaxAttempts[Deps, string](2),                    // transient-failure retry
    golem.WithUsageLimit[Deps, string](golem.UsageLimit{       // spend guardrail
        Requests: 12, TotalTokens: 100_000,
    }),
)

// per chat request
result, err := agent.RunStreamWithHistory(ctx,
    golem.RunContext[Deps]{Deps: Deps{UserID: uid, ConversationID: cid, Search: userSearch}},
    history, prompt,
    sseSink.Delta,
    golem.WithRunObserver(sseSink.RunEvent),  // v0.7.1: per-request event routing
)
```

- `search_documents` tool: schema `{query string, k int optional (default 5)}`. Exec calls `deps.Search` (Weaviate hybrid, user-filtered) and returns numbered chunks `[1] (filename) text…`. Tool results are text-only in golem — the citation format is carried in the system prompt.
- **Streaming:** `RunStreamWithHistory(ctx, runCtx, history, prompt, onDelta)`; `onDelta` writes `model.Delta.Content` frames to the SSE response. Returning an error from `onDelta` (client disconnect) cancels the run.
- **SSE protocol** (consumed with `fetch` + `ReadableStream`, since `EventSource` cannot send the Authorization header):
  - `event: meta` — run lifecycle: `tool_start`/`tool_end` (name, args), `model_end` (attempt usage) — powers "searching documents…" chips.
  - `event: delta` — `{"text": "…"}` text fragments.
  - `event: done` — `{"messageId", "inputTokens", "outputTokens", "costUsd"}`.
  - `event: error` — `{"stage", "message"}` from `golem.RunError` (stages: model|decode|tool|loop|usage); a client-cancelled run rides `RunError` since v0.7.1 (still matchable with `errors.Is(err, context.Canceled)`) and maps to a quiet stop; tool timeouts report the `tool` stage. A Gemini stream truncated in transit fails with `DecodeError` (v0.7.1 sentinel check) instead of emitting a silently short answer.
- **Post-run:** persist assistant message (`content` = output, `data` = `result.Messages` tail, usage from `result.Usage`), write a `chat` usage event with computed cost and the request count taken from the observer's `model_end` events (exact, retries included), update `conversations.updated_at`, auto-title new conversations from the first user message (cheap: first 48 chars; no extra model call).

## Ingestion pipeline

`POST /api/documents` (multipart): validate size/type → store original in MinIO → create row `status=processing` → **synchronously** (MVP; uploads capped at 20 MB): extract text → chunk → embed in batches of ≤ 96 texts (`RETRIEVAL_DOCUMENT`) → upsert to Weaviate → `status=ready` + `chunk_count`. Parse/extract failure → `status=failed` + `error`, object kept for debugging. Delete removes Weaviate objects + MinIO object + row.

Embeddings: `gemini-embedding-001`, `outputDimensionality=768`; queries use task type `RETRIEVAL_QUERY`, documents `RETRIEVAL_DOCUMENT`.

## Cost tracking

- **Chat:** exact — `result.Usage` (cumulative input/output across turns, retries, corrections) per run → one `usage_events` row, `estimated=false`.
- **Embeddings:** the Gemini embedding API returns no usage, so tokens are estimated at `len(text)/4` per input, `estimated=true` — flagged in UI and API.
- **Rate card** (`internal/cost/rates.go`, USD per 1M tokens, env-overridable, current public list prices): `gemini-2.5-flash` 0.30 in / 2.50 out; `gemini-embedding-001` 0.15 in. Costs stored as integer `micro_usd` to avoid float drift; recomputable from ledger since rates live next to raw token counts.
- **API:** `GET /api/usage/summary?days=30` → totals by kind/model + per-day series + per-document embedding spend.

## API surface

```text
POST /api/auth/register        {email, password}
POST /api/auth/login           {email, password}        → {accessToken, user} + refresh cookie
POST /api/auth/refresh         (cookie)                 → {accessToken} + rotated cookie
POST /api/auth/logout          (cookie)                 → revoke
GET  /api/me
GET  /api/conversations | POST | GET /{id} | PATCH /{id} | DELETE /{id}
POST /api/conversations/{id}/messages   {content}       → SSE stream
GET  /api/documents | POST /api/documents | DELETE /api/documents/{id}
GET  /api/usage/summary?days=30
GET  /healthz
```

Errors: consistent `{error: {code, message}}`; auth middleware on everything except register/login/refresh/healthz.

## Frontend

- **Stack:** Vite, React 19, TypeScript `strict`, react-router (data mode off — plain routes), TanStack Query v5 (all REST), Zustand (auth token + user), Tailwind v4 + shadcn/ui, `react-markdown` + GFM for assistant answers, `recharts` for usage, `sonner` toasts.
- **AI states via [Beautiful UI](https://www.beautifului.dev/)** (TurboProduct, MIT, copy-paste primitives — no registry/CLI documented, so we adapt them under `src/components/ai/` with attribution): Streaming Text (answer with inline sources, actions, follow-ups), Tool Chips (`search_documents` activity), Task Rows (per-run tool status: running/failed/completed), Thinking (expandable run-events trace), Loading State (pixel-grid loader while awaiting first token), plus their Chat composer / Prompt Bar / Sidebar Nav / Code Block / Context Cards (rendered retrieved sources). The Approval Card maps one-to-one onto golem's deferred tools for a future human-in-the-loop milestone.
- **Auth flow:** access JWT in memory only (Zustand, not localStorage); refresh cookie HttpOnly `SameSite=Strict`, path `/api/auth`. API client retries once through `/api/auth/refresh` on 401, then redirects to login.
- **Chat UI:** sidebar (conversations, new chat), thread with markdown rendering and citation rendering from `[n] (filename)` markers, tool-activity chips from `meta` events, streaming text via the fetch-SSE reader, Stop button (AbortController → server ctx cancel → golem run cancels), optimistic user message.
- **Documents UI:** dropzone upload with progress, list with status badges (`processing/ready/failed`), delete with confirmation.
- **Usage UI:** totals cards (tokens + USD by kind), 30-day bar chart, per-document embedding cost table.
- **Quality gates:** eslint (typescript-eslint, react-hooks), prettier, vitest + React Testing Library + MSW (auth refresh flow, SSE chat reducer, upload form); `npm run check` = lint + typecheck + test.

## Infrastructure (`deploy/docker-compose.yml`)

- `postgres:17-alpine` — volume, healthcheck, db/user/pass from `.env` (default `golem_chatbot`/`golem`/`golem`), port 5432.
- `weaviate` (pinned `semitechnologies/weaviate` tag) — port 8080, `PERSISTENCE_DATA_PATH` volume, `DEFAULT_VECTORIZER_MODULE: none`, auth disabled (local only), `QUERY_DEFAULTS_LIMIT=20`.
- `minio` + `minio/mc` init sidecar — ports 9000 (API) / 9001 (console), volume, `mc mb` idempotent bucket creation.
- Backend (`go run ./cmd/server`) and frontend (`npm run dev`) run on the host against compose infra. Prod images are out of scope for MVP (noted as follow-up).

## Testing strategy

- **Backend — fully offline by default** (golem house rule): agent behavior tested with `testmodel.Scripted` (search tool invoked → grounded answer; usage-limit → `StageUsage` + `UsageLimitError` via `errors.As`; tool failure → `StageTool`); embed client against `httptest` servers (success, 429 retryable, bad request); auth unit tests (bcrypt, JWT round-trip, refresh rotation + reuse-detection); handlers with `httptest` + fakes. Integration tests (pgx against `TEST_DATABASE_URL`, Weaviate, MinIO) are opt-in and **skip, not fail**, when env is absent.
- **Checks per change:** `gofmt -w && go build ./... && go vet ./... && go test ./...`; frontend `npm run check`. Conventional Commits.

## Error handling

- Backend: wrap with `%w`, classify at the edge; `golem.RunError` stages map to SSE `error` events; provider 408/429/5xx retried by golem (`WithMaxAttempts(2)` + default backoff); SSE emits `done` only on success.
- Client disconnect → `onDelta` write error → run ctx cancel → run aborts. Since v0.7.1 the failed run keeps its evidence on `RunError.Partial` (`Messages` through the last completed turn, `Usage`, `Requests`/`ToolCalls`), so the handler persists the partial assistant turn directly — content from `Partial.Messages`, usage from `Partial.Usage` — with `truncated=true`. `Partial.Messages` is resume-ready history (repair synthesizes results for orphaned tool calls). A run that fails before completing a model turn has nil `Partial`; nothing is persisted for it.
- A Gemini stream truncated in transit now fails with `DecodeError` instead of returning silently short text — surfaced to the UI as an error event with a retry affordance.
- Ingestion is idempotent per document (delete Weaviate objects for `document_id` before upsert).

## Golem gaps observed (framework follow-ups)

Gaps hit while designing this app. **v0.7.1 (shipped 2026-08-30) closed three of them**; the report below is kept as the record, with the app's consumption noted.

### Fixed in v0.7.1

- **Error-path evidence** (was: `runLoop` returned a zero `Result` on every error, discarding the partial transcript and spent usage). Now `RunError.Partial *PartialResult` carries `Messages` (through the last completed turn, resume-ready via `RunWithHistory`), `Usage`, and `Requests`/`ToolCalls` (failed attempts included); cancellation/deadline errors ride `RunError` while staying `errors.Is`-matchable. *App:* partial assistant turns persist straight from `Partial` — no delta reconstruction.
- **Gemini stream terminal integrity** (was: EOF indistinguishable from completion, so truncation billed as a short answer). Now a stream ending without a terminal `finishReason` fails with `DecodeError`, matching the other adapters' sentinel checks. *App:* truncation surfaces as an SSE error with a retry affordance.
- **Run-scoped observers** (was: `WithRunEvents` bound one callback per agent, forcing per-request agent construction in servers). Now `WithRunObserver(onEvent)` is a `RunOption` accepted by every run variant, composing with the agent-level observer. *App:* one shared agent, per-request observers — see the chat pipeline.

### Still open (first-class shapes golem could grow)

1. **Embeddings are absent** (explicit ROADMAP non-goal). The app hand-rolls a Gemini `batchEmbedContents` client (`internal/rag/embed.go`) mirroring golem's adapter conventions. *First-class:* a `model.Embedder` port (`EmbedDocuments`/`EmbedQuery`, batching, task types) with provider adapters and usage surfaced in `model.Usage`. Biggest remaining force-multiplier for RAG apps — the ROADMAP's "revisit when users ask" trigger.
2. **`Result.Usage` is shallow on the success path.** Only Input/Output tokens; the request/tool-call counts exist internally (they now surface on `PartialResult`) but are not returned for successful runs. *First-class:* add `Requests`/`ToolCalls` (and `TotalTokens`) to `model.Usage`. *App:* counts requests exactly from the observer's `model_end` events.
3. **History bounding counts messages, not tokens.** `TrimHistory(n)` has no token awareness and no `countTokens` port (Gemini exposes a free `:countTokens` endpoint). *First-class:* a token-budget `HistoryProcessor` and/or per-provider count-tokens helper. *App:* message-count trim (40) as a proxy.
4. **Tool results are text-only.** No images/parts can come back from a tool (multimodal input is prompt-side only). Not blocking text-RAG, but a ceiling for screenshot/document-image tools. *First-class:* `Parts` on tool messages.
5. **Gemini rejects `responseSchema` together with function declarations,** so structured output on Gemini must use output-tool mode (`WithOutputTool`) — golem-lab already works around this. *First-class:* adapter-level accommodation or a loud, guide-level constraint. No impact here (string output).

Minor notes: `UsageLimit` is checked post-response (one response may overshoot — a pre-send `countTokens` check would close it); streamed turns are single-attempt (no fragment replay) — failure is now at least explicit thanks to the truncation fix.

## Out of scope (follow-ups)

Prod containerization/CI-CD, conversation summarization beyond trim, sharing/multi-tenancy beyond user filters, streaming tool-progress token accounting per attempt in the ledger, docx tracked-changes edge cases, other file formats, rate-limiting, HTTPS/reverse proxy.

## Alternatives considered

- **Retrieve-then-prompt RAG** (inject chunks via `WithInstructionsFunc` every turn): simpler, fully predictable, but wastes tokens on every message and never exercises golem's tool loop or run events. Chosen against.
- **HTTP Basic auth literally:** incompatible with `EventSource` headers and logout; JWT with rotating refresh chosen by Abu.
- **WebSockets instead of SSE:** bidirectional not needed; SSE over `fetch` keeps the JWT header auth and is simpler to proxy.
- **Async ingestion queue (worker + jobs table):** better for large files, but a 20 MB cap with batched embeddings completes in seconds; synchronous keeps one request = one outcome. Revisit if the cap grows.
