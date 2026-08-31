#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ENV_FILE="$ROOT/.local/mindcreek.env"
PRODUCTION_OVERRIDE="$ROOT/deploy/phase5/compose.production.yml"

[ -f "$ENV_FILE" ] || {
  echo "production configuration is missing: $ENV_FILE" >&2
  exit 2
}
python3 "$ROOT/scripts/phase5-secret-check.py" --env-file "$ENV_FILE"

exec "$ROOT/scripts/phase5-compose.sh" -f "$PRODUCTION_OVERRIDE" "$@"
