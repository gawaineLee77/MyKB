#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TEMP=$(mktemp -d "${TMPDIR:-/tmp}/mindcreek-phase4-clean.XXXXXX")
trap 'rm -rf "$TEMP"' EXIT HUP INT TERM
COPY="$TEMP/MyKB"

git clone --quiet --no-hardlinks "$ROOT" "$COPY"
git -C "$COPY" config submodule.upstream/weknora.url "$ROOT/upstream/weknora"
git -C "$COPY" -c protocol.file.allow=always submodule update --init --recursive upstream/weknora

git -C "$ROOT" diff --binary HEAD | git -C "$COPY" apply --index
git -C "$ROOT" ls-files --others --exclude-standard | while IFS= read -r FILE; do
  [ -n "$FILE" ] || continue
  mkdir -p "$COPY/$(dirname -- "$FILE")"
  cp -p "$ROOT/$FILE" "$COPY/$FILE"
done

git -C "$COPY" add -A
git -C "$COPY" -c user.name=MindCreek-CI -c user.email=ci@mindcreek.invalid commit --quiet -m "test: phase4 clean-copy candidate"
[ -z "$(git -C "$COPY" status --porcelain --untracked-files=all)" ] || {
  echo "Phase 4 clean-copy check failed: reconstructed checkout is dirty" >&2
  exit 1
}

make -C "$COPY" phase0-check
make -C "$COPY" phase1-route-policy-check
make -C "$COPY" phase2-sharing-model-check
make -C "$COPY" phase2-route-actions-check
make -C "$COPY" phase3-gate-a-static-check
make -C "$COPY" phase3-gate-b-static-check
make -C "$COPY" phase3-gate-c-static-check
make -C "$COPY" phase4-gate-a-static-check
make -C "$COPY" phase4-gate-b-static-check
make -C "$COPY" phase4-gate-c-static-check
make -C "$COPY" phase4-gate-d-static-check
make -C "$COPY" phase4-upstream-contract-check
mkdir -p "$COPY/.local/gateway-go-build"
(
  cd "$COPY/services/gateway"
  GOCACHE="$COPY/.local/gateway-go-build" go test -run '^$' ./...
  GOCACHE="$COPY/.local/gateway-go-build" go test ./internal/access ./internal/agentscope ./internal/agentaudit ./internal/mcp ./internal/capability ./internal/database
)
make -C "$COPY" phase4-compose-config

echo "MindCreek Phase 4 clean-copy checks passed"
