# Deployment

Production runs on the VPS `51.210.177.80` as the `backend` service of the
compose project in `/opt/apps/portfolio/production/compose.yaml`. The
frontend (`portfolio-app` repo) is the other service in the same file.

Pushing to `main` (when Go code, migrations, the `Dockerfile` or the deploy
files change), or running the **Deploy Backend** workflow by hand, runs
`.github/workflows/deploy.yml`:

1. `go vet ./...` and `go test ./...`.
2. `docker build` on the runner, tagged
   `ghcr.io/ninjashadowboy/portfolio-backend-go:ci-<sha12>-<run>-<attempt>`.
3. `docker save | gzip | ssh deploy@vps 'gunzip | docker load'`. The image
   never goes through a registry, so the VPS needs no GHCR credentials.
4. `.github/deploy/remote-deploy.sh deploy backend <image>` on the VPS:
   - `pg_dumpall` of `infra-postgres` into `~/backups/pre-backend-<UTC time>.sql.gz`; the newest 10 `pre-*.sql.gz` are kept.
   - copy `compose.yaml` to `compose.yaml.bak`, set the backend `image:` line.
   - `docker compose up -d --wait backend`; if the container is not healthy within 180 s, restore `compose.yaml.bak` and bring the old image back up.
   - remove older images of this repository, keeping the newest 3 and the one just replaced.
5. From the runner: `https://alexabena.me/portfolio/v1/cron`,
   `https://alexabena.me`, `https://www.alexabena.me` and
   `https://alexabena.space` must answer 2xx within about a minute. If not,
   `remote-deploy.sh rollback backend <previous image>` puts the old image back.

Two deploys never overlap. Within this repo the `deploy-production`
concurrency group queues them. Across the two repos, `remote-deploy.sh`
takes a `flock` on `/opt/apps/portfolio/production/.deploy.lock`.
`portfolio-app` keeps an identical copy of the script, so change both.

Migrations ship in the image and run when the backend starts. The rollback
does not undo them, so a migration must stay compatible with the previous
binary or be restored from the backup by hand.

## GitHub setup (once per repo)

Secrets (Settings > Secrets and variables > Actions), the same values in
`portfolio-backend-go` and `portfolio-app`:

| Secret | Value |
| --- | --- |
| `VPS_SSH_KEY` | private half of a dedicated deploy key (below) |
| `VPS_KNOWN_HOSTS` | `[51.210.177.80]:6562 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIBfLX6tdoO+Q1LoT3ZySZmUe8NtdDVSul83QPVJX5I0x` |

Host, port and user are plain `env` values at the top of the workflow.

Create the deploy key on your machine, without a passphrase:

```bash
ssh-keygen -t ed25519 -N "" -C "github-actions-portfolio-deploy" -f portfolio-deploy
```

Paste the content of `portfolio-deploy` into `VPS_SSH_KEY`. Append the
public half to `/home/deploy/.ssh/authorized_keys` on the VPS, prefixed with
`restrict` (no tty, no port or agent forwarding; commands still run):

```
restrict ssh-ed25519 AAAA...contents of portfolio-deploy.pub... github-actions-portfolio-deploy
```

Then delete the local key files. To check the host key yourself:
`ssh-keyscan -p 6562 -t ed25519 51.210.177.80`.

The workflow uses the GitHub Environment `production`. GitHub creates it on
the first run; add required reviewers there if a deploy should wait for
approval.

## Rollback by hand

Re-run an older successful **Deploy Backend** run (it builds that commit
again), or on the VPS:

```bash
cd /opt/apps/portfolio/production
docker images ghcr.io/ninjashadowboy/portfolio-backend-go   # newest 3 are kept
cp compose.yaml.bak compose.yaml                            # or edit the image: line
docker compose up -d --wait backend
```

Restore a backup: `gunzip -c ~/backups/<file>.sql.gz | docker exec -i infra-postgres psql -U postgres`.

The Ansible project in the portfolio monorepo's `deploy/` directory
provisioned the box (nginx, TLS, Postgres, `backend.env`) but no longer
deploys. If you run its `deploy.yml`, pass the current image tags, or it
reverts them to its defaults.
