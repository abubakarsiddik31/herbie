# Backlog — ChatGPT-parity features, one at a time

Build in order. One branch per item, merge to `main` before starting the next.

## Working conventions (non-negotiable)

- Branch: `feat/<name>` from `main`; merge with `--no-ff`; push `main`; delete branch.
- Commits: Conventional Commits, small and reviewable (split BE/FE/docs).
- Gates before every commit: `make check` (backend) and `make fe-check` (frontend).
- Ports: `:8080` belongs to another project — never use/kill. Backend on `:8090`
  (`APP_PORT=8090` in local gitignored `.env`), frontend via
  `VITE_API_URL=http://localhost:8090 npm run dev`.
- Never commit `.env`. Never touch docker containers or other users' processes.
- Tests for every behavior change (Go tests + vitest/msw). No unnecessary comments.
- Live-verify on `:8090` before merging (register → use the feature → confirm).

## Completed features (merged to `main`)

- [x] **1. Dark mode toggle** (`af93da4`): ThemeToggle cycling light / dark / system with `next-themes`, localStorage persistence, unit tests.
- [x] **2. Share conversation link** (`1f41de3`): `POST/DELETE /api/conversations/{id}/shares`, token-hashed links, public read-only `/s/:token` route, share modal.
- [x] **3. Export conversation as Markdown** (`fa59487`): Markdown serializer preserving fences, timestamps, model and token stats; instant client-side download.
- [x] **4. Delete single message** (`d869199`): `DELETE /api/conversations/{id}/messages/{messageId}`, cascading drop of subsequent turns, confirmation dialog.
- [x] **5. Server-side conversation search** (`dd4320a`): `GET /api/conversations?q=`, debounced input, instant filtering for any number of chats.
- [x] **6. Global custom instructions** (`9e54e58`): `GET/PATCH /api/profile`, user-level defaults prepended to every run, Preferences page.
- [x] **7. Built-in web search tool** (`b32ecec`): Bundled `web_search` tool with Tavily and Brave Search providers, approval-gated by default, markdown link citations.
- [x] **8. Keyboard shortcuts + command palette** (`689f820`): `Cmd/Ctrl+K` command palette with search/navigation/theme, `?` shortcuts cheat-sheet, Enter/Escape/Slash hotkeys.
- [x] **10. Cumulative citation numbering** (`36042e2`): `search_documents` citations maintain run-wide cumulative indices `[1]`, `[2]`, `[3]...` matching source cards across multi-search runs.
- [x] **11. Artifacts & Code Preview** (`ca829ae`): Interactive code block previews for HTML (sandboxed iframe with reload), SVG (vector render), and Mermaid (flowcharts/diagrams with theme integration), with Code/Preview tabs, fullscreen modal expansion, and copy button.
- [x] **12. Cross-Chat Memory** (`3a80e64`): Persistent long-term user memories table & migrations, automatic memory injection (`[User memory]`) into every conversation run, built-in `remember` tool, and memory management UI on `/preferences`.
- [x] **13. Visual Workflows & Tool Automation**: n8n-style visual workflow builder with `@xyflow/react`, DAG execution engine in Go, universal HTTP/REST caller, external tool integration (GitHub, Slack, Discord, Golem tools), webhook triggers, condition branching, and agent tool bridge.
- [x] **14. One-Click OAuth Tool Integrations & Safety Core**: Authenticated AES-256-GCM encrypted credentials vault, unified socket-level SSRF & DNS rebinding protection across all workflow and tool calls, automatic secret redaction filter for outputs and logs, human-in-the-loop (HITL) approval gating for chat-invoked workflows, 1-click OAuth 2.0 authorization code connect/disconnect for GitHub & Slack, and fine-grained audit logging.
- [x] **15. Chat Apps & @ Mentions with 1-Click Google Calendar OAuth**: Full @ mention autocomplete popup in chat composer with keyboard navigation, 1-click Google Calendar OAuth integration with offline refresh tokens and auto background renewal, built-in Google Calendar agent tool with HITL approval gating for event creation, inline interactive AppConnectCard for unlinked integrations, and seamless return_to redirects.
- [x] **16. Sidebar Apps & MCP Integrations**: Apps & MCP integrated directly inside the main navigation list alongside Projects and Workflows; Model Context Protocol (MCP) server endpoint at POST /api/mcp and client bridge for external MCP servers with SSRF protection; comprehensive Tools & Apps settings tab with method badges, approval gating, and live toggles.
- [x] **17. App Linking via Model Context Protocol (MCP) and Presets**: Full app linking pipeline allowing any Chat App (GitHub, Slack, Google Calendar) to be linked via external MCP servers or OAuth; App MCP presets for GitHub, Slack, Google Calendar with URL templates, header builders, live testing, and tool discovery; unified LinkAppDialog accessible across Sidebar, In-Chat AppConnectCard, and Settings Tools tab; seamless chat agent tool execution for MCP-linked apps.
- [x] **18. 1-Click Curated MCP Catalog & Built-in Engine**: Zero-configuration curated MCP apps (GitHub, Slack, Google Calendar, Web Reader, Code Sandbox) with 1-click Link/Unlink; server-side built-in execution engine eliminating manual server setup; 1-click controls in Sidebar, Settings, and in-chat AppConnectCard with optional token accordion and developer custom server fallback.

