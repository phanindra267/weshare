# WeShare

WeShare is a local-first communication, social, and mini-app platform inspired by the major user-facing capabilities of WeChat. This repository contains a modular monolith built with **Next.js (App Router)** on the frontend and **Go** with **Gin + PostgreSQL (pgx)** on the backend.

> **Local-first only.** No paid APIs, cloud infrastructure, or real-money payment systems are required to run locally. All features are implemented against a real local database with honest error handling (no fake success states).

## Architecture

- **Frontend** (`apps/web`): Next.js 16 + React 19 + TypeScript (strict) + Tailwind CSS v4, Vitest + RTL for unit tests, Playwright for E2E.
- **Backend** (`services/api`): Go 1.26.5 + Gin + pgx/v5 + structured logging (`slog`), centralized error envelopes, embedded SQL migrations.
- **Database** (`infra/compose.yaml`): PostgreSQL 17 (Alpine) via Docker Compose (local volume).
- **Contracts** (`packages/contracts`): OpenAPI 3.1 spec + WebSocket event catalog.

## Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) (or Docker Engine + Compose v2)
- [Go](https://go.dev/dl/) 1.26.5+
- [Node.js](https://nodejs.org/) 24.x and npm 11.x

On Windows, PowerShell scripts are provided. If you have `make`, you can use the root Makefile.

## Quick start (recommended on Windows)

**1. Clone and enter the repo**
```powershell
cd C:\Users\phani\weshare
```

**2. Start PostgreSQL**
```powershell
powershell -ExecutionPolicy Bypass -File infra/scripts/db-up.ps1
```
(Postgres runs on `localhost:5433` by default; this avoids conflicts with common 5432 installs.)

**3. Apply migrations (or let the API auto-migrate on boot)**
```powershell
powershell -ExecutionPolicy Bypass -File infra/scripts/migrate.ps1 up
```
The API also runs migrations automatically on startup when `AUTO_MIGRATE=true`.

**4. Start the Go API (in a new terminal)**
```powershell
powershell -ExecutionPolicy Bypass -File infra/scripts/dev-api.ps1
```
API listens on http://localhost:8080. Health: http://localhost:8080/healthz, Ready: http://localhost:8080/readyz

**5. Start the Next.js frontend (in a new terminal)**
```powershell
powershell -ExecutionPolicy Bypass -File infra/scripts/dev-web.ps1
```
App runs on http://localhost:3000

## Make targets (if available)

```bash
make db-up          # start postgres
make migrate-up     # apply migrations
make api            # run API
make web            # run web
make test           # run all tests
make verify         # lint+typecheck+test+build (all)
```

## Environment variables

- **API**: `services/api/.env.example` → copy to `.env` for local overrides.
- **Web**: `apps/web/.env.example` → copy to `.env.local` (Next.js convention). Public vars are `NEXT_PUBLIC_API_BASE_URL` and `NEXT_PUBLIC_WS_URL`.
- **Infra**: `infra/.env.example` → copy to `.env` to control Postgres credentials/port.

Defaults are safe for local development. Never commit `.env` files.

## Testing

**Backend (Go):**
```powershell
cd services/api
go test ./...
```

**Frontend (unit/component):**
```powershell
cd apps/web
npm test
```

**E2E (Playwright):**  
Playwright browsers are not installed by default. Install Chromium once:
```powershell
cd apps/web
npx playwright install chromium
npm run e2e
```

## Quality gates

**Backend:** `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l .` (must be clean)  
**Frontend:** `npm run lint`, `npm run typecheck`, `npm test`, `npm run build`

## Database

Migrations are embedded in `services/api/migrations/*.sql` and executed via the built-in migration runner. Schema migrations are tracked in `schema_migrations`. Down migrations are supported for local reset.

- Check status: `powershell -ExecutionPolicy Bypass -File infra/scripts/migrate.ps1 status`
- Roll back last: `powershell -ExecutionPolicy Bypass -File infra/scripts/migrate.ps1 down 1`
- Reset database (DESTROYS data): `powershell -ExecutionPolicy Bypass -File infra/scripts/db-down.ps1 -Volumes && powershell -ExecutionPolicy Bypass -File infra/scripts/db-up.ps1 && powershell -ExecutionPolicy Bypass -File infra/scripts/migrate.ps1 up`

## Troubleshooting

- **Port 5432 taken:** WeShare uses 5433 by default. If you need to change, update `infra/.env` (`POSTGRES_PORT`) and `services/api/.env` (`DATABASE_URL`).
- **Port 8080 taken:** Edit `HTTP_ADDR` in `services/api/.env` (e.g. `:8081`) and update `NEXT_PUBLIC_API_BASE_URL`/`NEXT_PUBLIC_WS_URL` in `apps/web/.env.local`.
- **Docker not running:** Start Docker Desktop, then re-run `db-up.ps1`.

## Documentation

- `docs/IMPLEMENTATION_AUDIT.md` — current state inventory
- `docs/PHASE_STATUS.md` — per-phase status + test results
- `docs/NEXT_STEPS.md` — concrete next actions (Phase 2)
- `docs/ARCHITECTURE.md`, `docs/DATABASE.md`, `docs/API.md`, `docs/SECURITY.md`, `docs/TESTING.md`, `docs/DEPLOYMENT.md`, `docs/LOCAL_SETUP.md` — living documentation

## Implementation status

Phases 0 (Foundation) and 1 (Authentication, Accounts, and Profiles) are **Verified**. See `docs/PHASE_STATUS.md` for actual test results and `docs/NEXT_STEPS.md` for the Phase 2 (contacts & presence) plan.

## Legal / notes

This is a prototype for local development and learning. Do not claim E2E encryption, real-money payments, or production-grade security unless those capabilities have been genuinely implemented and verified. All limitations are documented transparently.
