# golem-chatbot

A local-first chat application built on [golem](https://github.com/abubakarsiddik31/golem) — a Go agent framework by the same author. Register, chat with a streaming agent, and see every run metered: tokens, request counts, and USD cost per day and per model.

## Architecture

```text
┌────────────┐   REST /api/* + SSE    ┌──────────────────────────────────┐
│ React SPA  │◄──────────────────────►│ Go backend (stdlib net/http)     │
│ Vite + TS  │  SSE over fetch (JWT)  │  golem agent → Gemini/OpenAI/    │
└────────────┘                        │  Anthropic (server-configured)   │
                                      └──────┬───────┬───────┬──────┘
                                         Postgres  Weaviate* MinIO*
                                             └─── provider APIs
  * Phase 2c (RAG) — composed behind the `rag` docker profile, dormant until then
```

## Models, settings, and multimodal chat

The model is a **per-conversation setting**, not a server constant. The server
advertises a curated catalog (`backend/internal/chat/catalog.go`) filtered by
which provider keys are configured in the environment — `GEMINI_API_KEY`,
optional `OPENAI_API_KEY` / `ANTHROPIC_API_KEY` (base URLs overridable for
proxies). Every conversation carries its own model, temperature (or provider
default), and system prompt; the composer's settings dialog edits them, and a
model registry resolves each run to a cached provider client. Per-model rates
price the usage ledger, so the dashboard's per-model table stays exact across
providers.

Chat is multimodal: attach up to 4 images (PNG/JPEG/WebP/GIF, ≤ 4 MB each) per
message and any catalog model answers over them via golem's image parts;
attachments persist with the message and replay in history. The composer also
dictates via the browser's Web Speech API (Chrome/Edge/Safari) — speech never
leaves the browser.

Threads behave the way you expect: conversation selection lives in the URL
(`/chat/<id>`), user messages can be edited and resent (truncating what
followed), the last answer can be regenerated, code blocks and messages have
copy buttons, and every answer shows its tokens, cost, and model.

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

Chat token counts are exact — taken from golem's per-run `Usage` — and prices are computed from rate-card env vars, overridable in `.env`: `CHAT_INPUT_USD_PER_MTOK` / `CHAT_OUTPUT_USD_PER_MTOK` (defaults **$0.30 / $2.50 per 1M tokens**, Gemini 2.5 Flash class). Costs are stored rounded to 5 decimal places; the dashboard shows 4. Embedding costs (RAG) land in the same ledger as exact `embedding` rows, priced by `EMBEDDING_INPUT_USD_PER_MTOK` (default **$0.15 per 1M tokens**).

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

## Custom tools (Phase 2a)

Each user can register HTTP API tools from the **Tools** page; the chat agent
advertises and executes them on every message. The page ships a gallery of 18
prebuilt templates (weather, GitHub, Hacker News, Wikipedia, RSS, FX/crypto
rates, and more — free, keyless, read-only APIs) that prefill the editor; a
vitest suite validates every template against the same authoring rules the
backend enforces. A tool is a name, a model-facing
description, a method + URL template (`{{param}}` path placeholders, query
params appended, optional JSON body template), typed params
(`string`/`number`/`boolean`, `path`/`query`), and static headers (secrets are
stored server-side, masked as `••••` in the API, and never sent to the model).
`Ask before running` gates a tool behind golem's deferred-tools flow: the chat
pauses, an approval card appears, and Approve/Deny resumes the run.

Execution is guarded and graceful: DNS-level SSRF block of loopback/private/
link-local hosts (`TOOL_ALLOW_PRIVATE_HOSTS=true` disables the guard for
dev), request timeout and size caps (`TOOL_HTTP_TIMEOUT`, `TOOL_HTTP_MAX_BYTES`,
`TOOL_RESULT_MAX_BYTES`), and errors returned to the model as text results so a
dead API degrades to a graceful answer instead of failing the run.

Tool lifecycle persists every run message (tool calls included) so history
replay and resume work across turns; provider-synthesized call IDs (Gemini's
`call-1`, …) are rewritten to conversation-unique IDs at persistence time
because they otherwise collide across runs. Schema migrations are generated
with [Atlas](https://atlasgo.io) from `backend/internal/storage/schema.hcl`
(the schema source of truth) into the goose-format files goose applies at
startup:

```bash
/opt/homebrew/bin/atlas migrate diff <name> \
  --dir file://backend/internal/storage/migrations --dir-format goose \
  --to file://backend/internal/storage/schema.hcl \
  --dev-url "docker://postgres/16/dev?search_path=public"
```

## RAG (Phase 2c)

Upload documents (txt, md, pdf, docx — 20 MB cap) on the **Documents** page; they are stored in MinIO, extracted, chunked (~1000 chars, paragraph-aware, ~150 char overlap), and embedded with **golem v0.7.5's embeddings port** (`gemini-embedding-001`, 768 dims) into Weaviate. The chat agent gets a `search_documents` tool and decides when to retrieve; answers cite `[n]` sources, rendered as collapsible source cards. Every query embedding is metered exactly (`kind=embedding`, `estimated=false`) and the usage page breaks spend down per document.

**Enable it** — the infra ships dormant behind the compose `rag` profile:

```bash
docker compose -f deploy/docker-compose.yml --profile rag up -d
# then set in .env:
RAG_ENABLED=true
```

| Env var | Default | Purpose |
|---|---|---|
| `RAG_ENABLED` | `false` | Master switch; requires `GEMINI_API_KEY` |
| `WEAVIATE_URL` | `http://localhost:8081` | Vector store (compose maps host **8081** — the backend owns 8080) |
| `MINIO_ENDPOINT` | `localhost:9000` | Object store for originals |
| `MINIO_ACCESS_KEY` / `MINIO_SECRET_KEY` | `golem` / `golem1234` | MinIO creds |
| `DOCUMENTS_BUCKET` | `golem-chatbot-documents` | Bucket (created by compose init) |
| `EMBEDDING_MODEL` | `gemini-embedding-001` | Embedding model |
| `EMBEDDING_DIMS` | `768` | Vector width |
| `EMBEDDING_BATCH` | `96` | Texts per embed call |
| `EMBEDDING_INPUT_USD_PER_MTOK` | `0.15` | Ledger rate |
| `MAX_UPLOAD_BYTES` | `20971520` | Upload cap (20 MB) |

API: `GET /api/documents`, `POST /api/documents` (multipart `file`), `DELETE /api/documents/{id}`; all 503 with `rag_disabled` when the stack is off. Ingestion is idempotent per document (stale vectors are deleted before re-upsert); a failed ingest keeps the row (`status=failed`) and the original object.

Retrieval quality is covered by a live golden-query eval: `BASE=http://localhost:8080 ./scripts/rag-eval.sh` uploads three fixture docs (`backend/internal/rag/testdata/eval/`, one topic each with a unique canary sentence), asks one question per doc, and passes only if every answer top-cites the expected document and carries a matching `[n]` bracket citation.
