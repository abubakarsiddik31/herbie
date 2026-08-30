# Golem Chatbot — Design

- **Date:** 2026-08-30 · revised 2026-08-30 (chat-first phasing)
- **Status:** Approved direction (Abu Bakar Siddik): Phase 1 chatbot first, RAG later. Implementation plan next.
- **Scope:** A chat application built on golem (`github.com/abubakarsiddik31/golem` v0.7.1): Go backend, React frontend, JWT auth, local Postgres, Google Gemini for chat, token/cost tracking. **Phase 2** adds RAG: file upload to MinIO, Weaviate vector search, an app-owned Gemini embedding client, and a retrieval tool on the agent.

## Phasing decision

**Phase 1 — chatbot (this plan).** Everything needed to register, chat with streaming answers, and see spend. No vector store, no object storage, no embeddings.

**Phase 2 — RAG.** Documents upload to MinIO, ingestion (extract → chunk → embed → Weaviate), the `search_documents` tool registered on the same shared agent, citations in answers, embedding costs in the ledger, documents UI.

Everything below is labeled **[P1]** (build now) or **[P2]** (build later); [P2] content is design-complete so Phase 2 needs no re-brainstorm.

**Embeddings decision (Abu):** the app embeds itself — a hand-rolled Gemini `batchEmbedContents` client in Phase 2. Golem's absence of a first-class `model.Embedder` stays a framework follow-up (see Golem gaps), not a blocker.

## Goal

**[P1]** A local-first chat app where signed-in users hold streaming conversations with a golem agent backed by Gemini, with every run metered and priced. **[P2]** The same users upload documents (txt/md/pdf/docx) and the agent answers grounded in them via a retrieval tool, citing sources.

**Success criteria — Phase 1**

- `docker compose up -d` starts Postgres (Weaviate/MinIO are composed but dormant behind the `rag` profile); two commands start backend and frontend.
- Register → login → chat with a streamed answer; Stop mid-stream keeps the partial answer marked truncated.
- Usage dashboard shows tokens, request counts, and USD cost per day and per model for chat.
- Backend tests fully offline (golem `testmodel` fakes, `httptest`); frontend tests under vitest/MSW.
- `gofmt`, `go build ./...`, `go vet ./...`, `go test ./...` clean; TypeScript strict, ESLint clean.

**Success criteria — Phase 2 (added)**

- Upload a PDF → `ready`; ask about it → streamed answer cites the document; embedding spend appears in the ledger and dashboard.

## Decisions made (with Abu)

1. **RAG style: agentic tool-based** [P2]. The agent gets a `search_documents` tool and decides when to retrieve; run events stream tool activity to the UI.
2. **Auth: JWT [P1].** Short-lived access JWT (15 min, HS256) held in memory; rotating opaque refresh token (30 d) in an HttpOnly cookie, hashed at rest in Postgres with reuse detection.
3. **Frontend: Vite + React 19 + TypeScript strict + react-router + TanStack Query + Zustand + Tailwind v4 + shadcn/ui [P1];** AI states from beautifului.dev.
4. **Ingestion formats: txt, md, pdf (`ledongthuc/pdf`), docx (zip + `word/document.xml`), 20 MB cap [P2].**
5. **Phasing: chatbot first (P1), RAG later (P2); embeddings are app-owned.**

## Architecture

```text
┌────────────┐  SSE over fetch (JWT header)   ┌──────────────────────────────┐
│ React SPA  │◄──────────────────────────────►│ Go backend (stdlib net/http) │
│ Vite + TS  │   REST /api/*                  │                              │
└────────────┘                                │  httpapi   handlers/mw       │
                                              │  auth      jwt, refresh, bcrypt
                                              │  chat      golem agent (shared)
                                              │  storage   pgx repos            [P2: + minio]
                                              │  cost      rate card, ledger    [P2: + embeddings]
                                              └──────┬───────┬───────┬───────┘
                                                     │       │       │
                                               Postgres  Weaviate  MinIO
                                               (docker)  [P2]      [P2]
                                                     │
                                                     └── Gemini API (chat via golem;
                                                         embeddings direct REST [P2])
```