## Capability audit — 2026-11-16

Goal: the open-source ChatGPT/Claude alternative for knowledge work, not coding.
Rule: fix correctness and trust in existing features before adding surface area.
Golem is on v1.1.0; its new seams are called out where they unlock an item.

### P0 — correctness, trust, production readiness (first)

- [x] CI gate: GitHub Actions runs `make check`, `make fe-check`, Docker build, and `scripts/smoke.sh`; protect `main` (`.github/workflows/ci.yml`).
- [x] Project isolation bug: `search_documents`, `list_documents`, `read_document`, and the project-files route now scope to the conversation's `project_id` (empty project stays empty); cross-project tests cover search, read, list, and the HTTP route.
- [x] Auth lifecycle: password reset, email verification, login/register rate limiting, session list + revoke (3a); account deletion and a ZIP export of JSON plus original files, with secrets redacted (3b).
- [ ] Per-user budgets: daily/monthly token and USD caps with warning thresholds and admin suspension; keep the golem per-run limit as the hard stop.
- [ ] Admin control plane: configure providers/keys/model catalog, set quotas, inspect per-user usage. Routine operations should not require editing `.env`.
- [ ] Observability: wire `golem/telemetry` to OpenTelemetry, add metrics, `/readyz` for Postgres/Weaviate/MinIO, structured error reporting, and audit every privileged action.
- [ ] RAG hardening: OCR scanned PDFs (golem Indra/Model OCR), re-index/retry failed documents, duplicate detection, upload quotas, page-level source preview, document sets scoped to projects.
- [ ] Data safety: documented Postgres/MinIO backup + restore drill, migration rollback policy, secret rotation.

### P1 — improve existing features (parity gaps users notice)

