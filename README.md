# Herbie

> **The free, flexible open-source AI platform.**  
> Built as an open alternative to ChatGPT Plus and Claude Pro for developers and power users who want complete flexibility, multi-model choice, visual automation workflows, custom tools, and self-hosted RAG without vendor lock-in.

Powered under the hood by [golem](https://github.com/abubakarsiddik31/golem) (a high-performance Go agent framework) with a fast React 19 + TypeScript SPA frontend.

### Why Herbie?
- 🌐 **Multi-Model Freedom**: Chat with Gemini, OpenAI, Claude, or local proxies — switch models per-conversation without losing context.
- 📁 **Project Workspaces**: Claude-style scoped projects with attached files, persistent guidelines, and isolated RAG knowledge.
- ⚡ **Visual Workflows**: Node-based automation canvas (n8n-style via `@xyflow/react`) — execute triggers, HTTP requests, LLMs, and expose workflows directly as native agent tools!
- 🛠️ **Custom API Tools & Web Search**: Register any HTTP API with JSON parameters and optional human-in-the-loop approval cards, plus built-in Brave/Tavily web search.
- 📚 **Production RAG**: Document uploads (PDF, DOCX, MD, TXT), chunking, Weaviate vector embeddings, listwise LLM reranking, and exact inline citations `[n]`.
- 📊 **Exact Cost & Token Metering**: Real-time per-run token counters, daily spend ledgers, and exact USD cost calculations down to $0.00001.
- 🎙️ **Multimodal & Voice**: Image attachments (up to 4 images) and speech-to-text dictation via browser Web Speech API.

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

## Social login

Google and GitHub sign-in are optional and off unless configured — the login
UI only shows providers with both a client ID and secret set:

```bash
OAUTH_GOOGLE_CLIENT_ID=…      # Google Cloud console → Credentials → OAuth client
OAUTH_GOOGLE_CLIENT_SECRET=…
OAUTH_GITHUB_CLIENT_ID=…      # GitHub Settings → Developer settings → OAuth Apps
OAUTH_GITHUB_CLIENT_SECRET=…
# OAUTH_REDIRECT_BASE=https://chat.example.com  # defaults to http://localhost:$APP_PORT
```

Register each provider's callback URL (`<base>/api/auth/oauth/<google|github>/callback`)
in its console. The flow is authorization-code + PKCE with a sealed `state`
cookie; verified provider emails link to an existing password account or
provision a passwordless one. The callback hands the SPA a single-use code
(`POST /api/auth/oauth/consume`) — tokens never travel in the URL. With
nothing configured, `GET /api/auth/providers` returns an empty list and auth
stays email+password only.

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

### Built-in web search

In addition to custom tools, the server bundles a built-in `web_search` tool
powered by Tavily or Brave Search. Set `TAVILY_API_KEY=...` or `BRAVE_API_KEY=...`
in `.env`. When configured, the model can search the public web for real-time facts
and recent developments; searches trigger an approval card by default
(`WEB_SEARCH_REQUIRE_APPROVAL=true`), and the answer cites web sources with
markdown links. When unset, web search remains disabled.

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

## Visual Workflows (n8n-style Automation)

Create and execute multi-step automation flows using an interactive node-based canvas powered by `@xyflow/react` on the **Workflows** page (`/workflows`):

- **Triggers**: Manual run / test payloads, unique public Webhook URLs (`/api/webhooks/:slug` with optional secret verification), or direct chat agent invocation.
- **External Tools & Connectors**: Universal HTTP Request node (GET/POST/PUT/DELETE with Bearer, Basic, API Key auth and body templates), Golem Custom Tools bridge, GitHub (create issues/comments), Slack, and Discord.
- **AI & LLM Nodes**: Prompt catalog models (Gemini, OpenAI, Anthropic) with prompt templates, system instructions, and structured JSON output extraction.
- **Logic & Control Flow**: Conditional branching (If/Else evaluation with True/False path routing), field mapping transforms (`code_transform`), and execution delays.
- **Agent Tool Bridge**: Toggle `Expose as Agent Tool` on any workflow to allow the main conversational chat assistant to autonomously execute the workflow as a native tool during chat!
- **Tracing & History**: Inspect past execution runs, durations, node-by-node status, inputs, and outputs.

## RAG (Phase 2c)

The agent owns retrieval — there is no fixed pipeline. On every turn it
formulates its own queries, calls `search_documents` (optionally scoped with
`documentIds` taken from earlier results), refines with narrower queries when
results look thin, and answers only from tool evidence.

Upload documents (txt, md, pdf, docx — 20 MB cap) on the **Documents** page; they are stored in MinIO, extracted into headed sections, chunked (~512 tokens / ~2000 chars with ~64-token overlap, recursive headings → paragraphs → sentences, carrying heading + page metadata via `CHUNK_TARGET_TOKENS` / `CHUNK_OVERLAP_TOKENS`), and embedded with **golem v0.7.6's embeddings port** (`gemini-embedding-001`, 768 dims) into Weaviate.

Each retrieval call runs: query embedding → hybrid over-retrieve (`RETRIEVAL_ALPHA` default 0.5, `fusionType: rankedFusion`, `RETRIEVE_MULT`×k capped at `RERANK_MAX_CANDIDATES`) → neighbor-window expansion (`EXPAND_BEFORE`/`EXPAND_AFTER`, default 1/1; the core chunk stays the citation unit) → listwise LLM rerank (`RERANK_MODEL` default `gemini-2.5-flash`, `RERANK_ENABLED=false` disables) → top-k with `[n] (Title § Heading, p.N)` citations, numbers restarting at 1 on every call.

Citation discipline is hard: every claim drawn from documents carries its
bracket number, only bracket numbers shown in a tool result are cited, and
missing evidence is admitted instead of guessed. The Sources card mirrors the
tool receipt (title, heading, page, snippet).

Long threads compact: histories over `COMPACTION_THRESHOLD_TOKENS` (default
40000) keep the last `COMPACTION_KEEP_RECENT` (10) turns verbatim while older
turns compress into the run instructions via `COMPACTION_MODEL` (summary cap
`COMPACTION_SUMMARY_TOKENS`). Compaction is ephemeral — history rows and role
alternation are untouched.

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
| `RETRIEVAL_ALPHA` | `0.5` | Hybrid BM25/vector blend (0..1) |
| `RETRIEVE_MULT` | `4` | Over-retrieve ×k (capped below) |
| `RERANK_MAX_CANDIDATES` | `40` | Candidate cap |
| `RERANK_ENABLED` | `true` | `false` keeps hybrid order, no rerank spend |
| `RERANK_MODEL` | `gemini-2.5-flash` | Listwise rerank model |
| `CHUNK_TARGET_TOKENS` | `512` | Chunk budget |
| `CHUNK_OVERLAP_TOKENS` | `64` | Word-safe overlap |
| `EXPAND_BEFORE` / `EXPAND_AFTER` | `1` / `1` | Neighbor-window size |
| `COMPACTION_ENABLED` | `true` | Summarize hot histories |
| `COMPACTION_MODEL` | `gemini-2.5-flash` | Summarizer model |
| `COMPACTION_THRESHOLD_TOKENS` | `40000` | History estimate trigger |
| `COMPACTION_KEEP_RECENT` | `10` | Verbatim recent turns |
| `COMPACTION_SUMMARY_TOKENS` | `800` | Summary cap |

Metering: query embeddings land as exact `kind=embedding` rows, rerank
generations as `kind=rerank`, compactions as `kind=compaction` — rerank and
compaction priced from the catalog (unknown models fall back to defaults); the
usage page breaks spend down per document.

API: `GET /api/documents`, `POST /api/documents` (multipart `file`), `DELETE /api/documents/{id}`; all 503 with `rag_disabled` when the stack is off. Ingestion is idempotent per document (stale vectors are deleted before re-upsert); a failed ingest keeps the row (`status=failed`) and the original object.

Retrieval quality is covered by a live golden-query eval: `BASE=http://localhost:8080 ./scripts/rag-eval.sh` uploads three fixture docs (`backend/internal/rag/testdata/eval/`, one topic each with a unique canary sentence), asks one question per doc, and passes only if every answer top-cites the expected document and carries a matching `[n]` bracket citation.
