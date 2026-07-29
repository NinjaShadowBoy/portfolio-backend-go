# Refresh tokens + VPS frontend deploy -  design

Date: 2026-07-16
Repos: `portfolio-backend-go` (Go/Gin), `portfolio-app` (Angular SSR)

## Goal

Two independent pieces of work, bundled because they touch the same deploy surface:

1. **Short-lived access tokens + long-lived refresh tokens.** Access tokens
   become 15-minute JWTs; a 30-day opaque refresh token, stored hashed in
   Postgres and delivered in an httpOnly cookie, lets the frontend transparently
   mint new access tokens. Logout revokes the refresh token server-side.
2. **Deploy the Angular SSR frontend on the same VPS**, behind nginx, alongside
   the Go backend.

## Confirmed decisions

- **Topology:** frontend at `alexabena.me` / `alexabena.space`, backend at a
  separate **`api.` subdomain** (e.g. `api.alexabena.me`). This is **cross-site**,
  so the refresh cookie is `SameSite=None; Secure` and CORS stays credentialed.
- **Rotation + multi-device with reuse detection.** Each login starts a token
  *family*; refresh rotates within the family; a replayed (already-revoked) token
  revokes the whole family (that one device), not the user's other sessions.
- **Lifetimes:** access 15 min, refresh 30 days.
- **Access token storage (frontend):** in-memory signal only (not localStorage);
  a silent `/auth/refresh` on browser bootstrap restores the session from the
  cookie.
- **Frontend runtime:** full Angular SSR (`@angular/ssr` + Express, `outputMode:
  server`, locale sub-paths `/fr/...`). Runs as a Node process; nginx reverse-
  proxies to it. It is **not** static files.

---

## Part 1 -  Refresh tokens (backend)

### 1.1 Token model

- **Access token:** unchanged shape -  stateless HS256 JWT, Bearer-authenticated,
  `Type: "ACCESS"`. Default lifetime drops from 24h to **15 min**.
- **Refresh token:** opaque 256-bit random string (base64url). Only its
  **SHA-256 hash** is stored. Chosen over a JWT refresh token because revocation
  and reuse-detection require server-side state; a stateless JWT can't be revoked
  without a blocklist that negates the benefit.

### 1.2 Database -  new migration `db/migrations/002_refresh_tokens.sql`

Leaves `001_init.sql` untouched (already applied on the VPS).

```sql
-- +goose Up
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    family_id  UUID NOT NULL,                 -- device/session lineage
    token_hash CHAR(64) NOT NULL UNIQUE,      -- sha256 hex of the opaque token
    expires_at TIMESTAMP NOT NULL,
    revoked_at TIMESTAMP,                      -- NULL = active
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id   ON refresh_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_family_id ON refresh_tokens(family_id);

-- +goose Down
DROP TABLE IF EXISTS refresh_tokens;
```

`family_id` uses `UUID` (Postgres `gen_random_uuid()` available via pgcrypto in
PG13+, or generated app-side -  app-side chosen to avoid an extension dependency).

### 1.3 sqlc queries -  `db/queries/refresh_tokens.sql`

- `CreateRefreshToken` (user_id, family_id, token_hash, expires_at) → row
- `GetRefreshTokenByHash` (token_hash) → row (includes revoked_at, expires_at)
- `RevokeRefreshToken` (id) -  set revoked_at = now
- `RevokeRefreshTokenFamily` (family_id) -  set revoked_at = now WHERE revoked_at IS NULL
- `DeleteExpiredRefreshTokens` () -  housekeeping, called opportunistically

Run `sqlc -f db/sqlc.yaml generate` after adding.

### 1.4 Service layer

- `auth` package gains refresh-token helpers: generate opaque token +
  `HashRefreshToken` (sha256 hex). A `RefreshTTL` duration on the `JWT` struct (or
  a small new config value) drives `expires_at`.
- **`IssueSession(ctx, user) (dto.LoginResponse, refreshPlain string, err)`** —
  new service helper. Mints the access JWT via existing `GenerateLoginResponse`,
  generates an opaque refresh token, inserts its hash with a fresh `family_id`,
  returns both. Called by `Login`, `OAuthLogin`, `PollGitHubDeviceLogin`.
- **`RefreshSession(ctx, refreshPlain) (dto.LoginResponse, newRefreshPlain, err)`:**
  1. hash → `GetRefreshTokenByHash`. Not found → `ErrUnauthorized`.
  2. `revoked_at` set → **reuse detected** → `RevokeRefreshTokenFamily` →
     `ErrUnauthorized`.
  3. `expires_at` past → revoke + `ErrUnauthorized`.
  4. otherwise **rotate**: revoke this row, insert a new token in the *same*
     `family_id`, load the user, return a fresh `LoginResponse` + new plaintext.