- [ ] Memory 2.0: edit/pin/delete, scopes (global/project/bot), automatic capture with review, dedupe/merge, import/export. Turn the `remember` tool into a memory lifecycle.
- [ ] Conversation UX: branch/fork from a message, regenerate with another model, prompt library, pin/archive, in-conversation search, semantic conversation search, mobile polish.
- [ ] Attachments 2.0: persist chat attachments as documents, auto-ingest into the conversation/project corpus, OCR/vision, audio/video transcription, paste/drag-drop, larger limits under quotas.
- [ ] Voice 2.0: server-side STT fallback for Firefox/Safari, TTS read-aloud, hands-free voice mode with interruption, per-message play.
- [ ] MCP 2.0: resources/prompts, elicitation, progress, auth-challenge re-auth, tool annotations in approval cards, catalog CRUD/discovery (golem v1.1 APIs).
- [ ] Bots 2.0: in-app/email/webhook notifications, run inbox, retries/backoff, failure alerts, per-bot budgets, template gallery, clearer timezone UX.
- [ ] Workflows 2.0: cron/schedule trigger, retries and error branches, loops, versioning + rollback, run replay, template gallery.
- [ ] Sharing/export 2.0: fork shared chat, link expiry/password, JSON/PDF export, import ChatGPT/Claude exports.
- [ ] Models 2.0: user BYOK, admin-managed catalog, automatic fallback/router, reasoning-effort and max-token controls, context-budget UI.
- [ ] Golem adoption: `guard` for moderation, deferred tools/tool search for large catalogs, `WithSessionRequired`, `SummarizeHistory`, prompt-context seam, OpenAI reasoning replay.

### P2 — new differentiators (after P0/P1)

- [ ] Deep Research: plan -> parallel web searches -> read sources -> verify -> cited report, runnable in the background through bots (golem `planning`).
- [ ] Artifacts/Canvas 2.0: editable documents, downloadable MD/PDF/DOCX/XLSX/PPTX, iterate on generated files; reuse the existing HTML/SVG/Mermaid preview.
- [ ] Image generation and editing (OpenAI/Gemini/local), gallery, image-aware follow-ups.
- [ ] Audio: TTS voices, podcast/meeting transcription, audio messages, read-aloud for long documents.
- [ ] Connectors: Google Drive, Notion, Gmail, Slack, Obsidian; scheduled digests/briefings; email/Slack/WhatsApp assistant channels.
- [ ] Collaboration: teams/workspaces, shared projects, roles, comments, presence.
- [ ] Public API: API keys, OpenAI-compatible chat endpoint, webhooks, MCP app marketplace.
- [ ] Reach: PWA/installable app, push notifications, i18n, accessibility audit, native mobile shells.
- [ ] Enterprise: OIDC/SAML SSO, SCIM, retention/PII policies, content moderation, audit export.

### Release sequence

1. **Trust & isolation** — CI, project scoping, auth lifecycle, quotas, observability.
2. **Knowledge & memory** — OCR, re-index, attachments, memory 2.0, conversation UX.
3. **Voice & creation** — STT/TTS/voice mode, file generation + canvas, image generation.
4. **Research & reach** — deep research, connectors, notifications/PWA, teams/SSO.

## Chat & models — execution plan (approved 2026-11-16)

Decisions: full message tree (not assistant-only versions); BYOK + admin catalog in
Phase 2; `auto` model alias with ordered fallback (`model.NewFallback`), no ML router;
FTS before semantic search; compare mode in Phase 3. One feature per branch/PR,
adversarial plan review before build, adversarial PR review before merge.

### Phase 1A — non-destructive history (feature: `feat/chat-branches`)

Adversarial plan review 2026-11-16: PROCEED WITH CHANGES — all 17 findings
incorporated below (2 blockers, 11 major, 4 minor).

