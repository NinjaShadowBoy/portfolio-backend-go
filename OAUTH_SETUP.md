# OAuth2 Login Setup (Google & GitHub)

The Google and GitHub OAuth2 login/register flow is fully implemented in the backend
(`portfolio-backend-go`) and frontend (`portfolio-app`). To turn it on you only need to
register OAuth apps with each provider and drop the credentials into `.env`.

> **API versioning:** the HTTP API is mounted under a versioned base path, `/portfolio/v1`
> (`APIPrefix` + `APIVersion` in `internal/router/router.go`). Every example URL below
> includes `/v1`. To introduce a breaking change later, add a `v2` group without touching `v1`.

---

## The two OAuth URLs (read this first)

These two config values are easy to confuse. They are **different hops** in the flow:

| Var | Points to | Used for | Who redirects the browser here |
|-----|-----------|----------|--------------------------------|
| `OAUTH_BASE_URL` | your **backend** (`:8080`, incl. `/portfolio/v1`) | building the provider callback URL you register in Google/GitHub | Google / GitHub, after consent |
| `OAUTH2_REDIRECT_URI` | your **frontend** (`:4200`) | the page the backend forwards you to after login | your backend, once the JWT is minted |

- `OAUTH_BASE_URL` — the backend appends `/auth/{google|github}/callback` to it
  (`service.go`), so `http://localhost:8080/portfolio/v1` →
  `http://localhost:8080/portfolio/v1/auth/google/callback`. This is what you register
  with the provider, character-for-character.
- `OAUTH2_REDIRECT_URI` — a real Angular route (`/oauth2/redirect`). The backend redirects
  here with `?token=<jwt>` (success) or `?error=<msg>` (failure). It is a **UI route, not an
  API route**, so it is *not* versioned.

```
[Browser] click "Continue with Google"
  → GET  <OAUTH_BASE_URL>/auth/google            backend (:8080) sets cookies, 307 → Google
  → Google consent screen
  → GET  <OAUTH_BASE_URL>/auth/google/callback   backend validates + exchanges code, mints JWT
  → 302  <OAUTH2_REDIRECT_URI>?token=<jwt>        frontend (:4200) /oauth2/redirect → /home
```

---

## 1. Create the OAuth apps

### Google
1. Go to https://console.cloud.google.com/apis/credentials
2. **Create Credentials → OAuth client ID → Web application**.
3. Add an **Authorized redirect URI** (must match exactly):
   - Local: `http://localhost:8080/portfolio/v1/auth/google/callback`
   - Prod:  `https://<your-domain>/portfolio/v1/auth/google/callback`
4. Copy the **Client ID** and **Client secret**.

> The redirect URI is `<OAUTH_BASE_URL>/auth/google/callback`, so it follows whatever you
> set `OAUTH_BASE_URL` to.

### GitHub
1. Go to https://github.com/settings/developers → **OAuth Apps → New OAuth App**.
2. **Authorization callback URL**:
   - Local: `http://localhost:8080/portfolio/v1/auth/github/callback`
   - Prod:  `https://<your-domain>/portfolio/v1/auth/github/callback`
3. Copy the **Client ID** and generate a **Client secret**.

---

## 2. Configure `.env`

Copy `.env.example` to `.env` and fill in at least:

```dotenv
DB_URL=postgres://postgres:<password>@localhost:5432/portfolio?sslmode=disable
JWT_SECRET=<a long random string>

OAUTH_BASE_URL=http://localhost:8080/portfolio/v1
OAUTH2_REDIRECT_URI=http://localhost:4200/oauth2/redirect

GOOGLE_CLIENT_ID=<from Google>
GOOGLE_CLIENT_SECRET=<from Google>
GITHUB_CLIENT_ID=<from GitHub>
GITHUB_CLIENT_SECRET=<from GitHub>
```

See **The two OAuth URLs** section above for exactly what `OAUTH_BASE_URL` (backend, versioned)
and `OAUTH2_REDIRECT_URI` (frontend UI route) mean.

If you deploy the frontend on a different host/port, update `OAUTH2_REDIRECT_URI` and make
sure that origin is in `CORS_ALLOWED_ORIGINS`. Keep `OAUTH_BASE_URL`'s `/portfolio/v1` suffix
in sync with the frontend's `apiBaseUrl` and the router `APIVersion`.

