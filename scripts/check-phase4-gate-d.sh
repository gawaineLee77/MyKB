#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PLAN="$ROOT/docs/PHASE4_IMPLEMENTATION_PLAN.md"
LEDGER="$ROOT/docs/UPSTREAM_PATCHES.md"

fail() {
  echo "Phase 4 Gate D check failed: $*" >&2
  exit 1
}

for FILE in \
  "$ROOT/config/phase4-capabilities.json" \
  "$ROOT/images/manifests/phase4-runtime.txt" \
  "$ROOT/scripts/check-phase4-upstream-contract.sh" \
  "$ROOT/scripts/phase4-clean-copy-check.sh" \
  "$ROOT/docs/PHASE4_OPERATIONS.md" \
  "$ROOT/docs/PHASE4_GATE_D.md"; do
  [ -f "$FILE" ] || fail "missing ${FILE#$ROOT/}"
done
rg -q '\[x\].*P4-17' "$PLAN" || fail "P4-17 is not recorded complete"
rg -q '\[x\].*P4-18' "$PLAN" || fail "P4-18 is not recorded complete"
rg -q '"phase": "phase4"' "$ROOT/config/phase4-capabilities.json" || fail "Phase 4 capability registry is not selected"
rg -q 'mindcreek-gateway:phase4' "$ROOT/images/manifests/phase4-runtime.txt" || fail "Phase 4 gateway image is absent"
rg -q 'mindcreek-ui:phase4' "$ROOT/images/manifests/phase4-runtime.txt" || fail "Phase 4 UI image is absent"
rg -q '0.5.0-phase4' "$ROOT/deploy/phase1/compose.gateway.yml" || fail "Phase 4 version is absent"
rg -q '^\| Ledger status \| No downstream patches \|' "$LEDGER" || fail "upstream patch ledger is not empty"
rg -q '^\| Last reviewed \| 2026-' "$LEDGER" || fail "upstream ledger review is stale"
[ -z "$(git -C "$ROOT/upstream/weknora" status --porcelain --untracked-files=all)" ] || fail "upstream submodule is dirty"

echo "MindCreek Phase 4 Gate D release contract verified"