- **`Logout(ctx, refreshPlain) error`:** look up by hash; revoke its whole
  family; idempotent (unknown/expired token → no error). Clears the cookie
  regardless.
- Register is unchanged (still returns a message, no session).

### 1.5 HTTP layer

New routes under the existing auth group:

- `POST /auth/refresh` -  no body; reads the refresh cookie; on success sets the
  new refresh cookie and returns `LoginResponse` (access token + user). On
  failure clears the cookie and returns 401.
- `POST /auth/logout` -  reads the refresh cookie, revokes the family, clears the
  cookie, returns 200 (200 even if no cookie -  logout is idempotent).

`Login`, the OAuth callback, and the device-flow completion all set the refresh
cookie in addition to their current responses. The OAuth callback sets it during
its 307 redirect to the frontend.

**Refresh cookie attributes:**
- Name `refresh_token`, value = opaque plaintext.
- `HttpOnly; Secure; SameSite=None`.
- `Path` = `<basePath>/auth` (e.g. `/portfolio/v1/auth`) so it's sent to
  refresh/logout/login only.
- `Max-Age` = refresh TTL (30 days); cleared with `Max-Age=-1` on logout/failure.
- `Secure` is controlled by a new `AUTH_COOKIE_SECURE` env (default `true`) so
  plain-http local testing can turn it off; `SameSite=None` still functions on
  `http://localhost` because browsers treat localhost as a secure context.

### 1.6 Config / env

New in `internal/config/config.go` + `.env.example`:
- `JWT_EXPIRATION` default changes `86400000` → `900000` (15 min).
- `REFRESH_TOKEN_EXPIRATION` (ms), default `2592000000` (30 days).
- `AUTH_COOKIE_SECURE` (bool), default `true`.

### 1.7 Tests

`internal/service` (and/or handler) tests, table-driven where practical:
- rotation: old token revoked, new token works, same family.
- reuse detection: replay of a revoked token revokes the family; a sibling token
  in that family stops working.
- logout: family revoked; subsequent refresh with that token → 401.
- expiry: expired refresh token → 401.
Backend gate: `go vet ./...`, `go test ./...`, `make build`.

---

## Part 2 -  Refresh tokens (frontend `portfolio-app`)

### 2.1 Auth service

- **Access token in memory only.** Remove the `localStorage` persistence for
  `auth.token` (keep `auth.user` optional, or also drop -  see 2.4). The token
  signal is the single source.
- `login`, `register`, device calls, and the new `refresh`/`logout` calls use
  `{ withCredentials: true }` so the cookie is stored and sent cross-site.
- **`refresh()`** → `POST /auth/refresh` (withCredentials, no body) → `LoginResponse`
  → `setSession`. Returns an observable other code can await.
- **`logout()`** → `POST /auth/logout` (withCredentials) → then clear local
  signals + show the existing logout notification. Server revocation completes
  the previously client-only logout.
- **Bootstrap:** on app init in the browser, if there is no in-memory token, call
  `refresh()` once silently to restore a session from the cookie (an
  `APP_INITIALIZER` / provider factory, browser-only via `isPlatformBrowser`).

### 2.2 Interceptor -  seamless refresh

`auth.interceptor.ts` becomes stateful (inject `AuthService`):
- attach `Authorization: Bearer <token()>` when present (read from the service
  signal, not localStorage).
- on a `401` from an API request (excluding `/auth/refresh` and `/auth/login`
  and `/auth/logout`): call a **shared, in-flight `refresh()`** (so N concurrent
  401s trigger exactly one refresh), then retry the original request once with
  the new token. If refresh fails, propagate the 401 and clear the session.

### 2.3 OAuth redirect page

Unchanged logic (`?token=` → `setSessionFromToken`); the refresh cookie is
already set by the callback. Ensure the redirect component / any first API call
uses `withCredentials` where it needs the cookie.

### 2.4 Notes

- `setSessionFromToken` / `expiresIn` handling stays; with a 15-min access token
  the interceptor's 401-retry is what keeps sessions seamless.
- SSR renders unauthenticated (cookie is httpOnly + browser-only bootstrap),
  matching today's behavior.
- Gate: `npx tsc --noEmit`, `ng build`.

---

## Part 3 -  Frontend deploy on the VPS (nginx + SSR)

