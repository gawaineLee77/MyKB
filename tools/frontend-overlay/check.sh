#!/bin/sh
set -eu
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
UPSTREAM_ROOT=${MINDCREEK_CANDIDATE_WEKNORA:-$REPO_ROOT/upstream/weknora}
TARGET=$(mktemp -d "${TMPDIR:-/tmp}/mindcreek-r4-overlay.XXXXXX")
trap 'rm -rf "$TARGET"' EXIT HUP INT TERM
rsync -a --exclude node_modules --exclude dist --exclude '.env*' "$UPSTREAM_ROOT/frontend/" "$TARGET/"
node "$SCRIPT_DIR/apply.mjs" "$TARGET" "$REPO_ROOT/branding/mindcreek"
node "$SCRIPT_DIR/check-native.mjs" "$TARGET" "$UPSTREAM_ROOT/frontend"
if [ "$UPSTREAM_ROOT" = "$REPO_ROOT/upstream/weknora" ]; then
  test -z "$(git -C "$UPSTREAM_ROOT" status --porcelain --untracked-files=all)"
fi
