# Anonymous ratings & comments + profile management — design

**Date:** 2026-07-16
**Repos:** `portfolio-backend-go` (Go/Gin), `portfolio-app` (Angular SSR)
**Status:** Approved design, ready for implementation plan

## Goal

Let anyone rate and comment on projects **without logging in**. Anonymous
contributions are attributed to an auto-generated identity (`anonymous-<animal>-N`,
e.g. `anonymous-goose-12`), persist across visits via a secure cookie, and are
**claimed** into a real account when the visitor registers or logs in. Add a
**profile management** page for real account holders.

## Core principle: an anonymous visitor is a real, auto-provisioned user

Rather than threading a second "identity kind" through every rating query,
ownership check, and name lookup, the first time someone rates or comments the
backend transparently creates a real `users` row with `provider='ANON'` and
`name='anonymous-<animal>-N'`. All existing foreign keys, `UNIQUE` constraints,
owner checks, and `getUserName` paths keep working unchanged. "Removing the login
requirement" becomes "auto-creating a session."

### Animal naming

On anon-user creation the backend picks a random animal from a fixed list and
sets `name = 'anonymous-' + animal + '-' + N`, where **N is that animal's own
running counter** (the 12th goose → `anonymous-goose-12`). N is computed as the
count of existing users whose name begins with `anonymous-<animal>-`, plus one.

## Data model

### `ratings` — one editable star per identity per project
- Keep `UNIQUE(user_id, project_id)` (one star per person per project).
- **Drop the `comment` column.** Before dropping, migrate every existing
  non-null `ratings.comment` into a `comments` row for the same
  `(user_id, project_id)`.

### `comments` — new table, many per identity per project
```sql
CREATE TABLE comments (
    id         BIGSERIAL PRIMARY KEY,
    body       VARCHAR(2000) NOT NULL,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_comments_project_id ON comments(project_id);
CREATE INDEX idx_comments_user_id ON comments(user_id);
```

### `users` — allow the anon provider
- Extend the `provider` CHECK to include `'ANON'`.

Delivered as a new goose migration (`002_*.sql`); `001_init.sql` is unchanged.

## Identity resolution — `EnsureIdentity` middleware

Applied to the public write endpoints. Resolves the caller in order:
1. `Authorization` JWT → the real logged-in user; else
2. `anon_token` cookie → the existing anon user (token is opaque, stored
   SHA-256-hashed server-side; the cookie holds the raw token); else
3. neither → create a fresh anon user, issue an `anon_token`, set the cookie.

**Cookie:** `HttpOnly; Secure; SameSite=None` (cross-site to the API domain),
carried by the already-enabled credentialed CORS (`AllowCredentials: true`,
explicit `AllowedOrigins`). The frontend never reads the cookie; it learns the
current identity from `GET /auth/me`.

A dedicated `anon_tokens` table stores `(token_hash, user_id, created_at)` so the
opaque cookie can be resolved to its anon user.

## Endpoints

### Public — behind `EnsureIdentity`
- `POST /ratings` — upsert my star for a project (one per identity/project)
- `PUT /ratings/:id`, `DELETE /ratings/:id` — owner-scoped
- `POST /comments` `{projectId, body}` — create a comment (many allowed)
- `PUT /comments/:id`, `DELETE /comments/:id` — owner-scoped
- `GET /projects/:id/comments` — public list for the thread
- `GET /auth/me` — current identity (real / anon / none)

Existing public read endpoints (`GET .../ratings`, `.../average`, `.../count`,
`.../distribution`) are unchanged.

### Profile — behind real-auth (`Auth`, rejecting `provider='ANON'`)
- `PATCH /users/me` — update `name` + avatar (`image_url`; avatar upload reuses
  the existing `SaveProfilePhoto` endpoint)
- `PATCH /users/me/password` — LOCAL accounts only, bcrypt
- `GET /users/me/ratings`, `GET /users/me/comments` — my activity
- `DELETE /users/me` — cascades my ratings and comments

## Claim on login / register / OAuth

No new route. The existing `login`, `register`, and OAuth-callback handlers, when
the request carries an `anon_token` cookie, re-associate the anon user's rows to
the authenticated real account, retire the anon user row, and clear the cookie:
- **Comments:** always transfer.
- **Star rating (one per project):** if the account already rated that project,
  the account's existing star wins and the anon star is dropped; otherwise the
  anon star transfers.

## Password hashing

Enabling "change password" requires fixing the current plaintext storage in
`internal/auth/password.go`: switch to bcrypt for register, login, and
change-password, and re-hash the seeded admin in `seeder.go`.

## Frontend (`portfolio-app`)

- **`IdentityService`**: on app init calls `GET /auth/me` (with credentials) and
  exposes an identity signal. An HTTP interceptor sets `withCredentials` on API
  calls so the anon cookie flows.
- **`project-detail`**: remove the `@if (isAuthenticated())` gate — the star form
  and a new **comments thread** (list + add + edit/delete own) are always shown,
  labeled with the current identity (`anonymous-goose-12` or the account name).
- **`/profile`** route (guarded: real account, not anon): edit name & avatar,
  change password, list my ratings & comments with edit/delete, delete account.
- After login/register/OAuth the identity signal refreshes (claim already
  happened server-side), so freshly-earned anon activity appears under the
  real account.

## Build order (small, shippable slices)

1. **Schema**: migration for `comments` (with data migration from
   `ratings.comment`), drop `ratings.comment`, `ANON` provider, `anon_tokens`
   table, animal-name helper + sqlc queries.
2. **Anon identity**: `EnsureIdentity` middleware, `anon_token` cookie,
   `GET /auth/me`.
3. **Public ratings/comments**: move rating writes off hard-auth; add comments
   CRUD + list.
4. **Claim-on-auth** + bcrypt password hashing.
5. **Frontend**: identity wiring, un-gated star + comments UI on project-detail.
6. **Profile page** + its endpoints.

## Out of scope
- Rate-limiting / anti-abuse on anonymous submissions (a visitor who clears
  cookies gets a new identity). Acceptable for a portfolio; can be added later.
- The separate refresh-token + VPS-deploy design (its own spec); this feature's
  anon cookie is independent but shares the same credentialed-CORS/cookie infra.
