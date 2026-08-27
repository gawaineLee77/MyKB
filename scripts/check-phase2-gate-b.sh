#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PLAN="$ROOT/docs/PHASE2_IMPLEMENTATION_PLAN.md"
EVIDENCE="$ROOT/docs/PHASE2_GATE_B.md"

fail() {
  echo "Phase 2 Gate B check failed: $*" >&2
  exit 1
}

"$ROOT/scripts/verify-upstream.sh" >/dev/null

for FILE in \
  "$ROOT/config/phase2-route-actions.json" \
  "$ROOT/services/gateway/internal/access/gate.go" \
  "$ROOT/services/gateway/internal/library/service.go" \
  "$ROOT/services/gateway/internal/server/sharing.go" \
  "$ROOT/services/gateway/internal/sessionscope/repository.go" \
  "$ROOT/services/gateway/internal/audit/audit.go" \
  "$ROOT/services/gateway/internal/database/migrations/000007_phase2_security_records.up.sql" \
  "$ROOT/scripts/phase2-gate-b-probe.py" \
  "$EVIDENCE"; do
  [ -f "$FILE" ] || fail "missing ${FILE#$ROOT/}"
done

for TASK in P2-07 P2-08 P2-09 P2-10 P2-11 P2-12 P2-13 P2-14; do
  rg -q "\[x\].*$TASK" "$PLAN" || fail "$TASK is not recorded complete"
done
rg -q 'Gate B.*Passed on 2026-08-27' "$PLAN" || fail "Gate B acceptance is not recorded"
rg -q 'Status \| Passed on 2026-08-27' "$EVIDENCE" || fail "Gate B evidence status is missing"
rg -q 'session_kb_scopes' "$ROOT/services/gateway/internal/database/migrations/000007_phase2_security_records.up.sql" || fail "session reauthorization records are missing"
rg -q 'kb_access_audit_events' "$ROOT/services/gateway/internal/database/migrations/000007_phase2_security_records.up.sql" || fail "access audit records are missing"
[ -z "$(git -C "$ROOT/upstream/weknora" status --porcelain --untracked-files=all)" ] || fail "upstream submodule is dirty"

echo "MindCreek Phase 2 Gate B enforcement and APIs verified"
