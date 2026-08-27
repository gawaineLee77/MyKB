# Build and LAN Deployment Guide

## 1. Scope and current safety status

The repository provides a verified **Phase 1 development runtime** based on unmodified WeKnora v0.7.2. Gates A–D cover the exclusive Product Gateway path, owner-only Personal Notes, recoverable revisions and quotas, and Plain RAG ingestion/retrieval/citations. It is suitable for development and a controlled LAN pilot, but it is not the final Phase 1 release: optional Note Wiki, excluded-UI cleanup, release packaging, corporate OAuth 2.0, and closed registration remain pending.

The MindCreek frontend applies Stage 1 branding and Stage 2 Personal Notes/Plain RAG modules to a temporary copy of the pinned frontend. The product pages call the gateway; `upstream/weknora` remains unchanged.

The recommended first topology is one internal server running Docker Compose. Only the frontend Nginx port is reachable from the LAN; the app API, PostgreSQL, Redis, and docreader remain private or loopback-only.

## 2. Server requirements

- 64-bit Linux or macOS server, at least 4 CPU cores and 8 GB RAM.
- Docker 20.10+ and Docker Compose v2.24.4 or newer (the overrides use `!override`). Docker Desktop is acceptable on macOS.
- Git and Make. Go 1.26, Node.js 24, Python 3.12, and `uv` are needed only for full source tests, not for image-based deployment.
- A reserved LAN address for the server, for example `192.168.1.50`.
- Enough persistent disk for Docker volumes and uploaded files; 50 GB is a practical pilot starting point.

## 3. Get and verify the source

On the server:

```sh
git clone --recurse-submodules https://github.com/gawaineLee77/MyKB.git
cd MyKB
git submodule update --init --recursive
make phase0-check
make phase1-check
make phase1-compose-config
```

For a complete source validation, install the language toolchains and run `make upstream-test`. This executes the Go, frontend, and MCP tests and may take considerable time. Normal deployment uses prebuilt images and does not require compilation.

To verify the overlay and build the current MindCreek frontend:

```sh
make stage1-check
make stage1-compose-config
make stage1-ui-build
```

The build runs the upstream frontend tests and type-check before creating `mindcreek-ui:stage1`. The assertion-checked overlay intentionally fails when an upstream release changes one of its expected integration anchors; review and update the product-owned adapter instead of editing the submodule.

To build the three application images from the pinned source instead:

```sh
cd upstream/weknora
./scripts/build_frontend_dist.sh
cd ../..
./scripts/phase0-compose.sh build app docreader frontend
```

## 4. Docker images to download

The verified Phase 1 runtime uses these images:

| Image | Purpose | Required for the pilot |
| --- | --- | --- |
| `mindcreek-ui:stage1` | Branded UI plus Stage 2 product modules | Yes; built locally |
| `mindcreek-gateway:phase2` | Product API, private sharing, policy, profiles, notes, and ingestion guard | Yes; built locally |
| `wechatopenai/weknora-app:v0.7.2` | WeKnora Go application | Yes |
| `wechatopenai/weknora-docreader:v0.7.2` | PDF/Office/image parsing | Yes |
| `paradedb/paradedb:v0.22.2-pg17` | PostgreSQL, vector, and keyword retrieval | Yes |
| `redis:7.0-alpine` | task queue and stream state | Yes |
| `python:3.12-alpine` | deterministic local embedding/chat probe sidecar | Test runtime only |

Building the UI also pulls `node:24-alpine` for the build stage and the digest-pinned `nginx:1.30.3-alpine` runtime base. Build the gateway with `make phase1-gateway-build-offline` when Go 1.26 is installed, or use `make phase1-build` for the normal image build.

Download them with the repository's resolved configuration:

```sh
make images-pull
make images-build
make images-list
```

These Stage 1 archive commands use the pinned manifests and Dockerfile under `images/`. They predate the gateway and do not yet form the final Phase 1 offline bundle; P1-25 will add release packaging. Use
`./images/manage.sh pull phase0` when you specifically need the original
upstream Phase 0 UI rather than the branded MindCreek UI.

The mock embedding/chat service verifies the pipeline but is not a useful production model. Configure approved embedding and chat models in the administration UI for actual use. Sandbox, MCP, Neo4j, MinIO, Qdrant, Milvus, SearXNG, and Langfuse images are not needed for Gates A–D.

