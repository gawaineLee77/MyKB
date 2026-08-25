# Build and LAN Deployment Guide

## 1. Scope and current safety status

The repository currently provides the verified **Phase 0 baseline** based on unmodified WeKnora v0.7.2. It is suitable for development and a trusted-LAN pilot. It is **not yet the final production service**: the Phase 0 probe proved that users in the same upstream workspace can discover and retrieve another member's knowledge. Do not store sensitive Personal Notes until the Phase 1 Product Gateway enforces owner-only access.

The recommended first topology is one internal server running Docker Compose. Only the frontend Nginx port is reachable from the LAN; the app API, PostgreSQL, Redis, and docreader remain private or loopback-only.

## 2. Server requirements

- 64-bit Linux or macOS server, at least 4 CPU cores and 8 GB RAM.
- Docker 20.10+ and Docker Compose v2.24.4 or newer (the overrides use `!override`). Docker Desktop is acceptable on macOS.
- Git and Make. Go 1.26, Node.js 24, Python 3.12, and `uv` are needed only for full source tests, not for image-based deployment.
- A reserved LAN address for the server, for example `192.168.1.50`.
- Enough persistent disk for Docker volumes and uploaded files; 50 GB is a practical pilot starting point.

## 3. Get and verify the source

The current Phase 0 commit is local until it is pushed to GitHub. From the development computer, publish it when ready:

```sh
git push -u origin main
```

Then, on the server:

```sh
git clone --recurse-submodules https://github.com/gawaineLee77/MyKB.git
cd MyKB
git submodule update --init --recursive
make phase0-check
make phase0-compose-config
```

For a complete source validation, install the language toolchains and run `make upstream-test`. This executes the Go, frontend, and MCP tests and may take considerable time. Normal deployment uses prebuilt images and does not require compilation.

To build the three application images from the pinned source instead:

```sh
cd upstream/weknora
./scripts/build_frontend_dist.sh
cd ../..
./scripts/phase0-compose.sh build app docreader frontend
```

## 4. Docker images to download

The default Phase 0 profile uses exactly these images:

| Image | Purpose | Required for the pilot |
| --- | --- | --- |
| `wechatopenai/weknora-ui:v0.7.2` | Web UI and Nginx API proxy | Yes |
| `wechatopenai/weknora-app:v0.7.2` | WeKnora Go application | Yes |
| `wechatopenai/weknora-docreader:v0.7.2` | PDF/Office/image parsing | Yes |
| `paradedb/paradedb:v0.22.2-pg17` | PostgreSQL, vector, and keyword retrieval | Yes |
| `redis:7.0-alpine` | task queue and stream state | Yes |
| `python:3.12-alpine` | deterministic Phase 0 mock embedding service | Phase 0 only |

Download them with the repository's resolved configuration:

```sh
./scripts/phase0-compose.sh pull
./scripts/phase0-compose.sh config --images
```

The mock embedding service verifies the pipeline but is not a useful semantic model. Configure a real embedding model and chat model in the WeKnora administration UI for actual use. Sandbox, MCP, Neo4j, MinIO, Qdrant, Milvus, SearXNG, and Langfuse images are optional profiles and are not needed for Phase 0.

For an offline server, pull the six images on a machine with the same CPU architecture, export them with `docker save`, transfer the tar file, and import it on the server with `docker load`.

## 5. Configure secrets and start locally

The first Compose command creates `.local/phase0.env` from the tracked example. Edit that ignored file before sharing the service:

```sh
make phase0-compose-config
openssl rand -hex 24       # use separate values for DB_PASSWORD and REDIS_PASSWORD
openssl rand -base64 48    # use for JWT_SECRET
openssl rand -hex 16       # exactly 32 characters; use for SYSTEM_AES_KEY
```

Replace all `phase0_*` credentials in `.local/phase0.env`. Keep `GIN_MODE=release`, `LLM_DEBUG_LOG=false`, cross-tenant access disabled, and RBAC enabled. Registration may remain enabled only while bootstrapping the first users; then set `DISABLE_REGISTRATION=true` and recreate the app container.

Start and verify the loopback-only deployment:

```sh
make phase0-up
make phase0-runtime-check
make phase0-probe
make phase0-ps
```

Visit `http://127.0.0.1:18080` on the server first.

## 6. Publish only the web frontend to the LAN

Do not edit the tracked Phase 0 override. Create the ignored file `.local/lan.override.yml`:

```yaml
services:
  frontend:
    ports: !override
      - "0.0.0.0:18080:80"
  app:
    ports: !override
      - "127.0.0.1:18081:8080"
```

Stop the loopback profile and start with the extra LAN override:

```sh
make phase0-down
docker compose --project-name mykb-phase0 \
  --env-file .local/phase0.env \
  -f upstream/weknora/docker-compose.yml \
  -f deploy/phase0/compose.override.yml \
  -f .local/lan.override.yml up -d
```

Allow inbound TCP port `18080` only from the local subnet in the server firewall. Do not expose ports `18081`, `5432`, `6379`, or `50051`. Also ensure the Wi-Fi/router setting commonly called **AP isolation** or **client isolation** is disabled. From another computer, test:

```sh
curl -I http://192.168.1.50:18080/
```

## 7. Assign a local domain name

First reserve `192.168.1.50` for the server in the router's DHCP settings. Prefer `mykb.home.arpa`; `.home.arpa` is intended for private home/LAN naming, while `.local` is reserved for multicast DNS and can cause conflicts.

Best option: add a local DNS `A` record in the router or corporate DNS:

```text
mykb.home.arpa  ->  192.168.1.50
```

If the router cannot host local DNS, add this line on each client:

```text
192.168.1.50  mykb.home.arpa
```

- Windows: edit `C:\Windows\System32\drivers\etc\hosts` as Administrator.
- macOS/Linux: edit `/etc/hosts` with administrator privileges.

Users can then open `http://mykb.home.arpa:18080`. For a permanent company deployment, use corporate DNS plus an internal-CA TLS certificate and expose only HTTPS port 443 through the company reverse proxy. A DNS name alone does not provide encryption.

## 8. Operations

Inspect services with `make phase0-ps` and logs with `./scripts/phase0-compose.sh logs -f app frontend`. Stop containers with `make phase0-down`; named volumes are preserved. Never use `docker compose down -v` unless permanent data deletion is intended. Back up the PostgreSQL and `data-files` volumes before upgrades, and promote a newly pinned WeKnora release only after the compatibility suite passes.
