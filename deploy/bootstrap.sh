#!/usr/bin/env bash
#
# One-time VPS setup for the portfolio backend. Run once on the server as a
# sudo-capable user. Safe to re-run; it is idempotent.
#
#   APP_PATH=/opt/portfolio ./bootstrap.sh
#
# Deploys are containerised: CI builds an image, pushes it to GHCR, and the box
# pulls it and rolls the compose stack. So this script only has to prepare what
# CI cannot - Docker itself, the directory tree, and the deploy user's access.
# It does NOT install a systemd unit; compose's `restart: unless-stopped` is what
# brings the containers back after a reboot.
set -euo pipefail

APP_PATH="${APP_PATH:-/opt/portfolio}"
DEPLOY_USER="${DEPLOY_USER:-$USER}"
ENVS=(production staging)

echo "APP_PATH=$APP_PATH  DEPLOY_USER=$DEPLOY_USER"

# 1. Docker + the compose plugin. `docker compose` (v2 subcommand) is required -
#    the deploy uses `up --wait`, which docker-compose v1 does not have.
if ! command -v docker >/dev/null 2>&1; then
  echo "Installing Docker"
  curl -fsSL https://get.docker.com | sudo sh
fi
docker compose version >/dev/null 2>&1 \
  || { echo "docker compose v2 missing - install the compose plugin" >&2; exit 1; }

# 2. The deploy user drives docker over SSH without sudo. Note that docker group
#    membership is effectively root on this box - use a dedicated deploy user and
#    a dedicated SSH key, so CI's access can be revoked without touching yours.
sudo usermod -aG docker "$DEPLOY_USER"

# 3. Per-environment directories. CI writes docker-compose.yml and .env into
#    these on every deploy, so they only need to exist with the right owner.
#    700: .env holds the JWT secret, the database password and the OAuth
#    credentials.
for env in "${ENVS[@]}"; do
  sudo install -d -o "$DEPLOY_USER" -g "$DEPLOY_USER" -m 700 "$APP_PATH/$env"
done

cat <<DONE

Bootstrap complete.

  Layout:     $APP_PATH/<env>/{docker-compose.yml,.env}   (both written by CI)
  Volumes:    the Postgres data volume is managed by Docker, not this tree
  Start:      handled by CI; by hand: cd $APP_PATH/production && docker compose up -d
  Logs:       cd $APP_PATH/production && docker compose logs -f api
  Status:     cd $APP_PATH/production && docker compose ps

Still to do (see DEPLOY.md):
  - GitHub Environments 'production' and 'staging' with the BACKEND_ENV secret
  - the repo variable APP_PATH, set to $APP_PATH
  - nginx reverse proxy + certbot in front of the loopback ports
DONE
