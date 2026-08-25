#!/bin/sh
set -eu

EXPECTED_TAG="v0.7.2"
EXPECTED_COMMIT="3d5d8bfcdfeeea266b292b71cea616847af28d0f"
EXPECTED_URL="https://github.com/Tencent/WeKnora.git"

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
UPSTREAM_DIR="$REPO_ROOT/upstream/weknora"

fail() {
  echo "phase0 upstream check failed: $*" >&2
  exit 1
}

test -f "$REPO_ROOT/.gitmodules" || fail ".gitmodules is missing"
test -e "$UPSTREAM_DIR/.git" || fail "initialize submodules with: git submodule update --init --recursive"

ACTUAL_URL=$(git config -f "$REPO_ROOT/.gitmodules" --get submodule.upstream/weknora.url)
test "$ACTUAL_URL" = "$EXPECTED_URL" || fail "unexpected upstream URL: $ACTUAL_URL"

ACTUAL_COMMIT=$(git -C "$UPSTREAM_DIR" rev-parse HEAD)
test "$ACTUAL_COMMIT" = "$EXPECTED_COMMIT" || fail "expected $EXPECTED_COMMIT, found $ACTUAL_COMMIT"

ACTUAL_TAG=$(git -C "$UPSTREAM_DIR" describe --tags --exact-match 2>/dev/null || true)
test "$ACTUAL_TAG" = "$EXPECTED_TAG" || fail "expected tag $EXPECTED_TAG, found ${ACTUAL_TAG:-none}"

DIRTY=$(git -C "$UPSTREAM_DIR" status --porcelain --untracked-files=all)
test -z "$DIRTY" || fail "upstream worktree is modified; use product-owned modules or record an approved patch"

SUBMODULE_STATUS=$(git -C "$REPO_ROOT" submodule status upstream/weknora)
case "$SUBMODULE_STATUS" in
  " $EXPECTED_COMMIT"*) ;;
  *) fail "unexpected submodule status: $SUBMODULE_STATUS" ;;
esac

echo "WeKnora baseline verified: $EXPECTED_TAG ($EXPECTED_COMMIT)"
echo "Downstream worktree patches: 0"
