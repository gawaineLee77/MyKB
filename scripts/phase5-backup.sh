#!/bin/sh
set -eu
umask 077

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ENV_FILE="$ROOT/.local/mindcreek.env"
COMPOSE="$ROOT/scripts/phase5-compose.sh"

env_value() {
  awk -F= -v key="$1" '$1 == key { sub(/^[^=]*=/, ""); gsub(/^\047|\047$/, ""); gsub(/^\042|\042$/, ""); print; exit }' "$ENV_FILE"
}

[ -f "$ENV_FILE" ] || { echo "runtime environment is missing: $ENV_FILE" >&2; exit 2; }
BACKUP_ROOT=${MINDCREEK_BACKUP_ROOT:-$(env_value MINDCREEK_BACKUP_ROOT)}
[ -n "$BACKUP_ROOT" ] || BACKUP_ROOT="$ROOT/.local/backups"
case "$BACKUP_ROOT" in /*) ;; *) BACKUP_ROOT="$ROOT/$BACKUP_ROOT" ;; esac

DESTINATION=""
if [ "${1:-}" = "--destination" ] && [ -n "${2:-}" ]; then
  DESTINATION=$2
  case "$DESTINATION" in /*) ;; *) DESTINATION="$ROOT/$DESTINATION" ;; esac
elif [ "$#" -ne 0 ]; then
  echo "usage: $0 [--destination <new-directory>]" >&2
  exit 2
fi

DB_USER=$(env_value DB_USER)
DB_NAME=$(env_value DB_NAME)
case "$DB_USER:$DB_NAME" in *[!A-Za-z0-9_:]*) echo "database identifiers contain unsupported characters" >&2; exit 2 ;; esac

mkdir -p "$BACKUP_ROOT"
if [ -n "$DESTINATION" ]; then
  [ ! -e "$DESTINATION" ] || { echo "backup destination already exists: $DESTINATION" >&2; exit 2; }
  mkdir -p "$DESTINATION"
  STAGING=$DESTINATION
else
  STAGING=$(mktemp -d "$BACKUP_ROOT/.phase5-$(date -u +%Y%m%dT%H%M%SZ).XXXXXX")
fi

RUNNING=$($COMPOSE ps --status running --services)
resume() {
  if [ -n "$RUNNING" ]; then
    # Word splitting is intentional: Compose service names contain no spaces.
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

docker exec MindCreek-postgres pg_dump --format=custom --compress=6 --no-owner --no-privileges -U "$DB_USER" "$DB_NAME" > "$STAGING/postgres.dump"
docker exec MindCreek-postgres psql -U "$DB_USER" -d "$DB_NAME" -Atqc \
  "SELECT version || ' ' || name FROM mindcreek.schema_migrations ORDER BY version" > "$STAGING/schema-migrations.txt"

DATA_VOLUME=$(docker inspect MindCreek-app --format '{{range .Mounts}}{{if eq .Destination "/data/files"}}{{.Name}}{{end}}{{end}}')
case "$DATA_VOLUME" in ""|*[!A-Za-z0-9_.-]*) echo "unable to resolve the application data volume" >&2; exit 1 ;; esac
docker run --rm --network none -v "$DATA_VOLUME:/source:ro" -v "$STAGING:/backup" python:3.12-alpine \
  sh -c 'cd /source && tar -czf /backup/data-files.tar.gz .'

tar -czf "$STAGING/product-config.tar.gz" -C "$ROOT" \
  config deploy/phase5 deploy/mindcreek/.env.example images/manifests docs/UPSTREAM_PATCHES.md
sed -n 's/^\([A-Za-z_][A-Za-z0-9_]*\)=.*/\1/p' "$ENV_FILE" | sort -u > "$STAGING/runtime-env.keys"

{
  printf 'created_at_utc=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  printf 'product_version=%s\n' "$(env_value MINDCREEK_VERSION)"
  printf 'repository_commit=%s\n' "$(git -C "$ROOT" rev-parse HEAD)"
  printf 'upstream_commit=%s\n' "$(git -C "$ROOT/upstream/weknora" rev-parse HEAD)"
  printf 'data_volume=%s\n' "$DATA_VOLUME"
  printf 'secret_material=excluded\n'
} > "$STAGING/metadata.txt"

(
  cd "$STAGING"
  shasum -a 256 postgres.dump data-files.tar.gz product-config.tar.gz schema-migrations.txt runtime-env.keys metadata.txt > CHECKSUMS.sha256
)
chmod -R go-rwx "$STAGING"
trap - EXIT HUP INT TERM
resume
echo "Phase 5 consistent backup verified; secret material excluded"
echo "$STAGING"
