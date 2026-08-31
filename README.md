# golem-chatbot

A local-first chat application built on [golem](https://github.com/abubakarsiddik31/golem) — a Go agent framework by the same author. Register, chat with a streaming Gemini-backed agent, and see every run metered: tokens, request counts, and USD cost per day and per model.

## Architecture

```text
┌────────────┐   REST /api/* + SSE    ┌──────────────────────────────┐
│ React SPA  │◄──────────────────────►│ Go backend (stdlib net/http) │
│ Vite + TS  │  SSE over fetch (JWT)  │  golem agent → Gemini        │
└────────────┘                        └──────┬───────┬───────┬──────┘
                                             │       │       │
                                         Postgres  Weaviate* MinIO*
                                             └─── Gemini API
  * Phase 2 (RAG) — composed behind the `rag` docker profile, dormant in Phase 1
```

## Quickstart

```bash
cp .env.example .env   # fill in GEMINI_API_KEY
make up                # Postgres via docker compose
make backend           # go run ./cmd/server  (default :8080)
make frontend          # vite dev server      (default :5173)
```

Then open http://localhost:5173 and register an account.

Note: the compose Postgres is mapped to host port **5433** (not 5432), so it can coexist with a local Postgres — `DATABASE_URL` in `.env` already points at 5433.

## Cost model

Chat token counts are exact — taken from golem's per-run `Usage` — and prices are computed from rate-card env vars, overridable in `.env`: `CHAT_INPUT_USD_PER_MTOK` / `CHAT_OUTPUT_USD_PER_MTOK` (defaults **$0.30 / $2.50 per 1M tokens**, Gemini 2.5 Flash class). Costs are stored rounded to 5 decimal places; the dashboard shows 4. Phase 2 will add embedding costs to the same ledger.

## Development

```bash
make check       # backend: go fmt, build, vet, test (fully offline — golem testmodel fakes)
make fe-check    # frontend: oxlint, tsc -b, vitest (alias for the line below)
cd frontend && npm run check
scripts/smoke.sh # end-to-end smoke against a running stack: register → conversation → streamed reply
```

The smoke script uses `BASE` (default `http://localhost:8080`) and `EMAIL` (default `smoke@test.dev`); a repeat run falls back to login when the email is already registered.

## Attribution

The AI-state components in `frontend/src/components/ai/` (streaming text, thinking trace, run loader) are adapted from [beautifului.dev](https://www.beautifului.dev/) (TurboProduct, MIT) with modifications for golem's run-event model.

## Phase 2

RAG — document upload to MinIO, ingestion into Weaviate, an app-owned Gemini embedding client, and a `search_documents` retrieval tool on the agent — is design-complete. See `docs/superpowers/specs/2026-08-30-golem-chatbot-design.md`.
