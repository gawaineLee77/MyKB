#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PLAN="$ROOT/docs/PHASE4_IMPLEMENTATION_PLAN.md"
ASK="$ROOT/tools/frontend-overlay/product/mindcreek/AskWorkspace.vue"

fail() {
  echo "Phase 4 Gate B check failed: $*" >&2
  exit 1
}

for FILE in \
  "$ROOT/services/gateway/internal/server/agent.go" \
  "$ROOT/services/gateway/internal/weknora/client.go" \
  "$ROOT/services/gateway/internal/access/gate.go" \
  "$ASK" \
  "$ROOT/scripts/phase4-gate-b-probe.py" \
  "$ROOT/docs/PHASE4_GATE_B.md"; do
  [ -f "$FILE" ] || fail "missing ${FILE#$ROOT/}"
done

rg -q 'mindcreek/agent/scope' "$ROOT/services/gateway/internal/server/agent.go" || fail "scope API is missing"
rg -q 'validateScopedSearchResponse' "$ROOT/services/gateway/internal/access/gate.go" || fail "search result scope defense is missing"
rg -q 'web_search_enabled.*false' "$ROOT/services/gateway/internal/access/gate.go" || fail "web-search suppression is missing"
rg -q 'resolveAgentScope' "$ASK" || fail "Ask does not preview effective scope"
rg -q 'builtin-smart-reasoning' "$ASK" || fail "Ask reasoning mode is missing"
rg -q 'data-testid="ask-button"' "$ROOT/tools/frontend-overlay/product/mindcreek/KnowledgeLibrary.vue" || fail "Ask navigation is missing"
for TASK in P4-07 P4-08 P4-09 P4-10 P4-11 P4-12; do
  rg -q "\[x\].*$TASK" "$PLAN" || fail "$TASK is not recorded complete"
done
"$ROOT/tools/frontend-overlay/check.sh" >/dev/null
[ -z "$(git -C "$ROOT/upstream/weknora" status --porcelain --untracked-files=all)" ] || fail "upstream submodule is dirty"

echo "MindCreek Phase 4 Gate B Web agent and retrieval baseline verified"
