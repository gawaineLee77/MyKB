#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
COMPOSE="$ROOT/scripts/phase5-compose.sh"

fail() {
  echo "MindCreek Phase 5 runtime check failed: $*" >&2
  exit 1
}

EXPECTED_SERVICES='app
docreader
frontend
gateway
mock-embedding
postgres
redis'
RUNNING_SERVICES=$($COMPOSE ps --status running --services | sort)
test "$RUNNING_SERVICES" = "$EXPECTED_SERVICES" || fail "not all seven services are running"

test "$(docker inspect MindCreek-app --format '{{.State.Health.Status}}')" = "healthy" || fail "application is not healthy"
test "$(docker inspect MindCreek-gateway --format '{{.State.Health.Status}}')" = "healthy" || fail "gateway is not healthy"
test -z "$(docker port MindCreek-app)" || fail "WeKnora app has a published host port"
test -z "$(docker port MindCreek-gateway)" || fail "gateway bypass port is published"

FRONTEND_IMAGE=$(docker inspect MindCreek-frontend --format '{{.Config.Image}}')
GATEWAY_IMAGE=$(docker inspect MindCreek-gateway --format '{{.Config.Image}}')
test "$FRONTEND_IMAGE" = "${MINDCREEK_PHASE5_UI_IMAGE:-mindcreek-ui:phase5}" || fail "unexpected frontend image: $FRONTEND_IMAGE"
test "$GATEWAY_IMAGE" = "${MINDCREEK_PHASE5_GATEWAY_IMAGE:-mindcreek-gateway:phase5}" || fail "unexpected gateway image: $GATEWAY_IMAGE"

INDEX=$(curl --fail --silent --show-error "http://127.0.0.1:${FRONTEND_PORT:-18080}/")
printf '%s' "$INDEX" | rg -q '<title>MindCreek</title>' || fail "MindCreek browser title is missing"
curl --fail --silent --show-error --output /dev/null "http://127.0.0.1:${FRONTEND_PORT:-18080}/health"

test "$(stat -f '%Lp' "$ROOT/.local/phase5/builtin_models.yaml")" = "600" || fail "managed model declaration permissions are not 0600"
"$ROOT/scripts/verify-upstream.sh" >/dev/null

echo "MindCreek Phase 5 runtime verified: private upstream, managed model declarations, and gateway-only access"
