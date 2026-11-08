#!/usr/bin/env bash
# Start the already-deployed site; no upload or rebuild required.
set -euo pipefail
cd "$(dirname "$0")"
action=${1:-start}
if [[ $action == --help ]]; then
  echo 'Usage: ./run.sh [start|restart|status|logs]'
  echo 'Set DEPLOY_TARGET=root@SERVER to manage another server.'
  echo 'Use ./deploy.sh to publish code changes; sh scripts/dev.sh run for local development.'
  exit 0
fi
[[ $# -le 1 ]] || { echo 'Expected one action.' >&2; exit 2; }
case "$action" in start|restart|status|logs) ;; *) echo 'Usage: ./run.sh [start|restart|status|logs]' >&2; exit 2 ;; esac
configured_target=${DEPLOY_TARGET:-}
if [[ -f .deploy.env ]]; then source .deploy.env; fi
target=${configured_target:-${DEPLOY_TARGET:-}}
[[ -n $target ]] || { echo 'Set DEPLOY_TARGET in .deploy.env or your environment.' >&2; exit 2; }
[[ $target =~ ^root@[a-zA-Z0-9.-]+$ ]] || { echo 'Use root@SERVER_IP or root@hostname.' >&2; exit 2; }
ssh "$target" bash -s -- "$action" <<'REMOTE'
set -euo pipefail
if [[ ! -f /opt/charitymatch/compose.yaml ]]; then
  echo 'The site has not been deployed yet. Run ./deploy.sh from your local project.' >&2
  exit 1
fi
cd /opt/charitymatch
case "$1" in
  status) docker compose ps -a; exit ;;
  logs) docker compose logs --tail=100 app caddy; exit ;;
esac
docker compose run --rm --no-deps app /app/db migrate
if [[ $1 == restart ]]; then
  docker compose up -d --force-recreate --wait --wait-timeout 120
else
  docker compose up -d --wait --wait-timeout 120
fi
# Verify the actual server, even if local DNS still has an old cached address.
. ./.env
curl --fail --silent --show-error --retry 6 --retry-delay 3 --retry-all-errors --max-time 10 \
  --resolve "$DOMAIN:443:127.0.0.1" "https://$DOMAIN/healthz"
printf '\nSite running: https://%s\n' "$DOMAIN"
REMOTE
