# Testing

This document describes how to run every automated gate in WeShare, including
the **live** end-to-end suites that require a running API and database.

## Prerequisites

- Docker Desktop running (Postgres via `infra/compose.yaml`, host port `5433`).
- Go 1.26.5+ and Node.js 24.x / npm 11.x.
- Chromium for Playwright (install once): `npx playwright install chromium`.

## 1. Start the database

```powershell
powershell -ExecutionPolicy Bypass -File infra/scripts/db-up.ps1
```

## 2. Start the API (migrations run automatically)

```powershell
powershell -ExecutionPolicy Bypass -File infra/scripts/dev-api.ps1
```

The API listens on `http://localhost:8080`. Its health endpoints are
`GET /healthz` and `GET /readyz` (there is intentionally no `/api/v1/health`).
`/readyz` returns `{"status":"ready","checks":{"database":"ok"}}` once the
database connection and migrations are healthy.

## 3. Backend gates (`services/api`)

```powershell
cd services/api
go build ./...
go vet ./...
gofmt -l .        # must print nothing
go test ./...
```

## 4. Frontend gates (`apps/web`)

```powershell
cd apps/web
npm run lint
npm run typecheck
npm test
npm run build
```

## 5. Live browser E2E (`apps/web/e2e`)

The Playwright config starts the Next dev server itself (`npm run dev`), so only
the API and database (steps 1–2) must be running beforehand. Point Playwright at
a different API with `NEXT_PUBLIC_API_BASE_URL` if needed.

```powershell
cd apps/web
npm run e2e
```

Suites:

- `e2e/messaging.spec.ts` — two users become contacts, open a direct chat, and
  exchange messages live over the realtime WebSocket (presence + read receipt).
- `e2e/groups.spec.ts` — three users form a group, an image is shared live to
  another member, the member sees the group panel without admin tools, and a
  non-member receives `404` when reading the group's history.

Playwright writes failure artifacts to `apps/web/test-results/` (git-ignored);
open the trace with `npx playwright show-trace <path>`.

## Notes

- The API runs migrations on boot when `AUTO_MIGRATE=true` (see
  `services/api/.env.example`). You can also apply them manually with
  `infra/scripts/migrate.ps1 up`.
- Rate limiting is in-memory (per IP/user) for local development; a full E2E run
  stays well within the configured limits.
- Uploaded media is stored on the local filesystem under
  `services/api/storage/` (git-ignored).
