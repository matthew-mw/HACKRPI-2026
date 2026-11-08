#!/usr/bin/env bash
# Build and deploy the live site using .env.production.
set -euo pipefail
cd "$(dirname "$0")"
if [[ ${1:-} == --help ]]; then
  echo 'Usage: ./deploy.sh [root@SERVER DOMAIN]'
  echo 'Settings: DEPLOY_TARGET, DEPLOY_DOMAIN, DEPLOY_ENV_FILE'
  exit 0
fi
[[ $# == 0 || $# == 2 ]] || { echo 'Usage: ./deploy.sh [root@SERVER DOMAIN]' >&2; exit 2; }
# Load local deployment settings without committing infrastructure details.
if [[ -f .deploy.env ]]; then
  configured_target=${DEPLOY_TARGET:-}
  configured_domain=${DEPLOY_DOMAIN:-}
  source .deploy.env
  DEPLOY_TARGET=${configured_target:-${DEPLOY_TARGET:-}}
  DEPLOY_DOMAIN=${configured_domain:-${DEPLOY_DOMAIN:-}}
fi
target=${1:-${DEPLOY_TARGET:-}}
domain=${2:-${DEPLOY_DOMAIN:-}}
[[ -n $target && -n $domain ]] || { echo 'Set .deploy.env or run ./deploy.sh root@SERVER DOMAIN.' >&2; exit 2; }
env_file=${DEPLOY_ENV_FILE:-.env.production}
[[ $target =~ ^root@[a-zA-Z0-9.-]+$ ]] || { echo 'Use root@SERVER_IP or root@hostname.' >&2; exit 2; }
[[ $domain =~ ^[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?$ && $domain == *.* && $domain != *..* ]] || { echo 'Use a domain without a scheme or path.' >&2; exit 2; }
[[ -f $env_file ]] || { echo 'Copy deploy/production.env.example to .env.production first.' >&2; exit 1; }
for command in ssh scp tar python3; do command -v "$command" >/dev/null || { echo "$command is required" >&2; exit 1; }; done
bundle=$(mktemp -d)
remote_stage=''
cleanup() {
  rm -rf "$bundle"
  if [[ -n $remote_stage ]]; then ssh "$target" "rm -rf -- '$remote_stage'" || true; fi
}
trap cleanup EXIT
python3 scripts/prepare-deploy-env.py "$env_file" "$bundle/production.env"
# Explicit inclusion keeps local credentials and Git history out of the image.
COPYFILE_DISABLE=1 tar -czf "$bundle/source.tar.gz" Dockerfile .dockerignore go.mod go.sum cmd internal migrations web
remote_stage=$(ssh "$target" 'umask 077; mktemp -d /tmp/charitymatch-deploy.XXXXXXXX')
[[ $remote_stage =~ ^/tmp/charitymatch-deploy\.[a-zA-Z0-9]+$ ]] || { echo 'Unexpected remote staging path' >&2; exit 1; }
scp "$bundle/source.tar.gz" "$target:$remote_stage/source.tar.gz"
scp "$bundle/production.env" "$target:$remote_stage/production.env"
scp deploy/compose.yaml deploy/Caddyfile "$target:$remote_stage/"
ssh "$target" bash -s -- "$remote_stage" "$domain" <<'REMOTE'
set -euo pipefail
stage=$1
domain=$2
. /etc/os-release
[[ $ID == ubuntu && ( $VERSION_ID == 24.04 || $VERSION_ID == 26.04 || $VERSION_ID == 26.10 ) ]] || { echo 'This script supports Ubuntu 24.04, 26.04, and 26.10.' >&2; exit 1; }
if ! command -v docker >/dev/null || ! docker compose version >/dev/null 2>&1; then
  apt-get update
  DEBIAN_FRONTEND=noninteractive apt-get install -y docker.io docker-compose-v2 ca-certificates curl
fi
systemctl enable --now docker
install -d -m 700 /opt/charitymatch
cd /opt/charitymatch
tar -xzf "$stage/source.tar.gz"
install -m 600 "$stage/production.env" .env.production
install -m 600 "$stage/compose.yaml" compose.yaml
install -m 644 "$stage/Caddyfile" Caddyfile
printf 'DOMAIN=%s\n' "$domain" > .env
chmod 600 .env
# Apply pending schema migrations before replacing the running containers.
docker compose build --pull app
docker compose run --rm --no-deps app /app/db migrate
docker compose run --rm --no-deps app /app/db check
docker compose up -d --wait --wait-timeout 120
# Reload an existing proxy when deploying to a different domain.
docker compose exec -T caddy caddy reload --config /etc/caddy/Caddyfile
printf '\nContainers are running. Checking public HTTPS…\n'
curl --fail --silent --show-error --retry 12 --retry-delay 5 --retry-all-errors --max-time 10 --resolve "$domain:443:127.0.0.1" "https://$domain/healthz"
printf '\nDeployed: https://%s\n' "$domain"
REMOTE