A provider whose client id/secret is left blank is simply disabled — its button will
redirect back with `?error=OAuth2 is not configured` instead of crashing.

---

## 3. Make sure the database exists

Migrations create the schema but **not** the database itself. Create it once:

```bash
createdb portfolio          # or: psql -c "CREATE DATABASE portfolio;"
```

(The database name must match the one in `DB_URL`.) On startup the backend runs the
goose migrations automatically.

---

## 4. Run it

```bash
# backend
cd portfolio-backend-go
go run ./cmd/api

# frontend (separate terminal)
cd portfolio-app
pnpm install
pnpm start        # serves on http://localhost:4200
```

---

## 5. Verify end-to-end

1. Open `http://localhost:4200/login`.
2. Click **Continue with Google** (or **GitHub**).
3. Approve the consent screen.
4. You should be redirected to `/oauth2/redirect?token=...`, then land on `/home` logged in.
   - Check `localStorage` has `auth.token` set.
5. Failure check: blank out a provider's credentials and click its button — you should be
   sent back to `/login` with an error toast.

---

## GitHub Device Flow (alternative sign-in)

Besides the browser redirect flow, GitHub sign-in is also available via the
[device authorization flow](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps#device-flow)
— useful for clients without a usable browser redirect. It needs **no client secret**, but
you must tick **"Enable Device Flow"** in your GitHub OAuth App settings (it reuses the same
`GITHUB_CLIENT_ID`).

Endpoints (backend):
- `POST <api>/auth/github/device` → `{ deviceCode, userCode, verificationUri, expiresIn, interval }`
- `POST <api>/auth/github/device/token` with `{ deviceCode }` → `200` `LoginResponse` when done,
  or `202 { status: "pending" | "slow_down" }` while waiting.

Frontend: open `/login/device` (linked from the login page). It shows the user code + link,
polls automatically, and logs you in on approval.

```
[Client] POST /auth/github/device            → userCode + verificationUri
[User]   opens https://github.com/login/device, enters userCode
[Client] POST /auth/github/device/token      (polls every `interval`s: 202 pending → 200 done)
[Client] stores JWT → /home
```

---

## How the flow works (reference)

```
Frontend "Continue with X" button
  → GET  <api>/auth/{google|github}            (backend sets state + PKCE cookies, 307 to provider)
  → provider consent screen
  → GET  <api>/auth/{google|github}/callback   (backend validates state, exchanges code with PKCE,
                                                 fetches profile, upserts/links user, issues JWT)
  → 302 to OAUTH2_REDIRECT_URI?token=<jwt>      (frontend /oauth2/redirect builds the session)
```

Key files:
- Routes/handlers: `internal/handler/handler.go`
- OAuth logic + provider configs: `internal/service/service.go`
- Config/env parsing: `internal/config/config.go`
- Frontend redirect handler: `portfolio-app/src/app/features/auth/oauth2-redirect/`
- Frontend device-flow page: `portfolio-app/src/app/features/auth/github-device/`
- Frontend auth service: `portfolio-app/src/app/core/services/auth.service.ts`

---

## Notes / what changed to make this work

- **Fixed a boot-blocking migration**: `db/migrations/001_init.sql` previously started with
  `CREATE DATABASE portfolio; USE portfolio;`, which is invalid Postgres and made the app fail
  to start on a fresh setup. Those lines were removed (see step 3 — create the DB manually).
- **Passwords are now hashed with bcrypt** (`internal/auth/password.go`). New registrations are
  hashed on the way in; old plaintext rows still verify (and get no worse). This does not affect
  OAuth users, who have no password.
- **Facebook** is intentionally not wired up (only Google + GitHub have code), so the
  `FACEBOOK_*` vars were dropped from `.env.example`.
- **API versioning added**: all routes moved from `/portfolio` to `/portfolio/v1`. The version
  lives in one place — `APIPrefix`/`APIVersion` in `internal/router/router.go` — and the OAuth
  cookie callback paths are derived from it, so a future `v2` only needs a new route group.
  The frontend `apiBaseUrl` (both `environment.ts` and `environment.prod.ts`) and
  `OAUTH_BASE_URL` were updated to match.
