#!/bin/sh
set -eu
umask 077

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ENV_FILE="$ROOT/.local/mindcreek.env"
COMPOSE="$ROOT/scripts/phase5-compose.sh"

if [ "$#" -ne 2 ] || [ "$1" != "--confirm-replace-current-data" ]; then
  echo "usage: $0 --confirm-replace-current-data <backup-directory>" >&2
  exit 2
fi
BACKUP=$2
case "$BACKUP" in /*) ;; *) BACKUP="$ROOT/$BACKUP" ;; esac
[ -d "$BACKUP" ] || { echo "backup directory not found: $BACKUP" >&2; exit 2; }
for file in CHECKSUMS.sha256 postgres.dump data-files.tar.gz metadata.txt; do
  [ -f "$BACKUP/$file" ] || { echo "incomplete backup: missing $file" >&2; exit 2; }
done
(cd "$BACKUP" && shasum -a 256 -c CHECKSUMS.sha256)

env_value() {
  awk -F= -v key="$1" '$1 == key { sub(/^[^=]*=/, ""); gsub(/^\047|\047$/, ""); gsub(/^\042|\042$/, ""); print; exit }' "$ENV_FILE"
}
DB_USER=$(env_value DB_USER)
DB_NAME=$(env_value DB_NAME)
case "$DB_USER:$DB_NAME" in *[!A-Za-z0-9_:]*) echo "database identifiers contain unsupported characters" >&2; exit 2 ;; esac

RUNNING=$($COMPOSE ps --status running --services)
resume() {
  if [ -n "$RUNNING" ]; then
    # shellcheck disable=SC2086
    $COMPOSE up -d $RUNNING >/dev/null 2>&1 || true
  fi
}
trap resume EXIT HUP INT TERM
for service in frontend gateway app docreader; do
  if printf '%s\n' "$RUNNING" | grep -qx "$service"; then
    $COMPOSE stop -t 30 "$service" >/dev/null
  fi
done

case "$DB_NAME" in postgres|template0|template1) echo "refusing to replace a PostgreSQL system database" >&2; exit 2 ;; esac
docker exec MindCreek-postgres dropdb --force -U "$DB_USER" "$DB_NAME"
docker exec MindCreek-postgres createdb -U "$DB_USER" --template=template0 --owner="$DB_USER" "$DB_NAME"
docker exec -i MindCreek-postgres pg_restore --no-owner --no-privileges -U "$DB_USER" -d "$DB_NAME" < "$BACKUP/postgres.dump"
DATA_VOLUME=$(docker inspect MindCreek-app --format '{{range .Mounts}}{{if eq .Destination "/data/files"}}{{.Name}}{{end}}{{end}}')
case "$DATA_VOLUME" in ""|*[!A-Za-z0-9_.-]*) echo "unable to resolve the application data volume" >&2; exit 1 ;; esac
docker run --rm --network none -v "$DATA_VOLUME:/target" -v "$BACKUP:/backup:ro" python:3.12-alpine \
  sh -c 'find /target -mindepth 1 -delete && tar -xzf /backup/data-files.tar.gz -C /target'

trap - EXIT HUP INT TERM
resume
$COMPOSE run --rm --no-deps gateway migrate up >/dev/null
echo "Phase 5 restore completed; run make phase5-runtime-check and the Gate A/B probes"
