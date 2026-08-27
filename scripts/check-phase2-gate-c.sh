#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PLAN="$ROOT/docs/PHASE2_IMPLEMENTATION_PLAN.md"
EVIDENCE="$ROOT/docs/PHASE2_GATE_C.md"

fail() {
  echo "Phase 2 Gate C check failed: $*" >&2
  exit 1
}

for FILE in \
  "$ROOT/tools/frontend-overlay/product/mindcreek/KnowledgeLibrary.vue" \
  "$ROOT/tools/frontend-overlay/product/mindcreek/SharingDialog.vue" \
  "$ROOT/tools/frontend-overlay/product/mindcreek/RAGWorkspace.vue" \
  "$ROOT/tools/frontend-overlay/product/mindcreek/api.ts" \
  "$EVIDENCE"; do
  [ -f "$FILE" ] || fail "missing ${FILE#$ROOT/}"
done

for TASK in P2-15 P2-16 P2-17; do
  rg -q "\[x\].*$TASK" "$PLAN" || fail "$TASK is not recorded complete"
done
rg -q 'Gate C.*Passed on 2026-08-27' "$PLAN" || fail "Gate C acceptance is not recorded"
rg -q 'mindcreek/KnowledgeLibrary.vue' "$ROOT/tools/frontend-overlay/apply.mjs" || fail "authorized library is not routed"
rg -q 'listKnowledgeLibrary' "$ROOT/tools/frontend-overlay/product/mindcreek/KnowledgeLibrary.vue" || fail "authorized views are not used"
rg -q "product_mode !== 'personal_notes'" "$ROOT/tools/frontend-overlay/product/mindcreek/KnowledgeLibrary.vue" || fail "Personal Notes sharing control is not suppressed"
rg -q 'grant.revision_conflict' "$ROOT/tools/frontend-overlay/product/mindcreek/SharingDialog.vue" || fail "sharing concurrency feedback is missing"
rg -q 'access\.can_edit_content' "$ROOT/tools/frontend-overlay/product/mindcreek/RAGWorkspace.vue" || fail "server permission-aware controls are missing"
"$ROOT/tools/frontend-overlay/check.sh" >/dev/null
[ -z "$(git -C "$ROOT/upstream/weknora" status --porcelain --untracked-files=all)" ] || fail "upstream submodule is dirty"

echo "MindCreek Phase 2 Gate C product UI verified"