- **Monorepo:** `backend/` (Go module `github.com/abubakarsiddik31/golem-chatbot`), `frontend/` (Vite), `deploy/docker-compose.yml`, `docs/`.
- **Golem dependency:** `go get github.com/abubakarsiddik31/golem@v0.7.1`; fall back to `replace github.com/abubakarsiddik31/golem => ../golem-agent` if the tag isn't reachable.
- **Backend style:** stdlib `net/http` with Go 1.22+ method/path routing, `log/slog` JSON logging, env-only config, `context.Context` propagation, wrapped errors, no globals — golem's house rules (AGENTS.md).

## Backend layout

```text
backend/
├── cmd/server/main.go           # config → pools → agent → mux → ListenAndServe, graceful shutdown
└── internal/
    ├── config/config.go         # env parsing, validation, defaults
    ├── httpapi/                 # mux, handlers, middleware, SSE protocol
    │   ├── server.go            # route table
    │   ├── middleware.go        # request id, slog, recover, CORS, auth
    │   ├── auth.go              # register/login/refresh/logout/me          [P1]
    │   ├── conversations.go     # CRUD + message history                  [P1]
    │   ├── chat.go              # POST message → SSE stream (agent run)   [P1]
    │   ├── usage.go             # cost summary                            [P1]
    │   └── documents.go         # upload/list/delete, ingest trigger      [P2]
    ├── auth/                    # jwt.go, password.go, refresh.go         [P1]
    ├── chat/                    # agent.go: shared golem agent, Deps, SSE bridge [P1; +search tool P2]
    ├── cost/rates.go            # per-model USD rates, ledger math        [P1]
    ├── storage/
    │   ├── db.go                # pgxpool + goose embedded migrations     [P1]
    │   ├── migrations/*.sql     # users/refresh/conversations/messages/usage [P1; +documents P2]
    │   ├── users.go, refresh_tokens.go, conversations.go, messages.go, usage.go [P1]
    │   ├── documents.go, objectstore.go                                   [P2]
    ├── rag/                     # extract/, chunk, embed, retrieve, ingest [P2]
    └── weaviate/client.go                                                 [P2]
```

## Data model

Postgres (goose migrations, embedded):

**[P1]**

- `users` — `id uuid pk`, `email citext unique`, `password_hash`, `created_at`.
- `refresh_tokens` — `id uuid pk`, `user_id fk`, `token_hash sha256 unique`, `expires_at`, `revoked_at`, `replaced_by`. Rotation: presenting a revoked token revokes the whole family (reuse detection).
- `conversations` — `id uuid pk`, `user_id fk`, `title`, `created_at`, `updated_at`.
- `messages` — `id uuid pk`, `conversation_id fk`, `user_id fk`, `role`, `content text` (what the UI renders), `data jsonb` (the full golem `model.Message`, additive-only JSON — round-trips via `json.Marshal`/`json.Unmarshal`), `input_tokens`, `output_tokens`, `requests int`, `cost_micro_usd bigint`, `truncated bool` (partial stream on client disconnect), `created_at`.
  - **History replay:** `RunStreamWithHistory(history, prompt)` where `history` = `data` of all prior messages, ordered. Golem auto-repairs orphaned tool calls/results, so interrupted runs replay safely.
- `usage_events` — `id bigserial pk`, `user_id fk`, `kind` (`chat` — gains `embedding` in P2), `model`, `conversation_id nullable`, `input_tokens`, `output_tokens`, `requests`, `estimated bool`, `cost_micro_usd bigint`, `created_at`.

**[P2]**

- `documents` — `id uuid pk`, `user_id fk`, `object_key`, `filename`, `mime`, `size_bytes`, `status` (`processing|ready|failed`), `error`, `chunk_count`, `created_at`.

Weaviate [P2]: single collection `DocumentChunk`, BYO vectors (no vectorizer module), properties `user_id`, `document_id`, `doc_title`, `chunk_index`, `content`, `created_at`; 768-dim; **hybrid search** (BM25 + vector) filtered by `user_id`.

MinIO [P2]: bucket `golem-chatbot-documents`, key `{user_id}/{document_id}/{filename}`; originals kept so re-ingestion never needs re-upload.

## Chat pipeline — the golem core [P1]

One **shared agent** is built at startup (golem agents are safe for concurrent use; `Deps` flow per run through `RunContext`, so user scoping needs no per-request construction). Each chat request runs through it, wiring that request's SSE sink via the v0.7.1 run-scoped observer:

