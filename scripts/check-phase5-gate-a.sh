#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PLAN="$ROOT/docs/PHASE5_IMPLEMENTATION_PLAN.md"

fail() {
  echo "Phase 5 Gate A check failed: $*" >&2
  exit 1
}

for FILE in \
  "$ROOT/config/phase5-capabilities.json" \
  "$ROOT/config/phase5-capabilities-overrides.json" \
  "$ROOT/deploy/phase5/builtin_models.yaml.tmpl" \
  "$ROOT/deploy/phase5/builtin_agents.yaml" \
  "$ROOT/deploy/phase5/compose.managed-models.yml" \
  "$ROOT/scripts/render-phase5-models.py" \
  "$ROOT/scripts/phase5-gate-a-probe.py" \
  "$ROOT/services/gateway/internal/managedmodel/service.go" \
  "$ROOT/services/gateway/internal/managedmodel/service_test.go" \
  "$ROOT/tools/frontend-overlay/product/mindcreek/ManagedModelSettings.vue" \
  "$ROOT/tools/frontend-overlay/product/mindcreek/AdvancedModelSettings.vue" \
  "$ROOT/docs/PHASE5_GATE_A.md"; do
  [ -f "$FILE" ] || fail "missing ${FILE#$ROOT/}"
done

for MODEL_ID in builtin-mindcreek-chat builtin-mindcreek-embedding builtin-mindcreek-rerank; do
  rg -q "$MODEL_ID" "$ROOT/services/gateway/internal/managedmodel/service.go" || fail "$MODEL_ID is absent from the product contract"
  rg -q "$MODEL_ID" "$ROOT/deploy/phase5/builtin_models.yaml.tmpl" || fail "$MODEL_ID is absent from the deployment template"
done
rg -q 'model_id: "builtin-mindcreek-chat"' "$ROOT/deploy/phase5/builtin_agents.yaml" || fail "Smart Reasoning chat default is missing"
rg -q 'rerank_model_id: "builtin-mindcreek-rerank"' "$ROOT/deploy/phase5/builtin_agents.yaml" || fail "Smart Reasoning reranker is missing"
rg -q 'id: "builtin-quick-answer"' "$ROOT/deploy/phase5/builtin_agents.yaml" || fail "Quick Answer managed profile is missing"
rg -q '"user_model_overrides": false' "$ROOT/config/phase5-capabilities.json" || fail "secure override default is not false"
rg -q '"user_model_overrides": true' "$ROOT/config/phase5-capabilities-overrides.json" || fail "explicit override opt-in registry is missing"
rg -q 'MINDCREEK_MODEL_OVERRIDE_HOSTS' "$ROOT/services/gateway/internal/config/config.go" || fail "override host allow-list is missing"
rg -q 'SYSTEM_AES_KEY' "$ROOT/scripts/render-phase5-models.py" || fail "override encryption-key validation is missing"
rg -q 'Cache-Control.*no-store' "$ROOT/services/gateway/internal/server/models.go" || fail "safe model facade is cacheable"
rg -q 'models.raw_route_disabled' "$ROOT/services/gateway/internal/server/models.go" || fail "raw upstream model mutations are not closed"
rg -q 'getManagedModels' "$ROOT/tools/frontend-overlay/product/mindcreek/api.ts" || fail "safe frontend model facade is unused"
rg -q 'testManagedModel' "$ROOT/tools/frontend-overlay/product/mindcreek/api.ts" || fail "managed model connection test is unavailable"
! rg -q "@/api/model" "$ROOT/tools/frontend-overlay/product/mindcreek/api.ts" || fail "product UI still imports the raw upstream model API"

TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/mindcreek-phase5-static.XXXXXX")
trap 'rm -rf "$TMP_DIR"' EXIT HUP INT TERM
python3 "$ROOT/scripts/render-phase5-models.py" --env-file /dev/null --output "$TMP_DIR/builtin_models.yaml" >/dev/null
[ "$(stat -f '%Lp' "$TMP_DIR/builtin_models.yaml")" = "600" ] || fail "rendered model declaration permissions are not 0600"
rg -q '\$\{MINDCREEK_MANAGED_LLM_API_KEY\}' "$TMP_DIR/builtin_models.yaml" || fail "rendered declaration does not retain secret environment references"
! rg -q 'development-only|mock-embedding' "$TMP_DIR/builtin_models.yaml" || fail "rendered declaration contains a development credential or endpoint"
if MINDCREEK_DEPLOYMENT_ENV=production python3 "$ROOT/scripts/render-phase5-models.py" --env-file /dev/null --output "$TMP_DIR/production.yaml" >/dev/null 2>&1; then
  fail "production rendering accepted missing managed provider settings"
fi
if MINDCREEK_USER_MODEL_OVERRIDES=true python3 "$ROOT/scripts/render-phase5-models.py" --env-file /dev/null --output "$TMP_DIR/overrides.yaml" >/dev/null 2>&1; then
  fail "override rendering accepted a missing SYSTEM_AES_KEY and host allow-list"
fi

for TASK in P5-01 P5-02 P5-03 P5-04 P5-05 P5-06 P5-07; do
  rg -q "\[x\].*$TASK" "$PLAN" || fail "$TASK is not recorded complete"
done
"$ROOT/tools/frontend-overlay/check.sh" >/dev/null
[ -z "$(git -C "$ROOT/upstream/weknora" status --porcelain --untracked-files=all)" ] || fail "upstream submodule is dirty"

echo "MindCreek Phase 5 Gate A managed-model contract verified"
