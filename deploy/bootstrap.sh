#!/usr/bin/env bash
#
# One-time VPS setup for the portfolio backend. Run once on the server as a
# sudo-capable user, from the repo's deploy/ directory (or anywhere -  it finds
# its own dir). Safe to re-run; it is idempotent.
#
#   APP_PATH=/opt/portfolio ./bootstrap.sh
#
# It creates the service user, the per-environment directories, placeholder .env
# files, and installs + enables the systemd unit. It does NOT start the service
# (there is no binary yet -  the first CI deploy ships it, then start it).
set -euo pipefail

APP_PATH="${APP_PATH:-/opt/portfolio}"
APP_USER="${APP_USER:-portfolio}"
ENVS=(production staging)
SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "APP_PATH=$APP_PATH  APP_USER=$APP_USER"

# 1. Service user -  system account, no login shell.
if ! id "$APP_USER" >/dev/null 2>&1; then
  echo "Creating service user $APP_USER"
  sudo useradd --system --home-dir "$APP_PATH" --shell /usr/sbin/nologin "$APP_USER"
fi

# 2. Per-environment directories + a locked-down placeholder .env.
for env in "${ENVS[@]}"; do
  sudo install -d -o "$APP_USER" -g "$APP_USER" -m 750 "$APP_PATH/$env" "$APP_PATH/$env/db"
  if [ ! -f "$APP_PATH/$env/.env" ]; then
    sudo install -o "$APP_USER" -g "$APP_USER" -m 600 /dev/null "$APP_PATH/$env/.env"
    echo "Created empty $APP_PATH/$env/.env"
  fi
done

# 3. Install the templated systemd unit with APP_PATH substituted in.
echo "Installing systemd unit"
sudo sed "s#__APP_PATH__#$APP_PATH#g" \
  "$SRC_DIR/portfolio-backend@.service" \
  | sudo tee /etc/systemd/system/portfolio-backend@.service >/dev/null
sudo systemctl daemon-reload
for env in "${ENVS[@]}"; do
  sudo systemctl enable "portfolio-backend@$env"
done

cat <<DONE

Bootstrap complete.

  .env files:   $APP_PATH/<env>/.env   (production / staging)
                CI overwrites these from the BACKEND_ENV environment secret on
                every deploy. Edit by hand only if you are not using CI for env.

  Start:        sudo systemctl start portfolio-backend@production
  Logs:         journalctl -u portfolio-backend@production -f
  Status:       systemctl status portfolio-backend@production

Make sure APP_PATH ($APP_PATH) matches the GitHub Actions variable APP_PATH,
and that the deploy SSH user may run systemctl for these units without a
password (see deploy/DEPLOY.md).
DONE
