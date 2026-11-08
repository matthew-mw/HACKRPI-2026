#!/bin/sh
# Run from any directory. Only source a .env file you trust.
set -eu
cd "$(dirname "$0")/.."
if [ -f .env ]; then
    set -a
    . ./.env
    set +a
fi
: "${DATABASE_URL:?Set DATABASE_URL in .env or your environment}"
case "${1:-run}" in
    run) exec go run ./cmd/server ;;
    check|init-db|migrate)
        exec go run ./cmd/db "$1"
        ;;
    *) echo 'Usage: sh scripts/dev.sh [run|check|init-db|migrate]' >&2; exit 2 ;;
esac
