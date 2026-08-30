#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
UPSTREAM="$ROOT/upstream/weknora/frontend"
BUILD_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/mindcreek-phase5-ui-build.XXXXXX")
trap 'rm -rf "$BUILD_ROOT"' EXIT HUP INT TERM

[ -d "$UPSTREAM/node_modules" ] || {
  echo "offline UI build requires upstream/weknora/frontend/node_modules; run the frontend dependency setup while online first" >&2
  exit 2
}

rsync -a --exclude node_modules "$UPSTREAM/" "$BUILD_ROOT/"
ln -s "$UPSTREAM/node_modules" "$BUILD_ROOT/node_modules"
node "$ROOT/tools/frontend-overlay/apply.mjs" "$BUILD_ROOT" "$ROOT/branding/mindcreek"
(
  cd "$BUILD_ROOT"
  npm test
  npm run type-check
  npm run build
)
rm "$BUILD_ROOT/node_modules"

docker build \
  --file "$ROOT/images/mindcreek-ui/Dockerfile.offline" \
  --build-arg MINDCREEK_UI_VERSION=0.6.0 \
  --tag mindcreek-ui:phase5 \
  "$BUILD_ROOT"

echo "Built mindcreek-ui:phase5 from the pinned local dependencies and Phase 4 runtime base"
