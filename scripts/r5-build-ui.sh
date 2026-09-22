#!/bin/sh
# Build the native UI in an isolated copy. Cached dependencies only.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
if [ -n "${R5_NODE_BIN:-}" ]; then PATH="$(dirname "$R5_NODE_BIN"):$PATH"; export PATH; fi
node -e 'if (process.versions.node.split(".")[0] !== "24") throw new Error("R5 requires Node 24")'
mkdir -p "$ROOT/.local/redesign-r5"
TARGET=$(mktemp -d /private/tmp/mindcreek-r5-ui.XXXXXX)
rsync -a --exclude node_modules --exclude dist --exclude '.env*' "$ROOT/upstream/weknora/frontend/" "$TARGET/"
node "$ROOT/tools/frontend-overlay/apply.mjs" "$TARGET" "$ROOT/branding/mindcreek"
ln -s "$ROOT/upstream/weknora/frontend/node_modules" "$TARGET/node_modules"
node "$ROOT/tools/frontend-overlay/check-native.mjs" "$TARGET" "$ROOT/upstream/weknora/frontend"
(
  cd "$TARGET"
  npm test
  npm run type-check
  VITE_IS_DOCKER=true npm run build
)
printf '%s' "$TARGET" > "$ROOT/.local/redesign-r5/ui-path"
python3 "$ROOT/tools/redesign/r5_evidence.py" record-ui
echo "R5 Node 24 UI verification passed"
