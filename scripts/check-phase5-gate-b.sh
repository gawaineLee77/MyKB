#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PLAN="$ROOT/docs/PHASE5_IMPLEMENTATION_PLAN.md"

fail() {
  echo "Phase 5 Gate B check failed: $*" >&2
  exit 1
}

for FILE in \
  "$ROOT/deploy/phase5/compose.identity.yml" \
  "$ROOT/services/gateway/internal/database/migrations/000010_phase5_corporate_identity.up.sql" \
  "$ROOT/services/gateway/internal/identity/broker.go" \
  "$ROOT/services/gateway/internal/identity/provider.go" \
  "$ROOT/services/gateway/internal/identity/oauth2_provider.go" \
  "$ROOT/services/gateway/internal/identity/gate.go" \
  "$ROOT/services/gateway/internal/identity/admin.go" \
  "$ROOT/services/gateway/internal/identity/broker_test.go" \
  "$ROOT/services/gateway/internal/identity/provider_test.go" \
  "$ROOT/tools/frontend-overlay/product/mindcreek/SSOLogin.vue" \
  "$ROOT/tools/frontend-overlay/product/mindcreek/AuthEntry.vue" \
  "$ROOT/scripts/phase5-gate-b-probe.py" \
  "$ROOT/docs/PHASE5_IDENTITY_PROVIDER.md" \
  "$ROOT/docs/PHASE5_GATE_B.md"; do
  [ -f "$FILE" ] || fail "missing ${FILE#$ROOT/}"
done

rg -q 'code_challenge_method.*S256' "$ROOT/services/gateway/internal/identity/oauth2_provider.go" || fail "corporate OAuth2 PKCE S256 is missing"
rg -q 'Authorization.*Bearer' "$ROOT/services/gateway/internal/identity/oauth2_provider.go" || fail "corporate OAuth2 bearer UserInfo is missing"
rg -q 'writeAuthorizationPost' "$ROOT/services/gateway/internal/identity/broker.go" || fail "corporate form-POST authorization is missing"
rg -Fq 'query.Set("response_type", "code")' "$ROOT/services/gateway/internal/identity/oauth2_provider.go" || fail "corporate response_type=code is missing"
rg -q 'application/json' "$ROOT/services/gateway/internal/identity/oauth2_provider.go" || fail "corporate JSON token exchange is missing"
rg -q 'transactionCookies' "$ROOT/services/gateway/internal/identity/broker.go" || fail "cookie-bound no-state callback is missing"
rg -Fq 'query.Set("access_token"' "$ROOT/services/gateway/internal/identity/oauth2_provider.go" || fail "corporate query-token UserInfo is missing"
for CLAIM in employeeType globalUserID tenantId uid uuid; do
  rg -q "$CLAIM" "$ROOT/deploy/mindcreek/.env.example" || fail "$CLAIM OAuth2 mapping is missing"
done
rg -q 'rsa.VerifyPKCS1v15' "$ROOT/services/gateway/internal/identity/provider.go" || fail "ID-token signature verification is missing"
for CLAIM in issuer audience nonce subject; do
  rg -qi "$CLAIM" "$ROOT/services/gateway/internal/identity/provider.go" || fail "$CLAIM validation is missing"
done
rg -q 'issuer, subject' "$ROOT/services/gateway/internal/database/migrations/000010_phase5_corporate_identity.up.sql" || fail "stable issuer/subject mapping is missing"
rg -q 'identity.closed_registration' "$ROOT/services/gateway/internal/server/server.go" || fail "closed registration enforcement is missing"
rg -q 'ErrSuspended' "$ROOT/services/gateway/internal/identity/gate.go" || fail "suspended-session enforcement is missing"
rg -q 'Sign in with your organization' "$ROOT/tools/frontend-overlay/product/mindcreek/SSOLogin.vue" || fail "SSO-only UI is missing"
rg -q 'mindcreekOAuthRootCallback' "$ROOT/tools/frontend-overlay/apply.mjs" || fail "corporate root callback bridge is missing"
rg -q 'MINDCREEK_IDENTITY_CLIENT_SECRET' "$ROOT/deploy/phase5/compose.identity.yml" || fail "provider secret injection is missing"
rg -q 'MINDCREEK_IDENTITY_PROTOCOL.*oauth2' "$ROOT/deploy/mindcreek/.env.example" || fail "plain OAuth2 deployment mode is missing"
! rg -q 'MINDCREEK_IDENTITY_CLIENT_SECRET=[^[:space:]]+' "$ROOT/deploy/mindcreek/.env.example" || fail "identity client secret has a committed value"

for TASK in P5-08 P5-09 P5-10 P5-11 P5-12 P5-13; do
  rg -q "\[x\].*$TASK" "$PLAN" || fail "$TASK is not recorded complete"
done

"$ROOT/tools/frontend-overlay/check.sh" >/dev/null
[ -z "$(git -C "$ROOT/upstream/weknora" status --porcelain --untracked-files=all)" ] || fail "upstream submodule is dirty"

echo "MindCreek Phase 5 Gate B corporate identity contract verified"
