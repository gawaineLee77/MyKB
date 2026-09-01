#!/bin/sh
set -eu
umask 077

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TEMP=$(mktemp -d "${TMPDIR:-/tmp}/mindcreek-phase5-production.XXXXXX")
CREATED_RUNTIME_ENV=false
cleanup() {
  rm -rf "$TEMP"
  if [ "$CREATED_RUNTIME_ENV" = true ]; then rm -f "$ROOT/.local/mindcreek.env"; fi
}
trap cleanup EXIT HUP INT TERM
CERT="$TEMP/tls.crt"
KEY="$TEMP/tls.key"
ENV_FILE="$TEMP/production.env"
CONFIG="$TEMP/compose.json"
: > "$CERT"
: > "$KEY"
chmod 600 "$KEY"

cat > "$ENV_FILE" <<EOF
MINDCREEK_DEPLOYMENT_ENV=production
WEKNORA_VERSION=v0.7.2
DB_USER=mindcreek
DB_PASSWORD=synthetic-db-credential-000001
DB_NAME=mindcreek
REDIS_PASSWORD=synthetic-redis-credential-0002
JWT_SECRET=synthetic-jwt-credential-00000003
SYSTEM_AES_KEY=0123456789abcdef0123456789ABCDEF
MINDCREEK_MANAGED_LLM_NAME=pilot-chat
MINDCREEK_MANAGED_LLM_BASE_URL=https://llm.example.invalid/v1
MINDCREEK_MANAGED_LLM_API_KEY=synthetic-llm-credential-00000004
MINDCREEK_MANAGED_LLM_PROVIDER=generic
MINDCREEK_MANAGED_EMBEDDING_NAME=pilot-embedding
MINDCREEK_MANAGED_EMBEDDING_BASE_URL=https://embedding.example.invalid/v1
MINDCREEK_MANAGED_EMBEDDING_API_KEY=synthetic-embedding-credential-0005
MINDCREEK_MANAGED_EMBEDDING_PROVIDER=generic
MINDCREEK_MANAGED_EMBEDDING_DIMENSION=1024
MINDCREEK_MANAGED_RERANK_NAME=pilot-rerank
MINDCREEK_MANAGED_RERANK_BASE_URL=https://rerank.example.invalid/v1
MINDCREEK_MANAGED_RERANK_API_KEY=synthetic-rerank-credential-000006
MINDCREEK_MANAGED_RERANK_PROVIDER=generic
MINDCREEK_IDENTITY_ENABLED=true
MINDCREEK_IDENTITY_PROTOCOL=oauth2
MINDCREEK_EXTERNAL_ORIGIN=https://mindcreek.example.invalid
MINDCREEK_IDENTITY_ISSUER=https://identity.example.invalid
MINDCREEK_IDENTITY_AUTHORIZATION_URL=https://identity.example.invalid/authorize
MINDCREEK_IDENTITY_TOKEN_URL=https://identity.example.invalid/accesstoken
MINDCREEK_IDENTITY_USERINFO_URL=https://identity.example.invalid/userinfo
MINDCREEK_IDENTITY_REFRESH_URL=https://identity.example.invalid/refreshtoken
MINDCREEK_IDENTITY_CLIENT_ID=synthetic-client
MINDCREEK_IDENTITY_CLIENT_SECRET=synthetic-identity-credential-0007
MINDCREEK_IDENTITY_AUTHORIZATION_METHOD=POST
MINDCREEK_IDENTITY_AUTHORIZATION_GRANT_TYPE=authorization_code
MINDCREEK_IDENTITY_PKCE_ENABLED=false
MINDCREEK_IDENTITY_STATE_REQUIRED=false
MINDCREEK_IDENTITY_USERINFO_TOKEN_TRANSPORT=query
MINDCREEK_IDENTITY_SCOPES=base.profile
MINDCREEK_BROKER_CLIENT_SECRET=synthetic-broker-credential-00000008
MINDCREEK_TLS_CERT_FILE=$CERT
MINDCREEK_TLS_KEY_FILE=$KEY
EOF
chmod 600 "$ENV_FILE"
if [ ! -f "$ROOT/.local/mindcreek.env" ]; then
  mkdir -p "$ROOT/.local"
  cp "$ENV_FILE" "$ROOT/.local/mindcreek.env"
  chmod 600 "$ROOT/.local/mindcreek.env"
  CREATED_RUNTIME_ENV=true
fi
python3 "$ROOT/scripts/phase5-secret-check.py" --env-file "$ENV_FILE" >/dev/null

docker compose --project-name mindcreek-phase5-production-check --env-file "$ENV_FILE" \
  -f "$ROOT/upstream/weknora/docker-compose.yml" \
  -f "$ROOT/deploy/phase0/compose.override.yml" \
  -f "$ROOT/deploy/mindcreek/compose.ui.yml" \
  -f "$ROOT/deploy/phase1/compose.gateway.yml" \
  -f "$ROOT/deploy/phase5/compose.managed-models.yml" \
  -f "$ROOT/deploy/phase5/compose.identity.yml" \
  -f "$ROOT/deploy/phase5/compose.production.yml" \
  config --format json > "$CONFIG"
python3 - "$CONFIG" <<'PY'
import json, sys
with open(sys.argv[1], encoding="utf-8") as handle:
    config = json.load(handle)
services = config["services"]
assert services["frontend"].get("ports"), "frontend TLS ports missing"
assert not services["gateway"].get("ports"), "gateway port published"
assert not services["app"].get("ports"), "upstream application port published"
published = {str(item["published"]) for item in services["frontend"]["ports"]}
assert published == {"80", "443"}, published
assert config["networks"]["WeKnora-network"].get("internal") is True
assert "mindcreek-egress" in services["gateway"]["networks"]
assert "mindcreek-egress" in services["app"]["networks"]
identity = services["gateway"]["environment"]
assert identity["MINDCREEK_IDENTITY_PROTOCOL"] == "oauth2"
assert identity["MINDCREEK_IDENTITY_AUTHORIZATION_METHOD"] == "POST"
assert identity["MINDCREEK_IDENTITY_USERINFO_TOKEN_TRANSPORT"] == "query"
assert identity["MINDCREEK_IDENTITY_AUTHORIZATION_URL"].endswith("/authorize")
assert identity["MINDCREEK_IDENTITY_TOKEN_URL"].endswith("/accesstoken")
assert identity["MINDCREEK_IDENTITY_USERINFO_URL"].endswith("/userinfo")
PY
echo "MindCreek Phase 5 production profile verified: TLS edge only, private dependencies, controlled egress"
