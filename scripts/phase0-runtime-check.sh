#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
COMPOSE="$ROOT/scripts/phase0-compose.sh"
EXPECTED_COMMIT=3d5d8bfcdfeeea266b292b71cea616847af28d0f
EXPECTED_VERSION=v0.7.2

fail() {
  echo "phase0 runtime check failed: $*" >&2
  exit 1
}

EXPECTED_SERVICES=$(printf '%s\n' app docreader frontend mock-embedding postgres redis | sort)
ACTUAL_SERVICES=$("$COMPOSE" config --services | sort)
test "$ACTUAL_SERVICES" = "$EXPECTED_SERVICES" || fail "unexpected Compose service set"

for SERVICE in app docreader mock-embedding postgres; do
  CONTAINER_ID=$("$COMPOSE" ps -q "$SERVICE")
  test -n "$CONTAINER_ID" || fail "$SERVICE is not running"
  HEALTH=$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$CONTAINER_ID")
  test "$HEALTH" = "healthy" || fail "$SERVICE status is $HEALTH"
done

for SERVICE in frontend redis; do
  CONTAINER_ID=$("$COMPOSE" ps -q "$SERVICE")
  test -n "$CONTAINER_ID" || fail "$SERVICE is not running"
  STATUS=$(docker inspect --format '{{.State.Status}}' "$CONTAINER_ID")
  test "$STATUS" = "running" || fail "$SERVICE status is $STATUS"
done

IMAGE=wechatopenai/weknora-app:$EXPECTED_VERSION
VERSION=$(docker image inspect "$IMAGE" --format '{{index .Config.Labels "org.opencontainers.image.version"}}')
REVISION=$(docker image inspect "$IMAGE" --format '{{index .Config.Labels "org.opencontainers.image.revision"}}')
test "$VERSION" = "$EXPECTED_VERSION" || fail "app image version is $VERSION"
test "$REVISION" = "$EXPECTED_COMMIT" || fail "app image revision is $REVISION"

curl --fail --silent --show-error http://127.0.0.1:18081/health >/dev/null
FRONTEND_STATUS=$(curl --silent --output /dev/null --write-out '%{http_code}' http://127.0.0.1:18080/)
test "$FRONTEND_STATUS" = "200" || fail "frontend returned HTTP $FRONTEND_STATUS"

echo "Phase 0 runtime verified: $EXPECTED_VERSION ($EXPECTED_COMMIT), six expected services"
