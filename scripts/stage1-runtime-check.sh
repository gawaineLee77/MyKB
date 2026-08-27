#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
COMPOSE="$ROOT/scripts/mindcreek-compose.sh"

fail() {
  echo "MindCreek Stage 1 runtime check failed: $*" >&2
  exit 1
}

EXPECTED_SERVICES='app
docreader
frontend
mock-embedding
postgres
redis'
RUNNING_SERVICES=$($COMPOSE ps --status running --services | sort)
test "$RUNNING_SERVICES" = "$EXPECTED_SERVICES" || fail "not all six services are running"

APP_HEALTH=$(docker inspect MindCreek-app --format '{{.State.Health.Status}}')
test "$APP_HEALTH" = "healthy" || fail "application health is $APP_HEALTH"

FRONTEND_IMAGE=$(docker inspect MindCreek-frontend --format '{{.Config.Image}}')
case "$FRONTEND_IMAGE" in
  mindcreek-ui:*) ;;
  *) fail "unexpected frontend image: $FRONTEND_IMAGE" ;;
esac

INDEX=$(curl --fail --silent --show-error http://127.0.0.1:${FRONTEND_PORT:-18080}/)
printf '%s' "$INDEX" | grep -q '<title>MindCreek</title>' || fail "MindCreek browser title is missing"
printf '%s' "$INDEX" | grep -q '/mindcreek-favicon.png' || fail "MindCreek favicon link is missing"
curl --fail --silent --show-error --output /dev/null \
  http://127.0.0.1:${FRONTEND_PORT:-18080}/mindcreek-favicon.png
curl --fail --silent --show-error --output /dev/null \
  http://127.0.0.1:${APP_PORT:-18081}/health

"$ROOT/scripts/verify-upstream.sh" >/dev/null

echo "MindCreek Stage 1 runtime verified: six services, branded UI, healthy backend"