- [ ] Migration HAND-WRITTEN (schema.hcl has pre-existing drift: `mcp_servers`,
      `project_id` columns missing — reconcile HCL in this PR; never run
      `atlas migrate diff` against the drifted HCL). Up (one tx): `messages.parent_id`
      self-FK CASCADE + index, `conversations.active_leaf_id` FK SET NULL,
      `pending_tool_calls.message_id` FK CASCADE; backfill linear chains via
      `lag(id) OVER (PARTITION BY conversation_id ORDER BY created_at, id)`
      (UUID tie-break accepted — matches today's display order), leaf = last row,
      parked pending rows matched to their tool-call row by call ID in `data`.
      Down: prune non-active-path rows first, then drop columns. Regenerate atlas.sum.
- [ ] Storage: `Add` returns the new row ID; `PathToLeaf` upward recursive CTE
      (UNION, cycle-safe) for run history; `DeleteBranch` in ONE tx (subtree CTE +
      active-leaf fallback to the deleted root's parent ONLY when the leaf is inside
      the subtree); NULL-leaf recovery = latest row by `(created_at,id)`, never a
      second root; `SetActiveLeaf`; fork copies the path in one tx (new IDs, settings
      minus `bot_id`, display-only usage/sources, no ledger events).
- [ ] Concurrency: in-process per-conversation run registry (`beginRun`/`runActive`);
      send/approvals/bot-execute hold it; edit/regenerate/delete/activate/fork 409
      while a run is active OR an approval is pending (delete included — blocker fix).
- [ ] Runs: prompt parent = active leaf; run rows chain parent-wise; active leaf =
      last persisted row on finish/pause/partial failure; `done.messageId` = that row.
      Resume history = path to the parked call's `message_id` (fallback: active-leaf
      path for legacy NULL rows); resume dedupe is POSITIONAL
      (`newFrom = len(processed history)`) on success AND failure, SSE and bot paths —
      no conversation-global payload skip (phantom-variant fix).
- [ ] Path-aware consumers (all of them): send history/known, edit, regenerate
      last-user scan, bot runner history/known/resume, `finishRun`/`finishResumed`
      done ID, transcript token estimate + compacted flag, shared transcript.
- [ ] API: edit = sibling user row (images copied); regenerate = sibling assistant
      chain; `POST .../messages/{messageId}/activate` → deepest leaf of the variant
      subtree (max depth, `(created_at,id)` DESC tie-break); `POST .../fork`
      (optional `messageId`, title "Fork of …", refuses active run/pending);
      delete = subtree. Bot-scoped conversations: branch mutations 409 server-side;
      runner appends to the leaf. Transcript: `activeLeafId` + branch metadata
      `{count,index,ids}`; assistant variants computed per CHAIN (roots = direct
      assistant children of the nearest ancestor user row, often hidden tool-call
      rows); switcher metadata rides the LAST visible assistant row of the chain.
- [ ] Frontend: reseed keyed on `selectedId + activeLeafId + tail message ID`
      (not just conversation switch), skipped while streaming; `onDone` invalidates
      the detail query so edited-row IDs remap before the next mutation; branch
      switcher (prev/next + i/n), fork action, "delete this branch" wording;
      mutations disabled while running; switcher hidden for bot conversations.
- [ ] Tests (review scenarios 1–14): backfill on populated DB incl. ties/paused/
      truncated (TEST_DATABASE_URL), edit×2 + regenerate×2 + switch both ways,
      multi-leaf determinism, pause with newer sibling + resume approve/deny/re-pause,
      delete-while-pending 409 / active-subtree fallback / inactive-branch no-move /
      root delete, mutations during in-flight run 409, bot 409s + crash-restart,
      resume-failure no duplicate tail, byte-identical regenerate persists both,
      fork semantics, NULL-leaf recovery, FE reseed/remap/msw 409s, Up/Down on
      populated DB, long-conversation path bounds.
- [ ] Gates: `make check` (+ storage integration with TEST_DATABASE_URL),
      `make fe-check`, live-verify on `:8090`.

### Phase 1B — organization & discovery (split per adversarial review 2026-11-16)

Adversarial review: PROCEED WITH CHANGES, split into four PRs; the review
found 6 blockers (tsvector insert limit, generated-column rewrite, title-source
backfill, bot/fork titles, SetTitle overloading, quote composition) and 9 majors.

**PR1 `feat/chat-pins-prompts` (current)**
- [x] Migration: `conversations.pinned_at`, `archived_at`, `title_source`
      (ADD with DEFAULT 'manual' so existing rows keep their titles, then
      SET DEFAULT 'heuristic'); `saved_prompts`; partial indexes. schema.hcl +
      atlas.sum. No generated columns, no message rewrite.
