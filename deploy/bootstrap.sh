#!/usr/bin/env bash
#
# One-time VPS setup for the portfolio backend. Run once on the server, AS
# THE DEPLOY USER itself - the same user whose SSH key CI deploys with.
# Safe to re-run; it is idempotent.
#
#   APP_PATH=/srv/apps/portfolio/backend ./bootstrap.sh
#
# Deploys ship a prebuilt binary: CI runs `make build` and scp's it up, along
# with the `db/` migrations it was built against. So this script only has to
# prepare what CI cannot - the directory tree and the systemd user unit that
# runs the binary. The app runs as a systemd --user service under the deploy
# user, not root and not a docker group (which was effectively root) - the
# only privileged step here is enabling linger, so the user's systemd
# session (and the app) survives reboots without an interactive login.
set -euo pipefail

APP_PATH="${APP_PATH:-/srv/apps/portfolio/backend}"
ENVS=(production staging)
UNIT_SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/portfolio-backend@.service"
UNIT_DST="$HOME/.config/systemd/user/portfolio-backend@.service"

echo "APP_PATH=$APP_PATH  USER=$USER"

# 1. Per-environment directories. CI writes a new releases/<sha>/ and .env
#    into these on every deploy, so they only need to exist with the right
#    permissions. 700: .env holds the JWT secret, the database password and
#    the OAuth credentials.
for env in "${ENVS[@]}"; do
  install -d -m 700 "$APP_PATH/$env/releases"
done

# 2. Render the unit template with the real APP_PATH and install it as a
#    user unit. No root needed to install, run, or manage it.
mkdir -p "$(dirname "$UNIT_DST")"
sed "s#__APP_PATH__#$APP_PATH#g" "$UNIT_SRC" > "$UNIT_DST"
systemctl --user daemon-reload

# 3. Let this user's systemd session - and the service running under it -
#    start on boot without an interactive login. The only step in this
#    script that needs sudo.
sudo loginctl enable-linger "$USER"

cat <<DONE

Bootstrap complete.

  Layout:     $APP_PATH/<env>/{releases/<sha>/,current,.env}   (written by CI)
  Unit:       $UNIT_DST (rendered from deploy/portfolio-backend@.service)
  Start:      handled by CI on first deploy; by hand:
              systemctl --user start portfolio-backend@production
  Logs:       journalctl --user -u portfolio-backend@production -f
  Status:     systemctl --user status portfolio-backend@production

Still to do (see DEPLOY.md):
  - GitHub Environments 'production' and 'staging' with the BACKEND_ENV secret
  - the repo variable APP_PATH, set to $APP_PATH
  - nginx reverse proxy + certbot in front of the loopback ports
DONE
