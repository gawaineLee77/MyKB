#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
RUNTIME_DIR="$ROOT/.local"
RUNTIME_ENV="$RUNTIME_DIR/mindcreek.env"
ENV_TEMPLATE="$ROOT/deploy/mindcreek/.env.example"
BASE_COMPOSE="$ROOT/upstream/weknora/docker-compose.yml"
PHASE0_OVERRIDE="$ROOT/deploy/phase0/compose.override.yml"
MINDCREEK_OVERRIDE="$ROOT/deploy/mindcreek/compose.ui.yml"

if [ ! -f "$RUNTIME_ENV" ]; then
  mkdir -p "$RUNTIME_DIR"
  cp "$ENV_TEMPLATE" "$RUNTIME_ENV"
  echo "Created local runtime configuration: $RUNTIME_ENV"
fi

if [ "$#" -eq 0 ]; then
  echo "usage: $0 <docker compose arguments>" >&2
  exit 2
fi

exec docker compose \
  --project-name mindcreek-stage1 \
  --env-file "$RUNTIME_ENV" \
  -f "$BASE_COMPOSE" \
  -f "$PHASE0_OVERRIDE" \
  -f "$MINDCREEK_OVERRIDE" \
  "$@"
