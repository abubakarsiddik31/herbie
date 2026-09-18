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

## 1. Dark mode toggle [next]

Infra is half-present (`dark:` Tailwind variants, `next-themes` dep) but there is
no toggle UI — likely system-only today.
- Add a theme toggle (sidebar or header) cycling light / dark / system.
- Persist choice (localStorage), default system. No flash on load.
- Accept: toggle switches theme instantly, survives reload, `fe-check` green.

## 2. Share conversation link

No share routes or UI exist.
- `POST /api/conversations/{id}/share` → token-based public URL; `DELETE` revokes.
- Public read-only route rendering the thread (no auth, no composer).
- Copy-link button in conversation menu. Tokens random 256-bit, stored hashed.
- Accept: link works logged-out, revoked link 404s, owner-only manage.

## 3. Export conversation

No export exists.
- Export button (conversation menu): downloads Markdown (title, messages, code
  fences preserved, timestamps). Keep PDF out of scope.
- Accept: exported file re-renders identically, includes model/tokens footer.

## 4. Delete single message

Only truncate-on-edit/regenerate exists; no per-message delete.
- `DELETE /api/conversations/{id}/messages/{messageId}` (owner-scoped) + UI
  affordance. Decide cascade: deleting a user message removes the answer that
  followed it (document the choice in the PR).
- Accept: message gone after reload, token totals stay consistent.

## 5. Server-side conversation search

Search is client-side title filter, hidden until ≥6 chats.
- `GET /api/conversations?q=` (ILIKE on title, maybe content later) + wire the
  sidebar to it with debounce. Remove the ≥6 threshold.
- Accept: results from a fresh account with 2 chats; shows server results.

## 6. Global custom instructions

Only per-conversation `systemPrompt` exists.
- `users` gets default instructions (settings page or profile menu); injected
  into every run under the per-conversation prompt.
- Accept: saved once, present in new conversations, per-chat prompt still wins
  on conflict (document precedence).

## 7. Built-in web search tool

Web access today needs user-configured HTTP tools. Reuse the tool/approval pipeline.
- Bundled `web_search` tool (pick one provider, env-keyed, disabled when unset —
  same fail-closed pattern as OAuth providers).
- Accept: question about current events triggers approval card, cited answer.

## 8. Keyboard shortcuts + command palette

Only Enter-to-send exists.
- `Cmd/Ctrl+K` palette: new chat, search chats, go to pages, toggle theme.
- Shortcuts: new chat, stop generation, regenerate, focus composer. `?` overlay.
- Accept: all actions reachable by keyboard, shortcuts listed in-overlay.

## 9. Bigger bets (each its own project)

- Artifacts/canvas: code blocks render/run with preview for HTML/SVG/Mermaid.
- Image generation tool (input-only today).
- Cross-chat memory (only per-conversation prompts today).
- Voice output (dictation input only today).
- OAuth sign-in, RAG citations, approval-gated tools: done, see git log.

## 10. Known bug: multi-call citation numbering

`search_documents` citations restart at `[1]` on every tool call within one run,
but the source-cards list accumulates across calls — later citations can point
at the wrong card. Fix is cumulative numbering across the run (touches prompt
template, `SourceCards.tsx`, citation plugin, and tests). Do as its own item.
