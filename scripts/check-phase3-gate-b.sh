#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PLAN="$ROOT/docs/PHASE3_IMPLEMENTATION_PLAN.md"
EVIDENCE="$ROOT/docs/PHASE3_GATE_B.md"
SERVER="$ROOT/services/gateway/internal/server/publication.go"

fail() {
  echo "Phase 3 Gate B check failed: $*" >&2
  exit 1
}

"$ROOT/scripts/verify-upstream.sh" >/dev/null
for FILE in \
  "$ROOT/services/gateway/internal/catalog/service.go" \
  "$ROOT/services/gateway/internal/access/gate.go" \
  "$ROOT/services/gateway/internal/library/service.go" \
  "$SERVER" \
  "$ROOT/scripts/phase3-gate-b-probe.py" \
  "$EVIDENCE"; do
  [ -f "$FILE" ] || fail "missing ${FILE#$ROOT/}"
done

for ROUTE in 'mindcreek/catalog' 'me/subscriptions' 'mark-seen' '/subscription' '/publication'; do
  rg -q "$ROUTE" "$SERVER" || fail "Phase 3 API route is missing: $ROUTE"
done
rg -q 'SourceOrganizationPublic' "$ROOT/services/gateway/internal/authorization/decision.go" || fail "organization-public decision source is missing"
rg -q 'SourceSubscription' "$ROOT/services/gateway/internal/authorization/decision.go" || fail "subscription decision source is missing"
rg -q 'HasSuffix.*"/download"' "$ROOT/services/gateway/internal/access/gate.go" || fail "original download restriction is missing"
rg -q 'ViewSubscribed' "$ROOT/services/gateway/internal/library/service.go" || fail "Subscribed library view is missing"
for TASK in P3-07 P3-08 P3-09 P3-10 P3-11 P3-12 P3-13 P3-14; do
  rg -q "\[x\].*$TASK" "$PLAN" || fail "$TASK is not recorded complete"
done
rg -q 'Gate B.*Passed on 2026-08-27' "$PLAN" || fail "Gate B acceptance is not recorded"
rg -q '^\| Status \| Passed on 2026-08-27 \|' "$EVIDENCE" || fail "Gate B evidence status is missing"
[ -z "$(git -C "$ROOT/upstream/weknora" status --porcelain --untracked-files=all)" ] || fail "upstream submodule is dirty"

echo "MindCreek Phase 3 Gate B APIs and enforcement verified"
