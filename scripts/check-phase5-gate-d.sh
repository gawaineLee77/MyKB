#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PLAN="$ROOT/docs/PHASE5_IMPLEMENTATION_PLAN.md"
LEDGER="$ROOT/docs/UPSTREAM_PATCHES.md"

fail() { echo "Phase 5 Gate D check failed: $*" >&2; exit 1; }
for file in \
  config/phase5-capabilities.json \
  images/manifests/phase5-runtime.txt \
  testdata/phase5/pilot-benchmark.json \
  scripts/phase5-pilot-probe.py \
  scripts/check-phase5-upstream-contract.sh \
  scripts/phase5-clean-copy-check.sh \
  scripts/phase5-compose-from-phase0.sh \
  scripts/phase5-server-reset.sh \
  docs/PHASE5_PILOT.md \
  docs/PHASE5_OPERATIONS.md \
  docs/PHASE5_FRESH_SERVER_INSTALL.md \
  docs/PHASE5_GATE_D.md; do
  [ -f "$ROOT/$file" ] || fail "missing $file"
done
for task in P5-19 P5-20 P5-21; do
  rg -q "\[x\].*$task" "$PLAN" || fail "$task is not recorded complete"
done
for image in mindcreek-ui:phase5 mindcreek-ui:0.6.0 mindcreek-gateway:phase5 mindcreek-gateway:0.6.0-phase5; do
  rg -q "^$image$" "$ROOT/images/manifests/phase5-runtime.txt" || fail "release image tag is missing: $image"
done
rg -q '"phase": "phase5"' "$ROOT/config/phase5-capabilities.json" || fail "Phase 5 capability registry is not selected"
rg -q -- '--confirm-destroy-all-mindcreek-data' "$ROOT/scripts/phase5-server-reset.sh" || fail "fresh server reset lacks destructive confirmation"
rg -q '^\| Ledger status \| No downstream patches \|' "$LEDGER" || fail "upstream patch ledger is not empty"
[ -z "$(git -C "$ROOT/upstream/weknora" status --porcelain --untracked-files=all)" ] || fail "upstream submodule is dirty"
echo "MindCreek Phase 5 Gate D release contract verified"