For an offline server, prepare the images on a machine with the same CPU
architecture and run `make images-save`. Transfer the generated archive and its
checksum from `images/archives/`, then import it with
`./images/manage.sh load <archive.tar>`. Generated archives are ignored by Git.
To create an AMD64 bundle from an ARM64 development Mac, run
`make images-pull-amd64`, `make images-build-amd64`, and
`make images-save-amd64`; Docker Desktop performs the cross-platform build.

## 5. Configure secrets and start locally

The first Phase 1 Compose command creates `.local/mindcreek.env` from the tracked example. Edit that ignored file before sharing the service:

```sh
make phase1-compose-config
openssl rand -hex 24       # use separate values for DB_PASSWORD and REDIS_PASSWORD
openssl rand -base64 48    # use for JWT_SECRET
openssl rand -hex 16       # exactly 32 characters; use for SYSTEM_AES_KEY
```

Replace all sample credentials in `.local/mindcreek.env`. Keep `GIN_MODE=release`, `LLM_DEBUG_LOG=false`, cross-tenant access disabled, and RBAC enabled. Phase 1 preserves upstream registration; corporate OAuth 2.0 and permanently closed registration are scheduled for Phase 5.

Build, start, and verify the loopback-only Phase 1 deployment:

```sh
make stage1-ui-build
make phase1-gateway-build-offline
make phase1-up
make phase1-runtime-check
make phase1-gate-c
make phase1-gate-d
make phase1-ps
```

Visit `http://127.0.0.1:18080` on the server first.

### 5.1 Run only the branded UI baseline

The first Stage 1 command creates `.local/mindcreek.env`. Replace its placeholder secrets before sharing the service, then start the branded distribution:

```sh
make stage1-compose-config
$EDITOR .local/mindcreek.env
make stage1-up
make stage1-runtime-check
make stage1-ps
```

Visit `http://127.0.0.1:18080`. Stop it with `make stage1-down`; named volumes are preserved. The Stage 1 wrapper uses the verified Phase 0 service selection and replaces only the frontend build plus product-facing container names.

## 6. Publish only the web frontend to the LAN

Do not edit tracked Compose files. Create the ignored file `.local/lan.override.yml`:

```yaml
services:
  frontend:
    ports: !override
      - "0.0.0.0:18080:80"
```

Stop the loopback profile and start Phase 1 with the extra LAN override:

```sh
make phase1-down
./scripts/phase1-compose.sh -f .local/lan.override.yml up -d
```

Allow inbound TCP port `18080` only from the local subnet in the server firewall. Do not expose the gateway, WeKnora app, database, Redis, or docreader ports. Also ensure the Wi-Fi/router setting commonly called **AP isolation** or **client isolation** is disabled. From another computer, test:

```sh
curl -I http://192.168.1.50:18080/
```

## 7. Assign a local domain name

First reserve `192.168.1.50` for the server in the router's DHCP settings. Prefer `mindcreek.home.arpa`; `.home.arpa` is intended for private home/LAN naming, while `.local` is reserved for multicast DNS and can cause conflicts.

Best option: add a local DNS `A` record in the router or corporate DNS:

```text
mindcreek.home.arpa  ->  192.168.1.50
```

If the router cannot host local DNS, add this line on each client:

```text
192.168.1.50  mindcreek.home.arpa
```

- Windows: edit `C:\Windows\System32\drivers\etc\hosts` as Administrator.
- macOS/Linux: edit `/etc/hosts` with administrator privileges.

Users can then open `http://mindcreek.home.arpa:18080`. For a permanent company deployment, use corporate DNS plus an internal-CA TLS certificate and expose only HTTPS port 443 through the company reverse proxy. A DNS name alone does not provide encryption.

## 8. Operations

Inspect services with `make phase1-ps` and logs with `./scripts/phase1-compose.sh logs -f gateway app frontend`. Stop containers with `make phase1-down`; named volumes are preserved. Never use `docker compose down -v` unless permanent data deletion is intended. Back up the PostgreSQL and `data-files` volumes before upgrades, and promote a newly pinned WeKnora release only after the compatibility suite passes.