Mirrors the backend's existing systemd + rsync + GitHub Actions pattern.

### 3.1 Runtime

- Node LTS installed on the VPS (NodeSource).
- Angular server build (`ng build --localize`) emits
  `dist/portfolio-app/{browser,server}`; `server/server.mjs` is the Express SSR
  entry, listening on `PORT` (default 4000). Verify at build time whether
  `server.mjs` is self-contained or needs `node_modules` at runtime; if needed,
  ship `package.json` + `package-lock.json` and run `npm ci --omit=dev` on the
  VPS. (Default Angular esbuild server bundle is self-contained; confirm with a
  smoke run during implementation.)

### 3.2 systemd -  `portfolio-frontend@.service` (in `portfolio-app/deploy/`)

Templated like the backend unit: `%i` = environment, `__APP_PATH__` substituted
by a bootstrap script. `WorkingDirectory=__APP_PATH__/%i`,
`ExecStart=/usr/bin/node __APP_PATH__/%i/server/server.mjs`,
`Environment=PORT=<per-env>` (or an `.env`/`EnvironmentFile`), journald logging
with `SyslogIdentifier=portfolio-frontend-%i`, same light hardening. Runs as a
non-privileged `portfolio` user.

Default frontend app path: `/opt/portfolio-frontend/<env>` (production/staging),
ports e.g. production 4000, staging 4001.

### 3.3 nginx

Two server blocks (kept in the repos under `deploy/nginx/` and documented):
- **`api.alexabena.me`** → `proxy_pass http://127.0.0.1:8081;` (Go backend).
  Forward `Host` and `X-Forwarded-Proto`; do **not** strip cookies. TLS via
  certbot.
- **`alexabena.me` / `alexabena.space`** → `proxy_pass http://127.0.0.1:4000;`
  (SSR). `proxy_http_version 1.1`, forward standard proxy headers, TLS via
  certbot. Static assets are served by the SSR Express layer (`express.static`
  with 1y cache) -  nginx can additionally cache/serve `/browser` if desired, but
  proxying everything to SSR is the simple correct default.

Backend sets `Secure` on the cookie unconditionally; nginx terminates TLS so the
browser always sees HTTPS. CORS `AllowOrigins` already includes the frontend
origins; keep `AllowCredentials: true`.

### 3.4 Frontend CI -  `.github/workflows/deploy.yml` (in `portfolio-app` repo)

New workflow, structured like the backend's: build job (`npm ci`,
`ng build --localize`, upload `dist/` artifact) + deploy job (rsync `dist/` to
`/opt/portfolio-frontend/<env>/`, `--exclude` any server-side env file, restart
`portfolio-frontend@<env>`, health-check the SSR port). Per-environment GitHub
Environments hold secrets, reusing the VPS host/user/key pattern.

### 3.5 Docs / bootstrap

- `portfolio-app/deploy/bootstrap.sh` -  one-time VPS setup for the frontend
  (Node install check, dirs, placeholder env, install+enable the unit).
- Extend `portfolio-backend-go/deploy/DEPLOY.md` (or a new frontend DEPLOY.md) to
  cover: nginx config install, certbot, the frontend systemd unit, journald log
  commands (`journalctl -u portfolio-frontend@production -f`), and the DNS records
  (`alexabena.me`, `api.alexabena.me`).

---

## Out of scope

- Migrating the existing GitHub Pages deploy away (the `build:gh-pages` script and
  `deploy` npm script stay; VPS deploy is additive).
- Rate limiting on `/auth/*` (nice-to-have, not required here).
- CSRF tokens for the refresh endpoint -  refresh relies on the cookie + a
  same-origin-less design; because the access token is Bearer (not cookie) and
  refresh returns tokens in the body (not acting on cookie-authenticated state
  mutations), classic CSRF on refresh yields nothing usable to an attacker. Noted
  explicitly rather than adding CSRF machinery.

## Verification (end-to-end)

1. Backend: migrate, `go test ./...`, boot. Login → receive access token + refresh
   cookie. Wait past 15 min (or force-expire) → an API call 401s → interceptor
   refreshes → call succeeds, no user-visible interruption.
2. Logout → refresh cookie cleared, `/auth/refresh` now 401s.
3. Reuse: capture a refresh token, rotate it once, replay the old one → family
   revoked, current session's next refresh also 401s.
4. Frontend on VPS: `alexabena.me` renders via SSR (including `/fr/...`),
   `api.alexabena.me/portfolio/v1/cron` responds, login round-trips the cookie
   cross-site.
