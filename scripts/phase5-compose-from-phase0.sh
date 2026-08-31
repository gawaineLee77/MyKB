#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
VOLUME_OVERRIDE="$ROOT/deploy/phase4/compose.phase0-volumes.yml"
exec "$ROOT/scripts/phase5-compose.sh" -f "$VOLUME_OVERRIDE" "$@"
