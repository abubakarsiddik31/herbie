<div align="center">

<p>
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/logo-mark-dark.svg" />
    <img src="docs/assets/logo-mark.svg" alt="Herbie — the bubble-bot" width="150" />
  </picture>
</p>

# Herbie

**The free, self-hosted open alternative to ChatGPT Plus and Claude Pro.**  
Built for developers and power users who want multi-model freedom, visual workflow automation, 1-click MCP apps, custom API tools, and production-grade RAG without vendor lock-in.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.23%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![React Version](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black)](https://react.dev/)
[![TypeScript](https://img.shields.io/badge/TypeScript-5-3178C6?logo=typescript&logoColor=white)](https://www.typescriptlang.org/)
[![Tailwind CSS](https://img.shields.io/badge/Tailwind_CSS-v4-06B6D4?logo=tailwindcss&logoColor=white)](https://tailwindcss.com/)
[![Docker Compose](https://img.shields.io/badge/Docker_Compose-Ready-2496ED?logo=docker&logoColor=white)](deploy/docker-compose.yml)

[Quickstart](#quickstart) • [Features](#features) • [Architecture](#architecture) • [Workflows](#visual-workflow-automation) • [Production RAG](#production-rag) • [Tools & Search](#tools--built-in-web-search) • [Contributing](CONTRIBUTING.md)

</div>

---

## Why Herbie?

SaaS AI subscriptions lock you into a single provider, hide token costs, silo your documents, and limit your automation abilities. **Herbie** gives you back full control with an interface that matches the polish of commercial chat apps while running entirely on your infrastructure.

Under the hood, Herbie pairs a high-concurrency Go backend powered by [golem](https://github.com/abubakarsiddik31/golem) with an instant React 19 + TypeScript frontend.

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│                                   HERBIE                                     │
├──────────────────────────────────────────────────────────────────────────────┤
│  ⚡ Multi-Model Chat     │  📁 Scoped Projects     │  ⚡ Visual Workflows     │
│  Gemini · OpenAI · Claude│  Custom prompt + docs   │  n8n-style canvas nodes │
├──────────────────────────┼─────────────────────────┼─────────────────────────┤
│  🔌 Curated MCP Apps     │  🛠️ Custom API Tools    │  📚 Production RAG       │
│  Model Context Protocol  │  Human-in-the-loop      │  Weaviate · Re-ranking  │
├──────────────────────────┼─────────────────────────┼─────────────────────────┤
│  📊 Micro-USD Metering   │  🎙️ Multimodal & Voice  │  🔒 Privacy & Security  │
│  Sub-cent spend ledger   │  Images + Web Speech    │  In-memory JWT + Rotate │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

## Features

### 🌐 Multi-Model Freedom
- **Per-Conversation Model Selection**: Chat with **Google Gemini** (Gemini 2.5 Flash, 3.5 Flash), **OpenAI** (GPT-4o, GPT-4o-mini), **Anthropic** (Claude 3.5 Sonnet), or custom **local proxy endpoints** (Ollama, vLLM, LiteLLM).
- **Independent Context & Settings**: Customize system instructions, temperature, and model parameters on any conversation thread at any time.
- **Deep Thread History**: URL-addressable threads (`/chat/:id`), message editing with downstream history truncation, response regeneration, and single-click markdown export.

### 📁 Claude-Style Project Workspaces
- Organize conversations, documents, and reference materials into isolated project spaces.
- Set workspace-level instructions and pin relevant project files that automatically feed the agent's context window.

### ⚡ Visual Workflow Automation (Canvas Nodes)
- Node-based automation canvas powered by `@xyflow/react` on `/workflows`.
- **Triggers**: Manual execution, test payloads, or unique incoming Webhook URLs (`/api/webhooks/:slug` with optional secret verification).
- **Core Nodes**: Universal HTTP requests (GET/POST/PUT/DELETE with Basic, Bearer, or API-key auth), Golem custom tools bridge, Slack, Discord, and GitHub issue/comment actions.
- **AI Transformations**: Prompt catalog models with templating and structured JSON outputs.
- **Agent Tool Bridge**: Flip **"Expose as Agent Tool"** on any workflow so the main chat assistant can autonomously run your workflow as a native tool during conversation!

### 🔌 1-Click MCP (Model Context Protocol) Apps
- Connect external capabilities seamlessly via standard Model Context Protocol.
- Built-in curated catalog to link and unlink integrations with single-click OAuth popup authorization.
- Deduplicated tool namespaces with active status indicators directly in the chat sidebar.

### 🛠️ Custom Tools & Built-in Web Search
- **API Tool Builder**: Register any HTTP endpoint with JSON parameters, static headers, and path/query parameter binding. Includes a gallery of 18 pre-built templates (Weather, GitHub, Hacker News, Wikipedia, Crypto rates, etc.).
- **Human-in-the-Loop Safety**: Enable *"Ask before running"* to pause agent execution and prompt you with an interactive Approve/Deny card before making external requests.
- **Built-in Web Search**: First-class web search support powered by **Wigolo** (local-first, keyless on-device engine), **Tavily**, or **Brave Search** with verified markdown citations.
- **SSRF Hardening**: Strict DNS-level validation blocks loopback, private RFC-1918, and link-local destinations.

### 📚 Production RAG (Retrieval-Augmented Generation)
- **Multi-Format Extraction**: Ingest PDF, DOCX, Markdown, and TXT files (up to 20 MB).
- **Hybrid Vector + Keyword Search**: Dense vectors in Weaviate (`gemini-embedding-001`, 768 dims) combined with BM25 keyword matching.
- **Listwise LLM Re-Ranking**: Filters candidate chunks with listwise re-ranking before injecting into the prompt context.
- **Strict Bracket Citations**: Verified `[n] (Doc § Heading, p.N)` citations referencing only chunks actually retrieved, paired with interactive source cards.
- **Hot History Compaction**: Threads exceeding token thresholds automatically compress older turns without losing context.

### 📊 Exact Token & Spend Metering
- Real-time token counters on every assistant response.
- Micro-USD spend ledger calculating prompt, completion, embedding, and re-ranking costs down to **$0.00001**.
- Usage dashboard featuring 30-day burn charts, per-model consumption tables, and document storage metrics.

### 🎙️ Multimodal Vision & Client-Side Voice
- Attach up to 4 images (PNG, JPEG, WebP, GIF, ≤ 4 MB each) per prompt for vision-capable models.
- Hands-free dictation using the browser's native Web Speech API (speech processing stays completely local to your browser).

### 🔒 Enterprise Security Core
- **In-Memory JWT**: Short-lived (15 min) HS256 access tokens stored only in frontend memory to prevent XSS credential exfiltration.
- **HttpOnly Rotating Refresh**: 30-day opaque refresh cookies (`SameSite=Lax`, path `/api/auth`) with automatic single-use rotation and family reuse revocation.
- **Silent Refresh on Reload**: Seamless session restoration on browser refresh without leaking tokens to `localStorage`.

---

## Architecture

```text
┌──────────────────────────────────────────────────────────────────────────┐
│                         Browser (React 19 SPA)                           │
│     Vite · Tailwind CSS v4 · Zustand (In-Memory Auth) · @xyflow/react     │
└─────────────────────────────────┬────────────────────────────────────────┘
                                  │ REST /api/* + Server-Sent Events (SSE)
                                  ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                         Herbie Go Backend                                │
│       net/http · Golem Agent Engine · Vault Encryption · SafeHTTP SSRF    │
└──────────────┬──────────────────┬──────────────────┬─────────────────────┘
               │                  │                  │
               ▼                  ▼                  ▼
      ┌────────────────┐  ┌──────────────┐  ┌──────────────────┐
      │   PostgreSQL   │  │   Weaviate   │  │      MinIO       │
      │ Users, History │  │ Vector Store │  │ Original Uploads │
      │  Usage Ledger  │  │ Hybrid Index │  │  Object Storage  │
      └────────────────┘  └──────────────┘  └──────────────────┘
               │
               ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                    External Providers & APIs                             │
│       Gemini · OpenAI · Claude · Tavily · Brave · Custom Webhooks        │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## Quickstart

### Prerequisites
- [Docker & Docker Compose](https://docs.docker.com/get-docker/)
- [Go 1.23+](https://go.dev/dl/)
- [Node.js 20+](https://nodejs.org/) & `npm`

### 1. Clone & Configure
```bash
git clone https://github.com/abubakarsiddik31/herbie.git
cd herbie

cp .env.example .env
```

Open `.env` and add your model provider key (at minimum, a free Google Gemini key):
```env
GEMINI_API_KEY=your_gemini_api_key_here
```

### 2. Start Core Infrastructure
Start PostgreSQL (mapped to port 5433 to avoid conflicts with existing local instances):
```bash
make up
```

### 3. Run the Stack

**Terminal 1 — Backend:**
```bash
make backend
# Running at http://localhost:8080
```

**Terminal 2 — Frontend:**
```bash
make frontend
# Running at http://localhost:5173
```

Open [http://localhost:5173](http://localhost:5173) in your browser and create your account.

---

## Optional Integrations

### Social Login (Google & GitHub)
Add your OAuth client credentials in `.env`:
```env
OAUTH_GOOGLE_CLIENT_ID=your_google_client_id
OAUTH_GOOGLE_CLIENT_SECRET=your_google_client_secret

OAUTH_GITHUB_CLIENT_ID=your_github_client_id
OAUTH_GITHUB_CLIENT_SECRET=your_github_client_secret
```
Callback URLs to register in Google/GitHub developer consoles:
- Google: `http://localhost:8080/api/auth/oauth/google/callback`
- GitHub: `http://localhost:8080/api/auth/oauth/github/callback`

### Enable Production RAG
Spin up Weaviate and MinIO via the `rag` Docker profile:
```bash
docker compose -f deploy/docker-compose.yml --profile rag up -d
```
Then set in `.env`:
```env
RAG_ENABLED=true
```

### Web Search Providers
Configure your preferred search engine in `.env`:
```env
# Option A: Built-in local Wigolo container (zero API keys needed)
WEB_SEARCH_PROVIDER=wigolo
WIGOLO_URL=http://localhost:3333

# Option B: Tavily API
WEB_SEARCH_PROVIDER=tavily
TAVILY_API_KEY=tvly-your_key_here

# Option C: Brave Search API
WEB_SEARCH_PROVIDER=brave
BRAVE_API_KEY=BSA-your_key_here
```

---

## Testing & Quality Gates

Herbie enforces strict quality gates across both backend and frontend.

```bash
# Run backend tests & checks (offline with mock fixtures)
make check

# Run frontend lint, typecheck, and unit test suite
make fe-check

# Run end-to-end smoke test against a live instance
scripts/smoke.sh
```

---

## Project Structure

```text
├── backend/
│   ├── cmd/server/             # HTTP server entrypoint
│   └── internal/
│       ├── auth/               # Password hashing, JWT maker, refresh rotation
│       ├── chat/               # Golem agent orchestration & catalog
│       ├── httpapi/            # REST API handlers, middleware, SSE streams
│       ├── mcp/                # Model Context Protocol client & catalog
│       ├── rag/                # Chunker, vector embeddings, reranker
│       ├── storage/            # PostgreSQL migrations & repositories
│       ├── vault/              # AES-GCM credential encryption
│       ├── websearch/          # Wigolo, Tavily, Brave search adapters
│       └── workflow/           # Visual canvas workflow evaluation engine
├── frontend/
│   ├── src/
│   │   ├── components/         # UI primitives, layout, sidebar, CodeBlock
│   │   ├── features/           # Auth, chat, documents, MCP, workflows
│   │   ├── pages/              # Chat, Projects, Workflows, Tools, Usage
│   │   ├── stores/             # Zustand in-memory auth & session store
│   │   └── lib/                # API client, types, utils, SSE parser
├── deploy/
│   └── docker-compose.yml      # PostgreSQL, Weaviate, MinIO, Wigolo
├── Makefile                    # Standard developer tasks
└── Dockerfile                  # Multi-stage production container build
```

---

## Contributing

We welcome contributions from the open-source community! Please review our [Contributing Guide](CONTRIBUTING.md) and [Code of Conduct](CODE_OF_CONDUCT.md) before submitting pull requests.

---

## License

Herbie is open-source software licensed under the [MIT License](LICENSE).
