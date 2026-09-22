#!/bin/sh
# Reuse the verified installation state machine with the independent R3 project.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
: "${MINDCREEK_R3_SECRET_DIR:?provide the dedicated R3 installation secret directory}"
export MINDCREEK_R2_SECRET_DIR="$MINDCREEK_R3_SECRET_DIR"
export MINDCREEK_INSTALL_PROFILE=r3
exec "$ROOT/scripts/r2-install.sh" "$@"
