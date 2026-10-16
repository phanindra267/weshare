# Phase Status

**Project:** WeShare  
**Phase:** 1 — Authentication, Accounts, and Profiles  
**Status:** Verified  
**Last updated:** 2026-10-15

## Acceptance criteria (Phase 1)

| Item | Status | Notes |
|---|---|---|
| Auth schema (users, credentials, sessions) | Completed | `000002_users`, `000003_auth`, `000004_sessions`, `000005_devices` applied (schema at v5) |
| Password hashing (Argon2id) | Completed | `alexedwards/argon2id`, defaults Memory 64 MiB / Parallelism NumCPU / 32-byte key |
| Sessions: opaque token, DB-backed | Completed | 256-bit token; only SHA-256 hex stored in `sessions.token_hash` |
| Secure cookies | Completed | HTTP-only `weshare_session`; SameSite=Lax (default); Secure by default in prod |
| CSRF (double-submit cookie) | Completed | `weshare_csrf` cookie echoed as `X-CSRF-Token`; enforced only for unsafe methods with a session |
| Rate limiting + login lockout | Completed | In-memory per-IP: login 30/15min, register 10/hr, password 10/hr; lockout after `MAX_LOGIN_ATTEMPTS` |
| Registration / login / logout / me | Completed | `POST /api/v1/auth/{register,login,logout}`, `GET /api/v1/auth/me` |
| Change password + session revocation | Completed | `POST /api/v1/auth/password` revokes other sessions; `GET`/`DELETE /api/v1/auth/sessions[/:id]` |
| Profiles (read/update/avatar) | Completed | `GET`/`PATCH /api/v1/me`, `POST /api/v1/me/avatar`, `GET /api/v1/users/:id` |
| Avatar storage + serving | Completed | Local FS (`STORAGE_DIR`), content-sniffed jpeg/png/webp, served at `/api/v1/media/avatars/:name` (auth-gated) |
| Frontend auth screens + protected routes | Completed | `/login`, `/register`, `/profile` (gated by `RequireAuth`), `AuthProvider`, `AppHeader` |
| Frontend session state | Completed | `AuthProvider` resolves `/auth/me`; header/nav reflects auth; toasts on success/error |
| Tests (backend + frontend) | Completed | All green (see below) |

## Test results (actual, not aspirational)

**Go backend:**
- `go build ./...` → exit 0
- `go vet ./...` → exit 0
- `gofmt -l .` → empty (no formatting drift)
- `go test ./...` → pass in 8 packages: `auth`, `config`, `database`, `health`, `http`, `httpx`, `media`, `middleware`
- Migrations: applied `[1 2 3 4 5]` (000001–000005); `migrate status` reflects applied

**Frontend:**
- `npm run lint` → exit 0 (no warnings/errors)
- `npm run typecheck` → exit 0
- `npm test` → 9/9 passed (vitest: `LoginForm`, `api` CSRF/envelope, plus existing UI tests)
- `npm run build` → production build succeeds; routes `/`, `/login`, `/register`, `/profile`

**Integration (live, via curl against `:8080`):**
- Register → 201 with HTTP-only session cookie + JS-readable CSRF cookie
- `GET /auth/me` → 200 (authenticated)
- Logout without `X-CSRF-Token` → 403; with token → 204 and cookies cleared
- Login with wrong password → 401 (generic message, no user enumeration)
- Duplicate register → 409; invalid register → 422 with `details.fields`
- Lockout → 5 wrong attempts then 429; correct password while locked → 429
- Change password → session A stays valid (200), session B revoked (401)
- Sessions list → `is_current: true` for the active device
- `PATCH /me` → 200 updates display name/bio/status; `GET /me` reflects changes
- Avatar upload → 200; serving it requires a session (200 with, 401 without); non-image → 422
- `GET /users/:id` → 200 public; unknown/invalid id → 404

**CORS/CSRF integration with the browser client:**
- Allowed origin `http://localhost:3000` → `Access-Control-Allow-Origin` + `Allow-Credentials: true`
- `X-CSRF-Token` is in the allowed request headers; cookie names align (`CSRF_COOKIE_NAME` = `NEXT_PUBLIC_CSRF_COOKIE_NAME` = `weshare_csrf`)

## Known gaps (Phase 1)

- `users` package has no unit tests yet (covered indirectly by live E2E curl checks)
- No E2E browser tests executed yet (Playwright configured; browser install deferred)
- `github.com/alexedwards/scs/v2` is an unused dependency (manual DB sessions chosen); remove with `go mod tidy`
- Email/SMS verification not implemented (documented as deferred; no fake delivery)

## Next action

Start **Phase 2** (realtime messaging foundations / conversations). Read `docs/NEXT_STEPS.md` for the ordered plan. Before that: `go mod tidy` to drop SCS, and keep `PHASE_STATUS.md`/`NEXT_STEPS.md` in sync.
