#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PLAN="$ROOT/docs/PHASE3_IMPLEMENTATION_PLAN.md"
EVIDENCE="$ROOT/docs/PHASE3_GATE_D.md"
LEDGER="$ROOT/docs/UPSTREAM_PATCHES.md"

fail() {
  echo "Phase 3 Gate D check failed: $*" >&2
  exit 1
}

for FILE in \
  "$ROOT/config/phase3-capabilities.json" \
  "$ROOT/scripts/check-phase3-upstream-contract.sh" \
  "$ROOT/scripts/phase3-clean-copy-check.sh" \
  "$ROOT/images/manifests/stage1-runtime.txt" \
  "$EVIDENCE"; do
  [ -f "$FILE" ] || fail "missing ${FILE#$ROOT/}"
done

rg -q '\[x\].*P3-18' "$PLAN" || fail "P3-18 is not recorded complete"
rg -q 'Gate D.*Passed on 2026-08-27' "$PLAN" || fail "Gate D acceptance is not recorded"
rg -q '^\| Status \| Passed on 2026-08-27 \|' "$EVIDENCE" || fail "Gate D evidence is not passed"
rg -q '"phase": "phase3"' "$ROOT/config/phase3-capabilities.json" || fail "Phase 3 capability registry is not selected"
# A later release may become the deployment default; the Phase 3 registry and
# compatibility image alias must remain reproducible for historical checks.
rg -q 'mindcreek-gateway:phase3' "$ROOT/scripts/build-gateway-image-offline.sh" || fail "Phase 3 compatibility image tag is absent"
rg -q '^\| Ledger status \| No downstream patches \|' "$LEDGER" || fail "upstream patch ledger is not empty"
rg -q '^\| Last reviewed \| 2026-' "$LEDGER" || fail "upstream ledger review is missing"
[ -z "$(git -C "$ROOT/upstream/weknora" status --porcelain --untracked-files=all)" ] || fail "upstream submodule is dirty"

echo "MindCreek Phase 3 Gate D release contract verified"
