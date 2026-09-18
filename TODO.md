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

## Future roadmap / Bigger bets (each its own project)

- Voice output / Read Aloud (dictation input only today).
- Image generation tool (input-only today).
