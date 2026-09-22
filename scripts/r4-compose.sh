#!/bin/sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
: "${MINDCREEK_R4_ENV_FILE:?provide a dedicated R4 environment file}"
: "${MINDCREEK_R4_SECRET_DIR:?provide a dedicated R4 installation secret directory}"
case "$MINDCREEK_R4_ENV_FILE:$MINDCREEK_R4_SECRET_DIR" in /*:/*) ;; *) echo "R4 paths must be absolute" >&2; exit 2 ;; esac
[ -f "$MINDCREEK_R4_ENV_FILE" ] && [ -d "$MINDCREEK_R4_SECRET_DIR" ] || exit 2
export MINDCREEK_R2_ENV_FILE="$MINDCREEK_R4_ENV_FILE"
export MINDCREEK_R2_SECRET_DIR="$MINDCREEK_R4_SECRET_DIR"
export MINDCREEK_R2_MODEL_FILE="$ROOT/.local/redesign-r4/runtime/builtin_models.yaml"
mkdir -p "$(dirname "$MINDCREEK_R2_MODEL_FILE")"
python3 "$ROOT/scripts/render-phase5-models.py" --env-file "$MINDCREEK_R4_ENV_FILE" --output "$MINDCREEK_R2_MODEL_FILE" >&2
exec docker compose --project-name mindcreek-native-r4 --env-file "$MINDCREEK_R4_ENV_FILE" \
  -f "$ROOT/upstream/weknora/docker-compose.yml" \
  -f "$ROOT/deploy/phase0/compose.override.yml" \
  -f "$ROOT/deploy/mindcreek/compose.ui.yml" \
  -f "$ROOT/deploy/phase1/compose.gateway.yml" \
  -f "$ROOT/deploy/phase5/compose.managed-models.yml" \
  -f "$ROOT/deploy/phase5/compose.identity.yml" \
  -f "$ROOT/deploy/r2/compose.enterprise.yml" \
  -f "$ROOT/deploy/r3/compose.native.yml" \
  -f "$ROOT/deploy/r4/compose.ui.yml" "$@"