```go
type Deps struct {
    UserID         string
    ConversationID string
    Search         func(ctx context.Context, query string, k int) ([]rag.Chunk, error) // [P2]
}

// built once at startup
agent := golem.New[Deps, string](
    geminiClient,                       // providers/gemini, StreamingModel
    golem.DecodeFunc[string](passthrough),
    golem.WithInstructionsFunc[Deps, string](systemPrompt),   // persona + citation rules [P2]
    golem.WithTools[Deps, string](searchTool()),              // [P2]
    golem.WithHistoryProcessor[Deps, string](golem.TrimHistory(40)),
    golem.WithMaxAttempts[Deps, string](2),                   // transient-failure retry
    golem.WithUsageLimit[Deps, string](golem.UsageLimit{      // spend guardrail
        Requests: 12, TotalTokens: 100_000,
    }),
)

// per chat request
result, err := agent.RunStreamWithHistory(ctx,
    golem.RunContext[Deps]{Deps: Deps{UserID: uid, ConversationID: cid}},
    history, prompt,
    sseSink.Delta,
    golem.WithRunObserver(sseSink.RunEvent),  // v0.7.1: per-request event routing
)
```

Phase 2 adds RAG without restructuring: register `searchTool()` at construction, fill `Deps.Search` per run, extend the instructions. The tool: schema `{query string, k int optional (default 5)}`; Exec calls `deps.Search` (Weaviate hybrid, user-filtered) and returns numbered chunks `[1] (filename) text…`; citation format carried in the system prompt.

- **Streaming:** `onDelta` writes `model.Delta.Content` frames to the SSE response; an `onDelta` error (client disconnect) cancels the run.
- **SSE protocol** (consumed with `fetch` + `ReadableStream`, since `EventSource` cannot send the Authorization header):
  - `event: meta` — run lifecycle: `model_start`/`model_end` (attempt usage) in P1; `tool_start`/`tool_end` (name, args) join in P2 — powers "searching documents…" chips.
  - `event: delta` — `{"text": "…"}` text fragments.
  - `event: done` — `{"messageId", "inputTokens", "outputTokens", "requests", "costUsd"}`.
  - `event: error` — `{"stage", "message"}` from `golem.RunError` (stages: model|decode|tool|loop|usage); a client-cancelled run rides `RunError` since v0.7.1 (still matchable with `errors.Is(err, context.Canceled)`) and maps to a quiet stop; tool timeouts report the `tool` stage [P2]. A Gemini stream truncated in transit fails with `DecodeError` (v0.7.1 sentinel check) instead of emitting a silently short answer.
