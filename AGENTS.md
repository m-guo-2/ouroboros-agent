# AGENTS.md

## Cursor Cloud specific instructions

### Overview

Ouroboros Agent is an AI Agent communication platform (monorepo, 5 sub-projects). See `README.md` for architecture and tech stack.

### Runtime Requirements

- **Bun** — primary runtime for `server`, `admin`, `channel-feishu`, `channel-qiwei`
- **Go 1.21+** — for the `agent` execution engine

### Services

| Service | Port | Start Command | Notes |
|---------|------|---------------|-------|
| server | 1997 | `cd server && bun run dev` | Central controller, SQLite via `bun:sqlite`, must start first |
| admin | 5173 | `cd admin && bun run dev` | Vite React SPA, proxies `/api` to server:1997 |
| agent | 1996 | `cd agent && go run ./cmd/agent/main.go` | Go execution engine, registers with server on startup |
| channel-feishu | 1999 | `cd channel-feishu && bun run dev` | Optional, requires Feishu credentials |
| channel-qiwei | 2000 | `cd channel-qiwei && bun run dev` | Optional, requires QiWei credentials |

Use `bun run dev` from root to start server + admin concurrently, or `bun run dev:core` for server + admin + agent.

### Key Commands

- **Install all deps**: `bun install && bun run install:all`
- **Lint**: `cd admin && bun run lint`
- **Tests**: `cd agent && go test ./... -v`
- **Build Go agent**: `cd agent && go build ./cmd/agent/...`

### Known Gotchas

- **Case-sensitivity issue**: `admin/src/main.tsx` imports `./app.tsx` but the file is `App.tsx`. This causes Vite errors on Linux (case-sensitive FS). The dev server starts but the React app won't render. This is a pre-existing codebase issue (likely developed on macOS).
- **Pre-existing lint errors**: ESLint reports 8 errors (unused imports, `set-state-in-effect` warnings). These are in the existing code.
- **TypeScript build**: `tsc -b` fails with pre-existing type errors. Vite dev server works independently since it only transpiles.
- **Agent registration**: The Go agent tries to register with server on startup. Start server first, then agent.
- **SQLite DB**: Auto-created at `server/data/config.db` on first server start. No external database needed.
