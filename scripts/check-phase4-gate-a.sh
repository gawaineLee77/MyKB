#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PLAN="$ROOT/docs/PHASE4_IMPLEMENTATION_PLAN.md"
MIGRATION="$ROOT/services/gateway/internal/database/migrations/000009_phase4_agent_operations.up.sql"

fail() {
  echo "Phase 4 Gate A check failed: $*" >&2
  exit 1
}

"$ROOT/scripts/verify-upstream.sh" >/dev/null
for FILE in \
  "$ROOT/services/gateway/internal/agentscope/resolver.go" \
  "$ROOT/services/gateway/internal/agentscope/resolver_test.go" \
  "$ROOT/services/gateway/internal/agentaudit/audit.go" \
  "$ROOT/services/gateway/internal/access/gate.go" \
  "$MIGRATION" \
  "$ROOT/services/gateway/internal/database/migrations/000009_phase4_agent_operations.down.sql" \
  "$ROOT/docs/PHASE4_GATE_A.md"; do
  [ -f "$FILE" ] || fail "missing ${FILE#$ROOT/}"
done

rg -q 'const MaxKnowledgeBases = 64' "$ROOT/services/gateway/internal/agentscope/resolver.go" || fail "scope limit is missing"
rg -q 'SourceOrganizationPublic' "$ROOT/services/gateway/internal/agentscope/resolver.go" || fail "public default exclusion is missing"
rg -q 'SelectionExplicit' "$ROOT/services/gateway/internal/agentscope/resolver.go" || fail "explicit scope is missing"
rg -q 'CREATE TABLE mindcreek.agent_operation_audit_events' "$MIGRATION" || fail "agent audit table is missing"
if rg -qi '(prompt|answer|excerpt|content)[[:space:]]+(text|json)' "$MIGRATION"; then
  fail "agent audit migration contains a sensitive payload column"
fi
rg -q 'first_count != "9"' "$ROOT/scripts/phase1-migration-probe.py" || fail "migration probe does not expect nine migrations"
for TASK in P4-01 P4-02 P4-03 P4-04 P4-05 P4-06; do
  rg -q "\[x\].*$TASK" "$PLAN" || fail "$TASK is not recorded complete"
done
[ -z "$(git -C "$ROOT/upstream/weknora" status --porcelain --untracked-files=all)" ] || fail "upstream submodule is dirty"

echo "MindCreek Phase 4 Gate A authorized-scope foundation verified"
