# Phase 5 Fresh Server Installation

This procedure replaces every known MyKB/MindCreek deployment on a dedicated server with a new MindCreek `0.6.0-phase5` database and file store. It is not an upgrade: accounts, notes, knowledge bases, uploads, indexes, publications, subscriptions, sessions, and audit history are permanently removed.

## 1. Prepare the release

Use a current clean checkout and copy the three AMD64 release artifacts into `images/archives/`:

```sh
git pull --ff-only
git submodule update --init --recursive
cd images/archives
sha256sum -c mindcreek-phase5-amd64.tar.sha256
cd ../..
./images/manage.sh load images/archives/mindcreek-phase5-amd64.tar linux/amd64
make phase5-images-list-amd64
```

The archive contains all seven runtime images. It does not contain configuration, credentials, certificates, or user data.

## 2. Inventory and erase old deployments

Save any configuration values you still need in the approved secret manager. Do not retain a data backup when the goal is a genuinely empty installation.

```sh
./scripts/phase5-server-reset.sh --list
./scripts/phase5-server-reset.sh --confirm-destroy-all-mindcreek-data
./scripts/phase5-server-reset.sh --list
```

The reset targets only Compose projects named `mykb-phase0`, `mindcreek-stage1`, `mindcreek-phase1`, `mindcreek-phase3`, `mindcreek-phase4`, `mindcreek-phase5`, and `weknora`. It removes their containers, attached anonymous volumes, named volumes, and networks. Images, archives, repository files, and `.local` configuration are preserved. Untraceable dangling anonymous volumes are reported but not deleted because they may belong to another workload.

## 3. Create fresh Phase 5 configuration

Generate a new protected file from the current template and edit it locally on the server:

```sh
mkdir -p .local
install -m 600 deploy/mindcreek/.env.example .local/mindcreek.env.phase5-new
${EDITOR:-vi} .local/mindcreek.env.phase5-new
mv .local/mindcreek.env.phase5-new .local/mindcreek.env
make phase5-compose-config
```

Replace every placeholder. Set independent database, Redis, JWT, and 32-character AES secrets; configure the managed chat, embedding, and rerank providers; and use the provider's exact embedding dimension. For production, also configure HTTPS URLs, TLS files, corporate OIDC, allowed groups, and the exact callback documented in [PHASE5_IDENTITY_PROVIDER.md](PHASE5_IDENTITY_PROVIDER.md). Never paste secrets into Git or deployment logs.

## 4. Start the service

For the same LAN profile used by Stage 1, retain or recreate `.local/lan.override.yml`, then run:

```sh
./scripts/phase5-compose.sh -f .local/lan.override.yml config --quiet
./scripts/phase5-compose.sh -f .local/lan.override.yml up -d
make phase5-runtime-check
make phase5-observability-probe
make phase5-ps
```

The runtime and observability checks read service state and write only local reports. Do not run `phase5-gate-a-probe` or `phase5-pilot-probe` on the real service: they create persistent synthetic users, knowledge bases, documents, and sessions. `phase5-migration-probe` creates and removes an isolated temporary database and is intended for staging/release verification rather than routine production startup.

For production TLS and corporate SSO, use the hardened profile instead:

```sh
python3 scripts/phase5-secret-check.py --env-file .local/mindcreek.env
make phase5-production-compose-config
./scripts/phase5-production-compose.sh up -d
make phase5-runtime-check
make phase5-gate-b-probe
```

Only the frontend should be reachable from the LAN. Confirm that PostgreSQL, Redis, gateway, application, document reader, and model sidecar do not publish host ports. Complete the corporate browser-login exercise before admitting internal users.
