#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PLAN="$ROOT/docs/PHASE2_IMPLEMENTATION_PLAN.md"
EVIDENCE="$ROOT/docs/PHASE2_GATE_D.md"
LEDGER="$ROOT/docs/UPSTREAM_PATCHES.md"

fail() {
  echo "Phase 2 Gate D check failed: $*" >&2
  exit 1
}

for FILE in \
  "$ROOT/config/phase2-capabilities.json" \
  "$ROOT/scripts/check-phase2-upstream-contract.sh" \
  "$ROOT/scripts/phase2-clean-copy-check.sh" \
  "$ROOT/images/manifests/stage1-runtime.txt" \
  "$EVIDENCE"; do
  [ -f "$FILE" ] || fail "missing ${FILE#$ROOT/}"
done

rg -q '\[x\].*P2-18' "$PLAN" || fail "P2-18 is not recorded complete"
rg -q 'Gate D.*Passed on 2026-08-27' "$PLAN" || fail "Gate D acceptance is not recorded"
rg -q '^\| Status \| Passed on 2026-08-27 \|' "$EVIDENCE" || fail "Gate D evidence is not passed"
rg -q '"phase": "phase2"' "$ROOT/config/phase2-capabilities.json" || fail "Phase 2 capability registry is not selected"
rg -q 'mindcreek-gateway:phase2' "$ROOT/images/manifests/stage1-runtime.txt" || fail "Phase 2 gateway image is absent from the runtime manifest"
rg -q '^\| Ledger status \| No downstream patches \|' "$LEDGER" || fail "upstream patch ledger is not empty"
rg -q '^\| Last reviewed \| 2026-08-27 \|' "$LEDGER" || fail "upstream ledger review is stale"
[ -z "$(git -C "$ROOT/upstream/weknora" status --porcelain --untracked-files=all)" ] || fail "upstream submodule is dirty"

echo "MindCreek Phase 2 Gate D release contract verified"
