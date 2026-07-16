# VPS deployment & operations

The GitHub Actions workflow (`.github/workflows/deploy.yml`) builds a binary,
ships it to the VPS, writes the `.env`, and restarts the service. This document
covers the one-time server setup, **where the `.env` lives**, and **how to read
logs**.

## Layout on the VPS

Everything lives under `APP_PATH` (default `/opt/portfolio`), one directory per
environment:

```
/opt/portfolio/
├── production/
│   ├── portfolio-backend      # binary (rsynced by CI)
│   ├── db/                     # migrations (rsynced by CI, applied on startup)
│   └── .env                    # <-- environment file (see below)
└── staging/
    ├── portfolio-backend
    ├── db/
    └── .env
```

`APP_PATH` must match the GitHub Actions **variable** `APP_PATH`.

## Where the `.env` file goes

**`/opt/portfolio/<env>/.env`** -  e.g. `/opt/portfolio/production/.env`.

The app reads `.env` from its working directory, which the systemd unit sets to
that folder. `PORT` in this file is the single source of truth for the bound
port -  the deploy's health check reads it back from here rather than hardcoding
it.

You have two ways to manage it:

- **Via CI (recommended):** put the full file contents in the environment-scoped
  secret `BACKEND_ENV` (Settings → Environments → production / staging). Every
  deploy rewrites `/opt/portfolio/<env>/.env` from it (mode `600`). The
  `rsync --delete` step excludes `.env`, so it is never wiped.
- **By hand:** edit `/opt/portfolio/<env>/.env` directly. Do this only if you are
  *not* setting `BACKEND_ENV`, otherwise the next deploy overwrites your edits.

Use `.env.example` (repo root) as the template. At minimum set `DB_URL`,
`JWT_SECRET`, `GOOGLE_CLIENT_ID/SECRET`, `GITHUB_CLIENT_ID/SECRET`, and
`OAUTH_BASE_URL` / `OAUTH2_REDIRECT_URI` (see `OAUTH_SETUP.md`).

## One-time setup

```bash
# On the VPS, as a sudo-capable user:
git clone <repo> && cd portfolio-backend-go/deploy
APP_PATH=/opt/portfolio ./bootstrap.sh
```

This creates the `portfolio` service user, the directories, placeholder `.env`
files, and installs + enables the `portfolio-backend@.service` systemd unit for
both environments. It does not start the service -  the first CI deploy ships the
binary, after which:

```bash
sudo systemctl start portfolio-backend@production
```

### Let the deploy user restart without a password

The workflow runs `sudo systemctl restart|status` over SSH, so the deploy SSH
user (`VPS_USER`) needs passwordless sudo for exactly those commands. Create
`/etc/sudoers.d/portfolio-deploy` (via `visudo -f`):

```
deployuser ALL=(root) NOPASSWD: /bin/systemctl restart portfolio-backend@*, \
                                 /bin/systemctl status portfolio-backend@*, \
                                 /bin/systemctl is-active portfolio-backend@*
```

(adjust `deployuser` and the `systemctl` path -  `which systemctl`, often
`/usr/bin/systemctl`).

## Collecting logs

All output goes to **journald** (tagged `portfolio-backend-<env>`), so `journalctl`
is the single place to look -  no log files to rotate.

```bash
# Follow live (production)
journalctl -u portfolio-backend@production -f

# Last 200 lines
journalctl -u portfolio-backend@production -n 200 --no-pager

# Since a time / today, errors only
journalctl -u portfolio-backend@production --since "1 hour ago"
journalctl -u portfolio-backend@production --since today -p err

# Staging
journalctl -u portfolio-backend@staging -f

# Export a window to a file to share
journalctl -u portfolio-backend@production --since "2026-07-16 09:00" \
  --until "2026-07-16 10:00" --no-pager > /tmp/portfolio.log
```

Handy shell aliases (add to `~/.bashrc`):

```bash
alias plogs='journalctl -u portfolio-backend@production -f'
alias slogs='journalctl -u portfolio-backend@staging -f'
```

Service health at a glance:

```bash
systemctl status portfolio-backend@production
# Same probe the deploy runs; $PORT is whatever PORT is set to in that env's .env.
curl -fsS "http://127.0.0.1:$(grep -oE '^PORT=[0-9]+' /opt/portfolio/production/.env | grep -oE '[0-9]+')/portfolio/v1/cron"
```

> journald keeps logs in a ring buffer capped by disk. To retain more, set
> `Storage=persistent` and a `SystemMaxUse=` in `/etc/systemd/journal.conf`.
