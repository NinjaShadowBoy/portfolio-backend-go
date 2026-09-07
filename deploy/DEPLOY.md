# Deployment

This service deploys as a Docker image. `.github/workflows/deploy.yml`
builds `ghcr.io/ninjashadowboy/portfolio-backend-go:sha-<commit>` from the
`Dockerfile` at the repo root and then runs the Ansible deploy playbook from
the infra repo (the `deploy/` directory of the portfolio monorepo, published
as its own repository `vps-infra` and named by the `INFRA_REPO` Actions
variable).

Pushing to `main` deploys production; `staging` deploys the staging stack.

The VPS is shared with other apps: one nginx, one Postgres (this service
gets the databases `portfolio_production` and `portfolio_staging`), one
SigNoz. The image only ever runs as the `backend` service of the
`portfolio-<env>` compose stack the playbook renders.

What lives where:

| Concern | Where |
| --- | --- |
| Image build (Go binary + `db/` migrations, `BIND_ADDR=0.0.0.0`) | `Dockerfile` |
| Server setup, nginx, TLS, Postgres, SigNoz, compose stacks | infra repo, `roles/` |
| Runtime environment (`.env` contents) | infra repo, `inventory/group_vars/all/` + Ansible Vault |
| Rollback | re-run the deploy playbook with an older `backend_image_tag` |
| Logs, traces, metrics | SigNoz (`OTEL_EXPORTER_OTLP_ENDPOINT` set by the deploy) |
| Alerts: availability, 5xx rate, p99 latency | infra repo, `signoz-alerts.yml` |

See the infra repo's `README.md` for the full runbook, including how to run
the playbooks by hand and how to migrate an existing database into the
shared Postgres.

Local: `docker compose up` at the repo root runs the same image against a
local Postgres and a local SigNoz (see `README.md`).
