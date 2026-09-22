#!/bin/sh
# Build the native UI in an isolated copy. Cached dependencies only.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
if [ -n "${R4_NODE_BIN:-}" ]; then PATH="$(dirname "$R4_NODE_BIN"):$PATH"; export PATH; fi
node -e 'if (process.versions.node.split(".")[0] !== "24") throw new Error("R4 requires Node 24")'
mkdir -p "$ROOT/.local/redesign-r4"
TARGET=$(mktemp -d /private/tmp/mindcreek-r4-ui.XXXXXX)
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
printf '%s' "$TARGET" > "$ROOT/.local/redesign-r4/ui-path"
PYTHONPATH="$ROOT/tools/redesign" python3 - "$TARGET" <<'PY'
import json,sys,hashlib
from pathlib import Path
from r4_evidence import ROOT,ui_digest
p=Path(sys.argv[1])
digest=hashlib.sha256()
for f in sorted((p/'dist').rglob('*')):
    if f.is_file(): digest.update(str(f.relative_to(p/'dist')).encode()+b'\0'+f.read_bytes())
(ROOT/'docs/plans/evidence/r4-ui-build.json').write_text(json.dumps({'status':'passed','ui_source_sha256':ui_digest(),'bundle_sha256':digest.hexdigest(),'checks':['tests','type-check','build','native-boundary']},indent=2)+'\n')
PY
echo "R4 Node 24 UI verification passed"
