# VPS deployment & operations

Deploys are containerised. `.github/workflows/deploy.yml` builds an image, pushes
it to GHCR, copies `deploy/docker-compose.yml` to the box, writes the `.env`, and
rolls the stack with `docker compose up -d --wait`. Pushing to `main` deploys
production; `staging` deploys the staging instance.

This document covers the one-time server setup, **where the `.env` lives**, and
**how to read logs**.

## Layout on the VPS

Everything lives under `APP_PATH` (default `/opt/portfolio`), one directory per
environment:

```
/opt/portfolio/
├── production/
│   ├── docker-compose.yml   # copied by CI from deploy/
│   └── .env                 # <-- environment file (see below)
└── staging/
    ├── docker-compose.yml
    └── .env
```

`APP_PATH` must match the GitHub Actions **variable** `APP_PATH`.

Database data lives in a Docker named volume (`db-portfolio`), not in this tree — so
`rm -rf /opt/portfolio` does not delete the database, and neither does a deploy.

## The stack

Two containers per environment, defined in `deploy/docker-compose.yml`:

| Service | Image | Exposed |
| --- | --- | --- |
| `db` | `postgres:18-alpine` | nothing — only reachable from `api` over the compose network |
| `api` | `ghcr.io/<owner>/portfolio-backend-go:<sha>` | `127.0.0.1:${API_PORT}` |

The API port is published on loopback only; host nginx terminates TLS and proxies
to it. `api` waits for `db` to pass its healthcheck before starting, because the
app applies migrations on boot and must not race Postgres.

Migrations ship **inside the image** (`COPY db`) and run on startup, so schema and
code can never arrive out of step.

## Where the `.env` file goes

**`/opt/portfolio/<env>/.env`** — e.g. `/opt/portfolio/production/.env`.

It serves two consumers at once: `env_file:` hands it to the app as its runtime
environment, and Compose reads it for interpolation (`API_IMAGE`, `API_PORT`,
`POSTGRES_*`).

- **Via CI (recommended):** put the app's variables in the environment-scoped
  secret `BACKEND_ENV` (Settings → Environments → production / staging). Every
  deploy rewrites the file from it, then appends the deploy-specific values
  (`API_IMAGE` pinned to the SHA just built, `DEPLOY_ENV`).
- **By hand:** edit it directly. Do this only if you are *not* setting
  `BACKEND_ENV`, otherwise the next deploy overwrites your edits.

Use `.env.example` (repo root) as the template. On top of what the app needs, the
compose file requires:

```bash
POSTGRES_USER=portfolio
POSTGRES_PASSWORD=<generate one>
POSTGRES_DB=portfolio
API_PORT=8081          # host-side loopback port; 8082 for staging
```

`DB_URL` is **overridden** by the compose file to point at the `db` service, so
whatever value `.env` carries for local dev is ignored in the container.

## One-time setup

```bash
# On the VPS, as a sudo-capable user:
git clone <repo> && cd portfolio-backend-go/deploy
APP_PATH=/opt/portfolio ./bootstrap.sh
```

This installs Docker if missing, adds the deploy user to the `docker` group, and
creates the per-environment directories. It installs no systemd unit — compose's
`restart: unless-stopped` brings the containers back after a reboot.

Then, in the repo settings:

- **Variables:** `APP_PATH` = `/opt/portfolio`
- **Secrets:** `VPS_HOST`, `VPS_USER`, `VPS_SSH_KEY`
- **Environments** `production` and `staging`, each with a `BACKEND_ENV` secret

No GHCR token is needed on the box: each deploy lends it the job-scoped
`GITHUB_TOKEN` over stdin and logs out again when the run ends.

### nginx

Proxy the public domain to the loopback port, then `certbot --nginx`. The
container is never reachable from the internet directly.

## Collecting logs

Container stdout/stderr, capped at 3×10MB per service by the compose `logging`
block, so nothing here can fill the disk.

```bash
cd /opt/portfolio/production

# Follow live
docker compose logs -f api

# Last 200 lines
docker compose logs --tail=200 api

# Since a time
docker compose logs --since 1h api
docker compose logs --since 2026-07-16T09:00:00 --until 2026-07-16T10:00:00 api > /tmp/portfolio.log

# The database
docker compose logs -f db
```

Status at a glance:

```bash
cd /opt/portfolio/production
docker compose ps                       # includes each container's health state
curl -fsS http://127.0.0.1:8081/portfolio/v1/cron   # the same probe the deploy waits on
```

## Rollback

Images are tagged with the commit SHA, so rolling back is repointing `API_IMAGE`:

```bash
cd /opt/portfolio/production
sudo sed -i "s|^API_IMAGE=.*|API_IMAGE=ghcr.io/<owner>/portfolio-backend-go:<previous-sha>|" .env
docker compose up -d --wait
```

Find previous SHAs under **Packages** on the repo, or in the Actions history.

Note this rolls back the **code**, not the **schema** — migrations already ran and
are not reversed automatically. Additive migrations roll back cleanly; a
destructive one needs a database restore.

## Manual deploy

```bash
cd /opt/portfolio/production
echo "$GITHUB_TOKEN" | docker login ghcr.io -u <you> --password-stdin
docker compose pull && docker compose up -d --wait
```
