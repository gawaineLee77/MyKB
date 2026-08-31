#!/bin/sh
set -eu
umask 077

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ENV_FILE="$ROOT/.local/mindcreek.env"
REPORT="$ROOT/.local/phase5-recovery-report.json"
DRILL_ROOT="$ROOT/.local/backups/recovery-drill-$(date -u +%Y%m%dT%H%M%SZ)-$$"
STARTED=$(date +%s)

env_value() {
  awk -F= -v key="$1" '$1 == key { sub(/^[^=]*=/, ""); gsub(/^\047|\047$/, ""); gsub(/^\042|\042$/, ""); print; exit }' "$ENV_FILE"
}
DB_USER=$(env_value DB_USER)
DB_NAME=$(env_value DB_NAME)
RTO=${MINDCREEK_BACKUP_RTO_MINUTES:-$(env_value MINDCREEK_BACKUP_RTO_MINUTES)}
[ -n "$RTO" ] || RTO=30
TEMP_DB="mindcreek_recovery_$(date +%s)_$$"
TEMP_VOLUME="mindcreek-recovery-$$_$(date +%s)"
case "$TEMP_DB:$TEMP_VOLUME" in *[!A-Za-z0-9_.:-]*) echo "generated recovery identifiers are invalid" >&2; exit 1 ;; esac

cleanup() {
  docker exec MindCreek-postgres dropdb -U "$DB_USER" --if-exists "$TEMP_DB" >/dev/null 2>&1 || true
  docker volume rm "$TEMP_VOLUME" >/dev/null 2>&1 || true
}
trap cleanup EXIT HUP INT TERM

"$ROOT/scripts/phase5-backup.sh" --destination "$DRILL_ROOT" >/dev/null
docker exec MindCreek-postgres createdb -U "$DB_USER" --template=template0 --owner="$DB_USER" "$TEMP_DB"
docker exec -i MindCreek-postgres pg_restore --no-owner --no-privileges -U "$DB_USER" -d "$TEMP_DB" < "$DRILL_ROOT/postgres.dump"
SOURCE_MIGRATIONS=$(docker exec MindCreek-postgres psql -U "$DB_USER" -d "$DB_NAME" -Atqc 'SELECT count(*) FROM mindcreek.schema_migrations')
RESTORED_MIGRATIONS=$(docker exec MindCreek-postgres psql -U "$DB_USER" -d "$TEMP_DB" -Atqc 'SELECT count(*) FROM mindcreek.schema_migrations')
[ "$SOURCE_MIGRATIONS" = "$RESTORED_MIGRATIONS" ] || { echo "restored migration count differs from source" >&2; exit 1; }

docker volume create "$TEMP_VOLUME" >/dev/null
docker run --rm --network none -v "$TEMP_VOLUME:/target" -v "$DRILL_ROOT:/backup:ro" python:3.12-alpine \
  sh -c 'tar -xzf /backup/data-files.tar.gz -C /target && find /target -type f | wc -l' > "$DRILL_ROOT/restored-file-count.txt"

FINISHED=$(date +%s)
DURATION=$((FINISHED - STARTED))
MAX_SECONDS=$((RTO * 60))
[ "$DURATION" -le "$MAX_SECONDS" ] || { echo "recovery drill exceeded the configured RTO" >&2; exit 1; }
BACKUP_BYTES=$(du -sk "$DRILL_ROOT" | awk '{print $1 * 1024}')
FILE_COUNT=$(tr -d '[:space:]' < "$DRILL_ROOT/restored-file-count.txt")
python3 - "$REPORT" "$DURATION" "$RTO" "$BACKUP_BYTES" "$FILE_COUNT" "$RESTORED_MIGRATIONS" <<'PY'
import json, sys
path, duration, rto, size, files, migrations = sys.argv[1:]
with open(path, "w", encoding="utf-8") as handle:
    json.dump({
        "status": "pass",
        "restore_seconds": int(duration),
        "rto_minutes": int(rto),
        "backup_bytes": int(size),
        "restored_files": int(files),
        "restored_migrations": int(migrations),
        "live_database_replaced": False,
    }, handle, indent=2)
    handle.write("\n")
PY
chmod 600 "$REPORT"
echo "Phase 5 recovery drill passed in ${DURATION}s without replacing live data"
