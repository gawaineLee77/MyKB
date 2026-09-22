#!/bin/sh
set -eu

case "${1:-}" in
  ""|--offline) ;;
  *) echo "usage: sh scripts/test-product-frontend.sh [--offline]" >&2; exit 2 ;;
esac

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
UPSTREAM="$ROOT/upstream/weknora/frontend"
if [ "${1:-}" = --offline ] && [ ! -d "$UPSTREAM/node_modules" ]; then
  echo "Offline frontend tests require installed upstream frontend dependencies" >&2
  exit 2
fi

BUILD_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/mindcreek-product-ui-test.XXXXXX")
trap 'rm -rf "$BUILD_ROOT"' EXIT HUP INT TERM

# Keep generated overlays, dependencies, and build output outside the submodule.
# Do not load developer-specific environment files into the test build.
rsync -a --exclude node_modules --exclude dist --exclude '.env*' "$UPSTREAM/" "$BUILD_ROOT/"
node "$ROOT/tools/frontend-overlay/apply.mjs" "$BUILD_ROOT" "$ROOT/branding/mindcreek"

if [ "${1:-}" = --offline ]; then
  ln -s "$UPSTREAM/node_modules" "$BUILD_ROOT/node_modules"
else
  (cd "$BUILD_ROOT" && npm ci --no-audit --no-fund)
fi

(
  cd "$BUILD_ROOT"
  npm test
  npm run type-check
  VITE_IS_DOCKER=true npm run build
)

echo "MindCreek frontend tests, type-check, and build passed in an isolated copy"
