# Native VPS deploy (drop Docker)

## Context

The backend currently deploys as two Docker containers on the VPS (`db` +
`api`), defined in `deploy/docker-compose.yml`, built/pushed to GHCR by
`.github/workflows/deploy.yml`, bootstrapped by `deploy/bootstrap.sh`.

The VPS already runs its own native Postgres instance, separate from the
`db` container, with `portfolio_user`/`portfolio` already created — the
production `.env`'s `DB_URL` (`localhost:5432`) already points at it. The
`db` container is redundant. Rather than just dropping the `db` service, this
also drops Docker for the `api` service and runs the Go binary natively via
systemd, removing the whole container/registry layer.

The frontend (`portfolio-app`) is a separate, unrelated deploy (still
containerized, no database) and is out of scope.

## Goals

- Stop running a `db` container; use the VPS's native Postgres as-is.
- Remove Docker entirely from the backend deploy: no `Dockerfile`, no
  `docker-compose.yml`, no GHCR.
- Keep the properties the current setup has: CI builds and gates the
  artifact, migrations travel with the code that expects them, deploys are
  atomic-ish with a fast rollback path, secrets stay server-side
  (`BACKEND_ENV` GitHub Environment secret), staging and production stay
  independent.
- No privilege escalation for the deploy user (today's docker-group
  membership is root-equivalent — avoid recreating that).

## VPS layout

```
${APP_PATH:-/srv/apps/portfolio/backend}/
├── production/
│   ├── releases/
│   │   ├── <sha>/
│   │   │   ├── portfolio-backend      # binary, from `make build`
│   │   │   └── db/                    # migrations + seeds for that build
│   │   └── ...                        # last 5 kept, older pruned each deploy
│   ├── current -> releases/<sha>      # symlink, repointed each deploy
│   └── .env                           # written from BACKEND_ENV secret each deploy
└── staging/
    └── ... (same shape)
```

`APP_PATH` default changes from `/opt/portfolio` to
`/srv/apps/portfolio/backend`, matching the value already put into
`deploy/bootstrap.sh` locally (uncommitted at design time). The repo variable
`APP_PATH` must be updated to match.

Each `releases/<sha>/` is self-contained (binary + the `db/` migrations dir
that binary expects), so a rollback via symlink can never pair a binary with
the wrong migration set.

## Process model

The app runs as a **systemd user service** under the deploy user — not a
system service, not root:

- Unit: `deploy/portfolio-backend@.service`, a templated unit (`%i` =
  `production` | `staging`) checked into the repo with an `__APP_PATH__`
  placeholder. `bootstrap.sh` renders it (substitutes the real `APP_PATH`)
  into `~/.config/systemd/user/portfolio-backend@.service` — a one-time
  install step, not repeated on every deploy. If the unit template changes,
  re-running `bootstrap.sh` (or the substitution step alone) picks it up;
  `DEPLOY.md` calls this out explicitly since CI never touches it.
- `WorkingDirectory=<APP_PATH>/%i/current` — the symlink, so the app's
  existing relative `MIGRATIONS_PATH=db/migrations` resolves correctly
  without change.
- `EnvironmentFile=<APP_PATH>/%i/.env`.
- `Restart=on-failure`.
- Enabled via `loginctl enable-linger $DEPLOY_USER` (done once in
  `bootstrap.sh`) so the user session — and the service — starts on boot
  without an interactive login.

This drops the docker-group (root-equivalent) trade-off entirely: the deploy
user owns only its own directory tree and its own systemd user session, no
sudo needed for routine deploys.

### DB_URL no longer needs overriding

Today, `docker-compose.yml` rewrites `DB_URL` at the compose level to point
at the `db` service hostname. Natively, `.env`'s own `DB_URL`
(`localhost:5432`) is already correct — the app connects straight to the
VPS's Postgres exactly as configured, no deploy-time override needed. This
also makes local dev and production semantically identical for the first
time (both just "Postgres reachable at the URL in `.env`").

### Bind-address code change

