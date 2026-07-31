# VPS deployment & operations

Deploys ship a prebuilt binary. `.github/workflows/deploy.yml` runs `make
build`, scp's the binary and the `db/` migrations it was built against into a
fresh release directory on the VPS, then repoints the `current` symlink and
restarts the systemd `--user` service. Pushing to `main` deploys production;
`staging` deploys the staging instance.

This document covers the one-time server setup, **where the `.env` lives**,
and **how to read logs**.

## Layout on the VPS

Everything lives under `APP_PATH` (default `/srv/apps/portfolio/backend`),
one directory per environment:

```
/srv/apps/portfolio/backend/
├── production/
│   ├── releases/
│   │   ├── <sha>/
│   │   │   ├── portfolio-backend   # binary, from `make build`
│   │   │   └── db/                 # migrations + seeds for that build
│   │   └── ...                     # last 5 kept, older pruned each deploy
│   ├── current -> releases/<sha>   # symlink, repointed each deploy
│   └── .env                        # environment file (see below)
└── staging/
    ├── releases/...
    ├── current
    └── .env
```

`APP_PATH` must match the GitHub Actions **variable** `APP_PATH`.

Each `releases/<sha>/` bundles the binary with the migrations it expects, so
a rollback (repointing `current`) can never pair a binary with the wrong
migration set. Postgres itself is **not** part of this tree at all — the app
connects to the VPS's own native Postgres instance, operated independently
of this repo.

## The service

The app runs as a **systemd `--user` service** under the deploy user (the
same user whose SSH key CI deploys with) — not root, no elevated group
membership:

| Unit | Runs |
| --- | --- |
| `portfolio-backend@production` | `current/portfolio-backend` in the `production` env dir |
| `portfolio-backend@staging` | `current/portfolio-backend` in the `staging` env dir |

The unit is templated at `deploy/portfolio-backend@.service`, versioned with
the code. `bootstrap.sh` renders it (substituting the real `APP_PATH`) into
`~/.config/systemd/user/portfolio-backend@.service` — a one-time install, not
repeated by CI on every deploy. If you change the template, re-run
`bootstrap.sh` (or redo the substitution by hand) and `systemctl --user
daemon-reload`.

The app binds to `127.0.0.1:$PORT`; host nginx terminates TLS and proxies to
it. It is never reachable from the internet directly.

Migrations run inside the app on startup (same as before), reading
`db/migrations` relative to `current` — which is exactly the `db/` shipped
in that release.

## Where the `.env` file goes

**`APP_PATH/<env>/.env`** — e.g. `/srv/apps/portfolio/backend/production/.env`.

It is both the app's runtime environment (`EnvironmentFile=` in the systemd
unit) and, unlike the old compose setup, needs **no deploy-time overrides** —
`DB_URL` in `.env` already points at the VPS's local Postgres
(`localhost:5432`), so whatever is in the file is exactly what the app uses.

- **Via CI (recommended):** put the app's variables in the environment-scoped
  secret `BACKEND_ENV` (Settings → Environments → production / staging).
  Every deploy rewrites the file from it, then appends `DEPLOY_ENV`.
- **By hand:** edit it directly. Do this only if you are *not* setting
  `BACKEND_ENV`, otherwise the next deploy overwrites your edits.

Use `.env.example` (repo root) as the template.

## One-time setup

```bash
# On the VPS, AS THE DEPLOY USER (the one CI's SSH key logs in as):
git clone <repo> && cd portfolio-backend-go/deploy
APP_PATH=/srv/apps/portfolio/backend ./bootstrap.sh
```

This creates the per-environment directories and installs the systemd user
unit. The only privileged step is `loginctl enable-linger`, which lets the
deploy user's systemd session — and the app running under it — start on boot
without an interactive login.

Then, in the repo settings:

- **Variables:** `APP_PATH` = `/srv/apps/portfolio/backend`
- **Secrets:** `VPS_HOST`, `VPS_USER`, `VPS_SSH_KEY`
- **Environments** `production` and `staging`, each with a `BACKEND_ENV` secret

### nginx

Proxy the public domain to the loopback port, then `certbot --nginx`. The
service is never reachable from the internet directly.

## Collecting logs

```bash
# Follow live
journalctl --user -u portfolio-backend@production -f

# Last 200 lines
journalctl --user -u portfolio-backend@production -n 200

# Since a time
journalctl --user -u portfolio-backend@production --since "1 hour ago"
journalctl --user -u portfolio-backend@production --since 2026-07-16T09:00:00 --until 2026-07-16T10:00:00 > /tmp/portfolio.log
```

Status at a glance:

```bash
systemctl --user status portfolio-backend@production
curl -fsS http://127.0.0.1:8081/portfolio/v1/cron   # the same probe the deploy waits on
```

## Rollback

Releases are kept by SHA under `releases/`, so rolling back is repointing the
`current` symlink:

```bash
cd /srv/apps/portfolio/backend/production
ls releases                              # find the previous SHA
ln -sfn releases/<previous-sha> current
systemctl --user restart portfolio-backend@production
```

Note this rolls back the **code**, not the **schema** — migrations already
ran and are not reversed automatically. Additive migrations roll back
cleanly; a destructive one needs a database restore.

## Manual deploy

```bash
cd /srv/apps/portfolio/backend/production
sha=<commit-sha>
# copy releases/$sha/{portfolio-backend,db} up by hand if not already there
chmod +x "releases/$sha/portfolio-backend"
ln -sfn "releases/$sha" current
systemctl --user restart portfolio-backend@production
```
