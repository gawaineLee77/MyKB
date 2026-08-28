# Phase 0 to Phase 3 Upgrade Guide

## 1. Upgrade contract

This procedure upgrades the repository's pinned WeKnora v0.7.2 Phase 0 deployment to MindCreek `0.4.0-phase3`. It preserves WeKnora PostgreSQL data and uploaded files, then adds the Product Gateway, MindCreek UI, Personal Notes, Plain RAG guards, Viewer/Editor sharing, publication, Discover, subscriptions, organization-public access, and revision badges.

Phase 3 product migrations create only the `mindcreek` PostgreSQL schema. They do not rewrite WeKnora's `public` schema. Never run Phase 0 and Phase 3 simultaneously against the same volumes, and never use `docker compose down -v` during an upgrade.

## 2. Prerequisites

- Confirm Phase 0 uses this repository's WeKnora v0.7.2 baseline.
- Obtain the Phase 3 repository source as well as the matching offline image archive. The image archive does not contain Compose files or configuration.
- Select the archive matching the server: `amd64` for ordinary Intel/AMD Linux servers or `aarch64` for ARM64.
- Reserve a maintenance window and enough disk for database and file backups.

Verify the running baseline:

```sh
make phase0-runtime-check
./scripts/phase0-compose.sh ps
docker volume inspect mykb-phase0_postgres-data mykb-phase0_data-files
```

If your Compose project was not named `mykb-phase0`, determine its volume names with `docker volume ls` and use the override variables described below.

## 3. Back up Phase 0

Create a protected backup directory. The environment backup contains secrets.

```sh
mkdir -p backups/phase0-before-phase3
chmod 700 backups/phase0-before-phase3
cp -p .local/phase0.env backups/phase0-before-phase3/phase0.env
./scripts/phase0-compose.sh exec -T postgres sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > backups/phase0-before-phase3/database.dump
make phase0-down
docker run --rm -v mykb-phase0_data-files:/source:ro -v "$PWD/backups/phase0-before-phase3":/backup python:3.12-alpine sh -c 'cd /source && tar -czf /backup/data-files.tar.gz .'
docker run --rm -v mykb-phase0_postgres-data:/source:ro -v "$PWD/backups/phase0-before-phase3":/backup python:3.12-alpine sh -c 'cd /source && tar -czf /backup/postgres-volume.tar.gz .'
```

Keep Phase 0 stopped after the backup. Confirm the backup files are non-empty and copy them to approved storage before proceeding.

## 4. Load the Phase 3 images

Copy the `.tar`, `.sha256`, and `.manifest.txt` files from `images/archives/` to the server. From their directory:

```sh
sha256sum -c mindcreek-phase3-amd64.tar.sha256
docker load --input mindcreek-phase3-amd64.tar
```

On macOS use `shasum -a 256 -c` instead. The manifest must list exactly these seven images:

```text
mindcreek-ui:stage1
mindcreek-gateway:phase3
wechatopenai/weknora-app:v0.7.2
wechatopenai/weknora-docreader:v0.7.2
paradedb/paradedb:v0.22.2-pg17
redis:7.0-alpine
python:3.12-alpine
```

## 5. Merge the runtime configuration

In the Phase 3 repository checkout, create the new ignored environment file:

```sh
make phase1-compose-config
chmod 600 .local/mindcreek.env
```

Edit `.local/mindcreek.env`. Copy the existing Phase 0 values for `DB_USER`, `DB_PASSWORD`, `DB_NAME`, Redis settings, storage settings, `JWT_SECRET`, and especially `SYSTEM_AES_KEY`. Losing or changing `SYSTEM_AES_KEY` makes stored model credentials unreadable. Keep `DB_HOST=postgres` and the Phase 3 values below:

```text
MINDCREEK_UI_IMAGE=mindcreek-ui:stage1
MINDCREEK_GATEWAY_IMAGE=mindcreek-gateway:phase3
MINDCREEK_VERSION=0.4.0-phase3
WEKNORA_VERSION=v0.7.2
```

Do not replace secrets with the sample values. Registration remains enabled until the planned Phase 5 OAuth 2.0 migration.

The default upgrade expects the original volume names. For another Phase 0 project name, add the actual names to `.local/mindcreek.env`:

```text
MINDCREEK_PHASE0_POSTGRES_VOLUME=your-project_postgres-data
MINDCREEK_PHASE0_FILES_VOLUME=your-project_data-files
MINDCREEK_PHASE0_DOCREADER_VOLUME=your-project_docreader-tmp
```

## 6. Start and verify Phase 3

Validate the resolved topology before starting:

```sh
make phase3-upgrade-compose-config
make phase3-upgrade-up
make phase3-upgrade-ps
make phase1-runtime-check
```

Confirm that all eight product migrations were applied:

```sh
./scripts/phase3-compose-from-phase0.sh exec -T postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atqc "SELECT count(*) FROM mindcreek.schema_migrations"'
```

The result must be `8`. Log in through `http://SERVER:18080`, open a known Phase 0 KB, preview a known document, and run a retrieval query before accepting the upgrade. Then verify Personal Notes, sharing, publication, Discover, subscription, update badges, unpublish, and organization-public read access with synthetic data. Do not run destructive or synthetic acceptance probes against production user data without a staging copy.

## 7. Roll back

For an application-level rollback, stop Phase 3 and restart Phase 0:

```sh
make phase3-upgrade-down
make phase0-up
make phase0-runtime-check
```

This works because both deployments use WeKnora v0.7.2 and Phase 0 ignores the isolated `mindcreek` schema. Do not delete that schema during ordinary rollback. Retain the logical and stopped-volume backups until Phase 3 has completed the organization's observation period. Restoring a physical PostgreSQL volume is an emergency operation and must be done only with both stacks stopped and the target volume explicitly verified.

## 8. Normal Phase 3 operations after upgrade

Continue using the upgrade-aware wrapper so the deployment always mounts the original persistent volumes:

```sh
make phase3-upgrade-up
make phase3-upgrade-ps
./scripts/phase3-compose-from-phase0.sh logs -f gateway app frontend
make phase3-upgrade-down
```

Back up PostgreSQL and `data-files` before every future image or WeKnora baseline upgrade.