`internal/server/server.go`'s `Addr: ":" + cfg.Port` binds all interfaces.
Under Docker this was safe because only the container's loopback-published
port was reachable from the host. Running natively, this would expose the
API on the VPS's public interface. Fix: change to
`Addr: "127.0.0.1:" + cfg.Port`. No behavior change in local dev (already
loopback-only in practice); on the VPS, nginx remains the only public path
by construction, no firewall rule to maintain.

## CI deploy flow (`.github/workflows/deploy.yml`)

- `test` job: unchanged (`go vet`, `go test`).
- `build` job: replace the Docker build/push step with `make build`
  (already produces a `CGO_ENABLED=0 GOOS=linux` binary — no new build logic
  needed). Upload the binary + `db/` as a build artifact instead of pushing
  to GHCR.
- `deploy` job:
  1. scp the binary + `db/` into a fresh `releases/<sha>/` on the VPS (new
     directory per deploy; nothing overwritten in place).
  2. ssh: write `.env` from the `BACKEND_ENV` secret exactly as today, plus
     `DEPLOY_ENV` — drop the `API_IMAGE`/`POSTGRES_*` interpolation lines,
     nothing left needs them.
  3. `chmod +x` the binary, repoint `current -> releases/<sha>`,
     `systemctl --user restart portfolio-backend@$DEPLOY_ENV`.
  4. Curl-retry loop against `/portfolio/v1/cron` (same probe used today)
     for up to ~30s; nonzero exit fails the job. This replaces
     `docker compose up -d --wait`.
  5. Prune `releases/` down to the last 5.
- Trigger `paths:` drops `Dockerfile` / `deploy/docker-compose.yml`, adds
  `deploy/portfolio-backend@.service`.
- No more `docker login`/`logout` to GHCR anywhere in the workflow.

## Rollback

Repoint `current` to a previous release and restart:

```bash
cd $APP_PATH/production
ln -sfn releases/<previous-sha> current
systemctl --user restart portfolio-backend@production
```

Simpler than today's `sed -i` on `API_IMAGE`. As today, this rolls back code
only — migrations already applied are not reversed automatically.

## Logs & status

- `journalctl --user -u portfolio-backend@production -f` replaces
  `docker compose logs -f api`.
- `systemctl --user status portfolio-backend@production` replaces
  `docker compose ps`.
- `curl -fsS http://127.0.0.1:8081/portfolio/v1/cron` unchanged.
- The Postgres logs/status section in `DEPLOY.md` goes away — it's the
  VPS's own Postgres now, operated however it already is, outside this
  repo's concern.

## File-by-file scope

**Removed:** `Dockerfile`, `.dockerignore`, `deploy/docker-compose.yml`.

**Rewritten:**
- `deploy/bootstrap.sh` — no Docker install / docker-group step; instead:
  create the per-env directory tree (`releases/`, `.env` placeholder, mode
  700), install the templated user unit, `loginctl enable-linger`,
  `systemctl --user daemon-reload`.
- `deploy/DEPLOY.md` — layout, systemd instead of compose, new
  rollback/logs commands, updated one-time setup steps, `.env` handling
  stays conceptually the same (`BACKEND_ENV` secret is still the source of
  truth).
- `.github/workflows/deploy.yml` — as described above.
- `.github/workflows/ci.yml` — remove the `image` job (builds the
  `Dockerfile`, no push, as a PR-time sanity check). With no `Dockerfile`
  there is nothing for it to build; the `test` job's existing `make build`
  step already catches a broken binary build.
- `README.md` — drop the `## Docker` section (build/run instructions for an
  image that no longer exists).
- `Makefile` — drop the `docker` target.

**New:** `deploy/portfolio-backend@.service` (templated systemd user unit),
versioned with the code and copied up like the compose file was.

**Changed:** `internal/server/server.go` — bind address, one line.

**Unchanged:** `Makefile` (`build` target reused as-is), `.env.example`,
all other app code, Postgres itself, nginx config, `portfolio-app` (frontend)
deploy.

## Out of scope

- Setting up Postgres itself on the VPS (already done).
- Any change to the frontend deploy.
- TLS/nginx config changes.