- **Post-run:** persist assistant message (`content` = output, `data` = `result.Messages` tail, usage from `result.Usage`, requests from the observer's `model_end` count), write a `chat` usage event with computed cost, update `conversations.updated_at`, auto-title new conversations from the first user message (first 48 chars; no extra model call).

## Ingestion pipeline [P2]

`POST /api/documents` (multipart): validate size/type → store original in MinIO → row `status=processing` → synchronously (20 MB cap): extract text → chunk (~1000 chars, ~150 overlap, paragraph/heading aware) → embed in batches of ≤ 96 texts (`gemini-embedding-001`, 768-dim, `RETRIEVAL_DOCUMENT`) → upsert to Weaviate → `status=ready` + `chunk_count`. Failure → `status=failed` + `error`, object kept. Delete removes Weaviate objects + MinIO object + row. Queries embed with task type `RETRIEVAL_QUERY`.

## Cost tracking [P1; embeddings half lands P2]

- **Chat:** exact — `result.Usage` (cumulative across turns, retries, corrections) plus request counts from the run observer → one `usage_events` row, `estimated=false`.
- **Embeddings [P2]:** the embedding API returns no usage, so tokens are estimated at `len(text)/4`, `estimated=true` — flagged in UI and API.
- **Rate card** (`internal/cost/rates.go`, USD per 1M tokens, env-overridable, current public list prices): `gemini-2.5-flash` 0.30 in / 2.50 out; `gemini-embedding-001` 0.15 in [P2]. Costs stored as integer `micro_usd`; recomputable from the ledger since raw token counts are kept.
- **API:** `GET /api/usage/summary?days=30` → totals by kind/model + per-day series (+ per-document embedding spend in P2).

## API surface

**[P1]**

```text
POST /api/auth/register        {email, password}
POST /api/auth/login           {email, password}        → {accessToken, user} + refresh cookie
POST /api/auth/refresh         (cookie)                 → {accessToken} + rotated cookie
POST /api/auth/logout          (cookie)                 → revoke
GET  /api/me
GET  /api/conversations | POST | GET /{id} | PATCH /{id} | DELETE /{id}
POST /api/conversations/{id}/messages   {content}       → SSE stream
GET  /api/usage/summary?days=30
GET  /healthz
```

**[P2]** `GET /api/documents | POST /api/documents | DELETE /api/documents/{id}`

Errors: consistent `{error: {code, message}}`; auth middleware on everything except register/login/refresh/healthz.

## Frontend [P1; documents UI joins P2]

- **Stack:** Vite, React 19, TypeScript `strict`, react-router (plain routes), TanStack Query v5 (all REST), Zustand (auth token + user), Tailwind v4 + shadcn/ui, `react-markdown` + GFM for assistant answers, `recharts` for usage, `sonner` toasts.
- **AI states via [Beautiful UI](https://www.beautifului.dev/)** (TurboProduct, MIT, copy-paste primitives — no registry/CLI documented, so we adapt them under `src/components/ai/` with attribution). **P1:** Streaming Text (answer, follow-ups), Thinking (expandable run-events trace), Loading State (pixel-grid loader before first token), Chat composer / Prompt Bar, Sidebar Nav, Code Block. **P2 adds:** Tool Chips (`search_documents`), Task Rows (tool status), Context Cards (rendered sources). Approval Card maps onto golem deferred tools for a future milestone.
- **Auth flow:** access JWT in memory only (Zustand); refresh cookie HttpOnly `SameSite=Strict`, path `/api/auth`. API client retries once through `/api/auth/refresh` on 401, then redirects to login.
- **Chat UI:** sidebar (conversations, new chat), thread with markdown rendering, streaming text via the fetch-SSE reader, Thinking trace from `meta` events, Stop button (AbortController → server ctx cancel → golem run cancels → partial persisted), optimistic user message.
- **Usage UI [P1]:** totals cards (tokens + USD), 30-day bar chart. **[P2]** per-document embedding cost table.
- **Documents UI [P2]:** dropzone upload with progress, status badges, delete with confirmation.
- **Quality gates:** eslint (typescript-eslint, react-hooks), prettier, vitest + React Testing Library + MSW (auth refresh flow, SSE chat reducer, composer); `npm run check` = lint + typecheck + test.

## Infrastructure (`deploy/docker-compose.yml`)

- **[P1]** `postgres:17-alpine` — volume, healthcheck, db/user/pass from `.env` (default `golem_chatbot`/`golem`/`golem`), port 5432.
- **[P2, behind compose profile `rag`]** `weaviate` (pinned tag) — port 8080, persistence volume, `DEFAULT_VECTORIZER_MODULE: none`, auth disabled (local), `QUERY_DEFAULTS_LIMIT=20`; `minio` + `minio/mc` init sidecar — ports 9000/9001, volume, idempotent `mc mb`.
- Backend (`go run ./cmd/server`) and frontend (`npm run dev`) run on the host against compose infra. Prod images out of scope (follow-up).

## Testing strategy

- **Backend — fully offline by default** (golem house rule): agent behavior tested with `testmodel.Scripted` (streaming passthrough; usage accumulation; usage-limit → `StageUsage` + `UsageLimitError` via `errors.As`; client disconnect → `RunError.Partial` persisted with `truncated=true`); auth unit tests (bcrypt, JWT round-trip, refresh rotation + reuse-detection); handlers with `httptest` + fakes. Integration tests (pgx against `TEST_DATABASE_URL`) opt-in and **skip, not fail**, when env is absent. **[P2 adds]** embed client against `httptest` (success, 429 retryable, bad request); chunker table tests; retrieval against Weaviate (opt-in).
- **Checks per change:** `gofmt -w && go build ./... && go vet ./... && go test ./...`; frontend `npm run check`. Conventional Commits.

## Error handling

- Backend: wrap with `%w`, classify at the edge; `golem.RunError` stages map to SSE `error` events; provider 408/429/5xx retried by golem (`WithMaxAttempts(2)` + default backoff); SSE emits `done` only on success.
- Client disconnect → `onDelta` write error → run ctx cancel → run aborts. Since v0.7.1 the failed run keeps its evidence on `RunError.Partial` (`Messages` through the last completed turn, `Usage`, `Requests`/`ToolCalls`), so the handler persists the partial assistant turn directly — content from `Partial.Messages`, usage from `Partial.Usage` — with `truncated=true`. `Partial.Messages` is resume-ready history. A run failing before its first completed turn has nil `Partial`; nothing is persisted.
- A Gemini stream truncated in transit fails with `DecodeError` — surfaced to the UI as an error event with a retry affordance.
- **[P2]** Ingestion is idempotent per document (delete Weaviate objects for `document_id` before upsert).

## Golem gaps observed (framework follow-ups)

Gaps hit while designing this app. **v0.7.1 (shipped 2026-08-30) closed three of them**; the report is kept as the record, with the app's consumption noted.

### Fixed in v0.7.1

- **Error-path evidence** (was: `runLoop` returned a zero `Result` on every error, discarding the partial transcript and spent usage). Now `RunError.Partial *PartialResult` carries `Messages` (resume-ready via `RunWithHistory`), `Usage`, and `Requests`/`ToolCalls` (failed attempts included); cancellation/deadline errors ride `RunError` while staying `errors.Is`-matchable. *App:* partial assistant turns persist straight from `Partial`.
- **Gemini stream terminal integrity** (was: EOF indistinguishable from completion, so truncation billed as a short answer). Now a stream ending without a terminal `finishReason` fails with `DecodeError`. *App:* truncation surfaces as an SSE error with a retry affordance.
- **Run-scoped observers** (was: `WithRunEvents` bound one callback per agent, forcing per-request agent construction). Now `WithRunObserver(onEvent)` is a `RunOption` accepted by every run variant, composing with the agent-level observer. *App:* one shared agent, per-request observers.

### Still open (first-class shapes golem could grow)

1. **Embeddings are absent** (explicit ROADMAP non-goal). *Decision (Abu): the app embeds itself* — Phase 2 ships `internal/rag/embed.go` (Gemini `batchEmbedContents`, mirroring golem's adapter conventions). *First-class:* a `model.Embedder` port (`EmbedDocuments`/`EmbedQuery`, batching, task types) with provider adapters and usage in `model.Usage`. Biggest remaining force-multiplier for RAG apps.
2. **`Result.Usage` is shallow on the success path.** Only Input/Output tokens; request/tool counts exist internally (they surface on `PartialResult`) but aren't returned for successful runs. *First-class:* `Requests`/`ToolCalls` (and `TotalTokens`) on `model.Usage`. *App:* counts requests exactly from the observer's `model_end` events.
3. **History bounding counts messages, not tokens.** `TrimHistory(n)` has no token awareness and no `countTokens` port (Gemini exposes a free `:countTokens` endpoint). *First-class:* a token-budget `HistoryProcessor` and/or per-provider count-tokens helper. *App:* message-count trim (40) as a proxy.
4. **Tool results are text-only.** No images/parts can come back from a tool. Not blocking text-RAG, but a ceiling for document-image tools. *First-class:* `Parts` on tool messages.
5. **Gemini rejects `responseSchema` together with function declarations,** so structured output on Gemini must use output-tool mode (`WithOutputTool`). *First-class:* adapter-level accommodation or a loud guide-level constraint. No impact here (string output).

Minor notes: `UsageLimit` is checked post-response (one response may overshoot — a pre-send `countTokens` check would close it); streamed turns are single-attempt (no fragment replay) — failure is at least explicit thanks to the truncation fix.

## Out of scope (follow-ups)

Prod containerization/CI-CD, conversation summarization beyond trim, sharing/multi-tenancy beyond user filters, per-attempt ledger rows, docx tracked-changes edge cases, additional file formats, rate-limiting, HTTPS/reverse proxy, golem `model.Embedder` port (golem-side).

## Alternatives considered

- **Retrieve-then-prompt RAG** [P2 alternative] (inject chunks via `WithInstructionsFunc` every turn): simpler, fully predictable, but wastes tokens on every message and never exercises golem's tool loop or run events. Chosen against.
- **HTTP Basic auth literally:** incompatible with `EventSource` headers and logout; JWT with rotating refresh chosen by Abu.
- **WebSockets instead of SSE:** bidirectional not needed; SSE over `fetch` keeps JWT header auth and is simpler to proxy.
- **Async ingestion queue [P2 alternative]** (worker + jobs table): better for large files, but a 20 MB cap with batched embeddings completes in seconds; synchronous keeps one request = one outcome. Revisit if the cap grows.
- **Building RAG in Phase 1:** rejected — chat validates the whole vertical slice (auth, streaming, persistence, metering, UI) with none of the vector-store surface area; RAG then lands as one tool + one pipeline on a stable base.
