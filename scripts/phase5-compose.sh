#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
RUNTIME_DIR="$ROOT/.local"
RUNTIME_ENV="$RUNTIME_DIR/mindcreek.env"
ENV_TEMPLATE="$ROOT/deploy/mindcreek/.env.example"
MODEL_CONFIG="$RUNTIME_DIR/phase5/builtin_models.yaml"
BASE_COMPOSE="$ROOT/upstream/weknora/docker-compose.yml"
PHASE0_OVERRIDE="$ROOT/deploy/phase0/compose.override.yml"
MINDCREEK_OVERRIDE="$ROOT/deploy/mindcreek/compose.ui.yml"
GATEWAY_OVERRIDE="$ROOT/deploy/phase1/compose.gateway.yml"
PHASE5_OVERRIDE="$ROOT/deploy/phase5/compose.managed-models.yml"

if [ ! -f "$RUNTIME_ENV" ]; then
  mkdir -p "$RUNTIME_DIR"
  cp "$ENV_TEMPLATE" "$RUNTIME_ENV"
  chmod 600 "$RUNTIME_ENV"
  echo "Created local runtime configuration: $RUNTIME_ENV"
fi

python3 "$ROOT/scripts/render-phase5-models.py" --env-file "$RUNTIME_ENV" --output "$MODEL_CONFIG" >&2

if [ "$#" -eq 0 ]; then
  echo "usage: $0 <docker compose arguments>" >&2
  exit 2
fi

exec docker compose \
  --project-name mindcreek-phase5 \
  --env-file "$RUNTIME_ENV" \
  -f "$BASE_COMPOSE" \
  -f "$PHASE0_OVERRIDE" \
  -f "$MINDCREEK_OVERRIDE" \
  -f "$GATEWAY_OVERRIDE" \
  -f "$PHASE5_OVERRIDE" \
  "$@"
