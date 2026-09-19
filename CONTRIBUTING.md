# Contributing to Herbie

Thank you for your interest in contributing to Herbie! We welcome issues, bug reports, documentation improvements, and pull requests from developers of all backgrounds.

---

## Code of Conduct

All contributors and participants agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md). Please read it before getting started.

---

## Getting Started

### Prerequisites
- **Go**: Version 1.23 or newer
- **Node.js**: Version 20 or newer (with `npm`)
- **Docker & Docker Compose**: For local PostgreSQL, Weaviate, MinIO, and Wigolo

### Local Setup
1. **Fork & clone the repository**:
   ```bash
   git clone https://github.com/your-username/herbie.git
   cd herbie
   ```

2. **Configure environment variables**:
   ```bash
   cp .env.example .env
   ```
   Provide at least one model provider key (e.g. `GEMINI_API_KEY`).

3. **Start local database**:
   ```bash
   make up
   ```

4. **Run the stack**:
   - Backend: `make backend` (runs on `http://localhost:8080`)
   - Frontend: `make frontend` (runs on `http://localhost:5173`)

---

## Development Guidelines

### Architecture & Style
- **Backend (Go)**:
  - Idiomatic Go using the standard library (`net/http`) where possible.
  - All database queries live in `backend/internal/storage/`.
  - Schema migrations are managed via Atlas and goose in `backend/internal/storage/migrations/`.
  - Sensitive credentials (tokens, OAuth secrets) must be encrypted using `internal/vault`.
  - Network egress requests must pass through `internal/workflow/safehttp.go` to enforce SSRF blocks.
- **Frontend (React / TypeScript)**:
  - React 19 functional components with TypeScript strict mode.
  - State management: Zustand for global stores (e.g. `useAuth`), TanStack Query for server state.
  - Tailwind CSS v4 for utility-first styling adhering to the project's design tokens.
  - Keep access tokens in-memory only; never persist access credentials to `localStorage`.

### Running Tests

Before submitting any code changes, ensure all quality gates pass:

```bash
# 1. Backend formatting, vet, build, and unit tests
make check

# 2. Frontend linter, typecheck, and Vitest suite
make fe-check

# 3. Optional: End-to-end smoke test against running backend
scripts/smoke.sh
```

---

## Submitting Pull Requests

1. Create a feature branch from `main`:
   ```bash
   git checkout -b feat/your-feature-name
   # or
   git checkout -b fix/your-bug-fix
   ```
2. Write meaningful, atomic commits using conventional commits:
   - `feat(area): description`
   - `fix(area): description`
   - `docs(area): description`
   - `chore(area): description`
3. Include tests for any new features or bug fixes.
4. Open a pull request against the `main` branch with a clear summary of changes and how they were tested.

---

## Questions or Help?

Feel free to open a [GitHub Discussion](https://github.com/abubakarsiddik31/herbie/discussions) or submit an issue with the appropriate template.
