#!/bin/sh
# Explicit local installation operations; never called by normal gateway startup.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
COMPOSE="$ROOT/scripts/r2-compose.sh"
case "${MINDCREEK_INSTALL_PROFILE:-r2}" in
  r2) ;;
  r3) COMPOSE="$ROOT/scripts/r3-compose.sh" ;;
  r4) COMPOSE="$ROOT/scripts/r4-compose.sh" ;;
  *) echo "unknown installation profile" >&2; exit 2 ;;
esac
operation=${1:-status}
if [ "$#" -gt 0 ]; then shift; fi
run_install() {
  "$COMPOSE" run --rm --no-deps \
    -v "$MINDCREEK_R2_SECRET_DIR:/run/mindcreek-enterprise:rw" gateway install "$@"
}
case "$operation" in
  status|default-space|repair-member-key) run_install "$operation" "$@" ;;
  admin)
    "$COMPOSE" up -d postgres redis mock-embedding app gateway
    status=$(run_install status)
    stage=$(printf '%s' "$status" | python3 -c 'import json,sys; print(json.load(sys.stdin)["stage"])')
    case "$stage" in account_ready|space_creating|space_ready|key_creating|ready) printf '%s\n' "$status"; exit 0 ;; esac
    close_window() { MINDCREEK_R2_REGISTRATION_DISABLED=true MINDCREEK_R2_BOOTSTRAP_EMAIL= "$COMPOSE" up -d --no-deps --force-recreate app; }
    trap close_window EXIT
    if [ "$stage" = new ] || [ "$stage" = registering ]; then
      MINDCREEK_R2_REGISTRATION_DISABLED=false "$COMPOSE" up -d --no-deps --force-recreate app
      "$COMPOSE" exec -T app sh -c 'for i in $(seq 1 60); do wget -q -O /dev/null http://127.0.0.1:8080/health && exit 0; sleep 1; done; exit 1'
      status=$(run_install prepare "$@")
    fi
    email=$(printf '%s' "$status" | python3 -c 'import json,sys; print(json.load(sys.stdin)["admin_email"])')
    MINDCREEK_R2_REGISTRATION_DISABLED=true MINDCREEK_R2_BOOTSTRAP_EMAIL="$email" "$COMPOSE" up -d --no-deps --force-recreate app
    "$COMPOSE" exec -T app sh -c 'for i in $(seq 1 60); do wget -q -O /dev/null http://127.0.0.1:8080/health && exit 0; sleep 1; done; exit 1'
    run_install confirm-admin
    ;;
  *) echo "usage: r2-install.sh [status|admin [--admin-id ID]|default-space [--default-space-id ID] [--member-key-id ID]|repair-member-key --member-key-id ID]" >&2; exit 2 ;;
esac
