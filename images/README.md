# Docker Image Workspace

This directory owns MindCreek Docker build definitions, image manifests, and
offline-transfer tooling. It does **not** contain application data or uploaded
knowledge files.

## Prepare the current Phase 4 images

From the repository root:

```sh
make phase4-images-pull   # pull runtime dependencies and UI build bases
make phase4-images-build  # build mindcreek-ui:phase4 and mindcreek-gateway:phase4
make phase4-images-list   # verify all seven native-platform images
make phase4-images-save   # create the native offline archive
```

`manifests/phase4-runtime.txt` lists the final images needed by a MindCreek
server. The product-owned UI replaces the upstream UI image, and the gateway is
the exclusive product API/MCP entry point. Compatibility aliases are created
for older ignored environment files, but new deployments should use Phase 4 tags.

The native output is `archives/mindcreek-phase4-<architecture>.tar` with
matching `.sha256` and `.manifest.txt` files. The checksum contains a portable
relative filename. The original Phase 0 and historical Stage 1/Phase 3 profiles
remain available only for reproduction and rollback.

The original Phase 0 profile is also supported:

```sh
./images/manage.sh pull phase0
./images/manage.sh list phase0
```

## Build for an AMD64 server from an ARM64 Mac

Docker Desktop supplies the required CPU emulation. Pull and build every image
for the target platform explicitly:

```sh
make phase4-images-pull-amd64
make phase4-images-build-amd64
make phase4-images-list-amd64
make phase4-images-save-amd64
```

The result is `archives/mindcreek-phase4-amd64.tar`. Cross-platform builds are
slower than native builds because the frontend test and Vite build run under
emulation.

## Transfer to an offline server

Build on a machine with the same CPU architecture as the target server, then:

```sh
make phase4-images-save
./images/manage.sh load images/archives/mindcreek-phase4-<architecture>.tar
```

The save command refuses to overwrite an existing archive and writes a SHA-256
checksum next to it. Files under `archives/` are intentionally ignored by Git:
container archives are large, generated, and platform-specific. Transfer them
through your approved internal artifact store or removable media.

When WeKnora or a base image changes, update the manifests and pinned Dockerfile
together, then run `make phase4-check`, the live Gate B/C probes, and the runtime checks before deployment.
