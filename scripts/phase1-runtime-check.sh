#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
COMPOSE="$ROOT/scripts/phase1-compose.sh"

fail() {
  echo "MindCreek Phase 1 runtime check failed: $*" >&2
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

APP_HEALTH=$(docker inspect MindCreek-app --format '{{.State.Health.Status}}')
test "$APP_HEALTH" = "healthy" || fail "application health is $APP_HEALTH"
GATEWAY_HEALTH=$(docker inspect MindCreek-gateway --format '{{.State.Health.Status}}')
test "$GATEWAY_HEALTH" = "healthy" || fail "gateway health is $GATEWAY_HEALTH"

APP_BINDINGS=$(docker port MindCreek-app)
test -z "$APP_BINDINGS" || fail "WeKnora app has a published host port: $APP_BINDINGS"
GATEWAY_BINDINGS=$(docker port MindCreek-gateway)
test -z "$GATEWAY_BINDINGS" || fail "gateway bypass port is published: $GATEWAY_BINDINGS"

FRONTEND_GATEWAY=$(docker inspect MindCreek-frontend --format '{{range .Config.Env}}{{println .}}{{end}}' | grep '^APP_HOST=')
test "$FRONTEND_GATEWAY" = "APP_HOST=gateway" || fail "frontend is not routed through gateway"

INDEX=$(curl --fail --silent --show-error "http://127.0.0.1:${FRONTEND_PORT:-18080}/")
printf '%s' "$INDEX" | grep -q '<title>MindCreek</title>' || fail "MindCreek browser title is missing"

python3 "$ROOT/scripts/phase1-policy-probe.py"
"$ROOT/scripts/verify-upstream.sh" >/dev/null

echo "MindCreek Phase 1 runtime verified: gateway-only API path, private upstream, enforced exclusions"
