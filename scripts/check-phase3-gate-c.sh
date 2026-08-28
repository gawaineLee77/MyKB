#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PLAN="$ROOT/docs/PHASE3_IMPLEMENTATION_PLAN.md"
EVIDENCE="$ROOT/docs/PHASE3_GATE_C.md"
LIBRARY="$ROOT/tools/frontend-overlay/product/mindcreek/KnowledgeLibrary.vue"

fail() {
  echo "Phase 3 Gate C check failed: $*" >&2
  exit 1
}

for FILE in \
  "$LIBRARY" \
  "$ROOT/tools/frontend-overlay/product/mindcreek/CatalogView.vue" \
  "$ROOT/tools/frontend-overlay/product/mindcreek/PublicationDialog.vue" \
  "$ROOT/tools/frontend-overlay/product/mindcreek/api.ts" \
  "$EVIDENCE"; do
  [ -f "$FILE" ] || fail "missing ${FILE#$ROOT/}"
done

for TASK in P3-15 P3-16 P3-17; do
  rg -q "\[x\].*$TASK" "$PLAN" || fail "$TASK is not recorded complete"
done
rg -q 'Gate C.*Passed on 2026-08-27' "$PLAN" || fail "Gate C acceptance is not recorded"
rg -q 'data-testid="subscribed-tab"' "$LIBRARY" || fail "Subscribed tab is missing"
rg -q 'data-testid="discover-tab"' "$LIBRARY" || fail "Discover tab is missing"
rg -q 'PublicationDialog' "$LIBRARY" || fail "publication controls are missing"
rg -q 'markPublicationSeen' "$LIBRARY" || fail "mark-seen behavior is missing"
rg -q "product_mode !== 'personal_notes'" "$LIBRARY" || fail "Personal Notes publication control is not suppressed"
"$ROOT/tools/frontend-overlay/check.sh" >/dev/null
[ -z "$(git -C "$ROOT/upstream/weknora" status --porcelain --untracked-files=all)" ] || fail "upstream submodule is dirty"

echo "MindCreek Phase 3 Gate C product UI verified"
