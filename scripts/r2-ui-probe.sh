#!/bin/sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
node -e 'if (process.versions.node.split(".")[0] !== "24") throw new Error("Use Node 24")'
: "${R2_CHROME_EXECUTABLE:?set a disposable-browser executable}"
BUILD_ROOT=$(mktemp -d /private/tmp/mindcreek-r2-ui.XXXXXX)
# Keep the entire served bundle outside Desktop while a browser is running.
rsync -a --exclude node_modules --exclude dist --exclude '.env*' "$ROOT/upstream/weknora/frontend/" "$BUILD_ROOT/"
node "$ROOT/tools/frontend-overlay/apply.mjs" "$BUILD_ROOT" "$ROOT/branding/mindcreek"
ln -s "$ROOT/upstream/weknora/frontend/node_modules" "$BUILD_ROOT/node_modules"
(cd "$BUILD_ROOT" && VITE_IS_DOCKER=true npm run build)
python3 "$ROOT/tools/redesign/r2_ui_server.py" "$BUILD_ROOT/dist" &
server_pid=$!
trap 'kill "$server_pid" 2>/dev/null || true' EXIT HUP INT TERM
R2_REPORT_ROOT="$BUILD_ROOT" node "$ROOT/tools/redesign/r2_ui_probe.mjs"
mkdir -p "$ROOT/docs/assets/r2"
cp "$BUILD_ROOT/docs/assets/r2/"*.png "$ROOT/docs/assets/r2/"
cp "$BUILD_ROOT/docs/plans/evidence/r2-ui-probe.json" "$ROOT/docs/plans/evidence/"
printf 'R2 UI probe passed; temporary build and screenshot originals: %s\n' "$BUILD_ROOT"