- [x] Title source: split `SetHeuristicTitle` (first-message heuristic) from
      `RenameTitle` (user rename → manual); bot creation and forks stamp manual;
      PATCH title uses RenameTitle and returns 404 for unknown/foreign rows.
- [x] Pin/archive: dedicated `PUT /api/conversations/{id}/pin|archive`
      ({pinned|archived:bool}), 404 on unknown/foreign, archive 409 while a run
      is active, unarchive always allowed; list params `pinned`/`archived`
      (default excludes archived; title `q` excludes archived); send/edit/
      regenerate auto-unarchive before the turn.
- [x] Prompt library: `saved_prompts` CRUD (`GET/POST /api/prompts`,
      `PATCH/DELETE /api/prompts/{id}`), ownership + limits (100/user, title 200,
      body 8000 runes), sidebar/composer picker + command-palette entry; no
      variables in v1.
- [x] Frontend: pinned section + actions, archived section/unarchive, prompt
      library dialog with insert, tests (vitest + msw). Checked PATCH/SetSettings
      404s (no more silent 204s).

**PR2 `feat/chat-feedback-quote`**
- [x] `message_feedback` (unique per user/message, assistant rows only, IDOR
      checks, omitted from shared transcripts), thumbs UI.
- [x] `reply_to_id` + server-snapshotted `quote_text` (capped), compose the
      model-facing text in ONE place for send/edit/regenerate, keep raw `content`
      for transcript/search, carry quotes through edit/regenerate and fork.

**PR3 `feat/chat-search`**
- [x] FTS with a truncated vector (no unbounded generated tsvector; state the
      index/lock cost), `simple` config, escaped prefix queries, `ts_headline`
      sentinel markers rendered as text (no HTML injection), limits + pagination,
      archived excluded; activate-the-branch-then-jump for off-path hits;
      in-conversation find; PathToLeaf selects explicit columns.

**PR4 `feat/chat-autotitle`**
- [x] Async title in `finishRun`/`finishResumed` only (approval-paused first turn
      included), `TITLE_MODEL` via registry at request time, sanitized input
      (strip file blocks, truncate, never memories/instructions), first user turn
      only, bot/fork excluded, manual rename wins, timeout/fallback to heuristic,
      `title` usage kind + label.

### Phase 2 — models 2.0 (feature: `feat/models-2`)

- [x] `top_p`, `max_output_tokens`, reasoning level on conversations/bots/user/project
      defaults; provider mapping (OpenAI `ReasoningEffort`, Anthropic thinking,
      Gemini thinking level/budget); capability-gated UI.
- [x] `auto` alias → ordered fallback chain via `model.NewFallback` (pre-first-fragment
      only); used-model stamping in usage/transcript; fallback indicator in the run trace.
- [x] Inline model switcher; regenerate-with-model (branch, not a settings change).
- [x] BYOK: per-user provider keys in the vault (set/test/delete, audited, never
      returned); server keys as fallback; `ALLOW_USER_PROVIDER_KEYS` flag.
- [x] Admin model catalog in Postgres (seeded from the hardcoded list): CRUD, rates,
      capabilities, enabled flag; `/api/models` = enabled ∧ key-available.

### Phase 3 — context, resilience, compare (features: `feat/context-2`, `feat/compare`)

- [x] Exact input-token counting via `tokens.Counter` (Gemini/Anthropic),
      estimate fallback; context breakdown (system/memory/history/prompt/attachments).
- [x] Persisted editable conversation summary; manual "compact now"; summary view.
- [ ] Idempotent sends (`clientMessageId` unique per conversation); reconnect after a
      dropped SSE (refetch + run status); continue-after-stop.
- [ ] Compare mode: one prompt → 2–4 models as sibling branches, per-answer usage,
      pick-a-winner activates that branch.
- [ ] Semantic conversation search over the embedding stack (opt-in, RAG-independent
      storage decision at build time).
