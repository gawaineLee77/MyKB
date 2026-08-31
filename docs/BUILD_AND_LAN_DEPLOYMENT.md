# Build and LAN Deployment Guide

## 1. Scope and current safety status

The repository provides a verified **Phase 5 Gate B development runtime** based on unmodified WeKnora v0.7.2. Inherited gates cover the exclusive Product Gateway path, Personal Notes, Plain RAG, sharing/subscriptions, Authorized Ask, hosted read-only MCP, and immediate revocation. Gate A adds server-managed chat, embedding, and rerank defaults; Gate B adds corporate OAuth/OIDC, first-login provisioning, closed registration, and suspension enforcement. Production provider activation and operational hardening remain Phase 5 Gates C–D work.

The MindCreek frontend applies branding and product modules—including Personal Notes, Plain RAG, sharing, Discover, Subscribed, and Authorized Ask—to a temporary copy of the pinned frontend. The product pages and `/mcp` call the gateway; `upstream/weknora` remains unchanged.

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
make phase2-check
make phase3-check
make phase4-check
make phase4-compose-config
make phase5-gate-a-static-check
make phase5-gate-b-static-check
make phase5-compose-config
```

For a complete source validation, install the language toolchains and run `make upstream-test`. This executes the Go, frontend, and MCP tests and may take considerable time. Normal deployment uses prebuilt images and does not require compilation.

To verify the overlay and build the current MindCreek frontend and gateway without contacting a registry:

```sh
make phase4-gate-b-static-check
make phase5-build-offline
```

The build runs the frontend tests and type-check before creating `mindcreek-ui:phase5` and `mindcreek-gateway:phase5`. The assertion-checked overlay intentionally fails when an upstream release changes an expected integration anchor; review and update the product-owned adapter instead of editing the submodule.

## 4. Docker images to download

The verified Phase 5 Gate B runtime uses these images:

| Image | Purpose | Required for the pilot |
| --- | --- | --- |
| `mindcreek-ui:phase5` | Branded UI, redacted managed-model status, Advanced Settings, and `/mcp` proxy | Yes; built locally |
| `mindcreek-gateway:phase5` | Product API, authorization, model facade/defaults, sharing, Web Ask, and hosted MCP | Yes; built locally |
| `wechatopenai/weknora-app:v0.7.2` | WeKnora Go application | Yes |
| `wechatopenai/weknora-docreader:v0.7.2` | PDF/Office/image parsing | Yes |
| `paradedb/paradedb:v0.22.2-pg17` | PostgreSQL, vector, and keyword retrieval | Yes |
| `redis:7.0-alpine` | task queue and stream state | Yes |
| `python:3.12-alpine` | deterministic local embedding/chat/rerank probe sidecar | Test runtime only |

The online build uses `node:24-alpine`, `golang:1.26-alpine`, `alpine:3.23`, and the digest-pinned `nginx:1.30.3-alpine`. `make phase5-build-offline` instead uses installed pinned frontend dependencies, the host Go 1.26 toolchain, a scratch gateway runtime, and the local Phase 4 UI runtime base.

Build Phase 5 after the base runtime images have been downloaded or loaded:

```sh
make phase4-images-pull
make phase5-build-offline
docker image inspect mindcreek-ui:phase5 mindcreek-gateway:phase5 >/dev/null
```

These Phase 4 commands use the pinned manifests and Dockerfiles under `images/`. Use
`./images/manage.sh pull phase0` when you specifically need the original
upstream Phase 0 UI rather than the branded MindCreek UI.

The mock embedding/chat/rerank service verifies the pipeline but is not a production model. Phase 5 production values belong only in `.local/mindcreek.env`; the committed YAML contains environment references. Smart Reasoning is bound to the stable managed KnowledgeQA and Rerank IDs. The upstream MCP-server, sandbox, Neo4j, MinIO, Qdrant, Milvus, SearXNG, and Langfuse images are not needed for Gate A.

For an offline server, prepare the images on a machine with the same CPU
architecture and run `make phase4-images-save`. Transfer the generated archive and its
checksum from `images/archives/`, then import it with
`./images/manage.sh load <archive.tar>`. Generated archives are ignored by Git.
To create an AMD64 bundle from an ARM64 development Mac, run
`make phase4-images-pull-amd64`, `make phase4-images-build-amd64`, and
`make phase4-images-save-amd64`; Docker Desktop performs the cross-platform build.

## 5. Configure secrets and start locally

The first Phase 5 Compose command creates `.local/mindcreek.env` from the tracked example. Edit that ignored file before sharing the service:

```sh
make phase5-compose-config
openssl rand -hex 24       # use separate values for DB_PASSWORD and REDIS_PASSWORD
openssl rand -base64 48    # use for JWT_SECRET
openssl rand -hex 16       # exactly 32 characters; use for SYSTEM_AES_KEY
```

Replace all sample credentials and set every `MINDCREEK_MANAGED_*` model value in `.local/mindcreek.env`. The embedding dimension must match the provider. Staging/production rendering rejects absent, mock, non-HTTPS, and obvious placeholder values. Keep `MINDCREEK_USER_MODEL_OVERRIDES=false` unless the opt-in capability file, a new 32-byte `SYSTEM_AES_KEY`, exact host/provider allow-lists, and organization egress approval are all in place. Configure the corporate issuer, exact HTTPS origin, client credentials, callback, and optional group policy using [PHASE5_IDENTITY_PROVIDER.md](PHASE5_IDENTITY_PROVIDER.md).

Build, start, and verify the loopback-only Phase 5 deployment:

```sh
make phase5-build-offline
make phase5-up
make phase5-runtime-check
make phase5-gate-a
make phase5-gate-b
# After real corporate provider activation:
make phase5-gate-b-probe
make phase5-ps
```

Visit `http://127.0.0.1:18080` on the server first.

## 6. Publish only the web frontend to the LAN

Do not edit tracked Compose files. Create the ignored file `.local/lan.override.yml`:

```yaml
services:
  frontend:
    ports: !override
      - "0.0.0.0:18080:80"
```

Stop the loopback profile and start Phase 5 with the extra LAN override:

```sh
make phase5-down
./scripts/phase5-compose.sh -f .local/lan.override.yml up -d
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

Inspect services with `make phase5-ps` and logs with `./scripts/phase5-compose.sh logs -f gateway app frontend`. Stop containers with `make phase5-down`; named volumes are preserved. Never use `docker compose down -v` unless permanent data deletion is intended. Back up PostgreSQL and `data-files` before upgrades, follow [PHASE4_OPERATIONS.md](PHASE4_OPERATIONS.md) for inherited procedures, and promote a newly pinned WeKnora release only after the compatibility suite passes. Gate A evidence is in [PHASE5_GATE_A.md](PHASE5_GATE_A.md).

When replacing a disposable server deployment with a deliberately empty Phase 5 installation, use [PHASE5_FRESH_SERVER_INSTALL.md](PHASE5_FRESH_SERVER_INSTALL.md). Its reset helper requires an explicit destructive confirmation and does not affect Docker images or release archives.
