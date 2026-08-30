#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PLAN="$ROOT/docs/PHASE4_IMPLEMENTATION_PLAN.md"
HANDLER="$ROOT/services/gateway/internal/mcp/handler.go"
SERVICE="$ROOT/services/gateway/internal/mcp/service.go"

fail() {
  echo "Phase 4 Gate C check failed: $*" >&2
  exit 1
}

for FILE in "$HANDLER" "$SERVICE" "$ROOT/scripts/phase4-gate-c-probe.py" "$ROOT/docs/PHASE4_GATE_C.md"; do
  [ -f "$FILE" ] || fail "missing ${FILE#$ROOT/}"
done
rg -q 'ModernProtocol = "2026-07-28"' "$HANDLER" || fail "modern MCP protocol is missing"
rg -q 'LegacyProtocol = "2025-11-25"' "$HANDLER" || fail "legacy MCP compatibility is missing"
rg -q 'http.MaxBytesReader.*1<<20' "$HANDLER" || fail "MCP payload limit is missing"
rg -q 'NewFixedWindowLimiter' "$HANDLER" || fail "MCP rate limiter is missing"
for TOOL in list_knowledge_bases search_knowledge get_source_excerpt ask_knowledge_agent list_publications list_subscriptions; do
  rg -q "\"$TOOL\"" "$SERVICE" || fail "MCP tool is missing: $TOOL"
done
rg -q '"mcp": true' "$ROOT/config/phase4-capabilities.json" || fail "MCP release capability is disabled"
for TASK in P4-13 P4-14 P4-15 P4-16; do
  rg -q "\[x\].*$TASK" "$PLAN" || fail "$TASK is not recorded complete"
done
[ -z "$(git -C "$ROOT/upstream/weknora" status --porcelain --untracked-files=all)" ] || fail "upstream submodule is dirty"

echo "MindCreek Phase 4 Gate C authenticated hosted MCP verified"
