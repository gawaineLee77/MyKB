# Docker Image Workspace

This directory owns MindCreek Docker build definitions, image manifests, and
offline-transfer tooling. It does **not** contain application data or uploaded
knowledge files.

## Prepare the Stage 1 images

From the repository root:

```sh
make images-pull   # pull runtime dependencies and UI build bases
make images-build  # build mindcreek-ui:stage1 and mindcreek-gateway:phase3
make images-list   # show whether every Stage 1 runtime image is available
```

`manifests/stage1-external.txt` lists images fetched from registries.
`manifests/stage1-runtime.txt` lists the final images needed by a MindCreek
server. The locally built `mindcreek-ui:stage1` replaces the upstream UI image,
and `mindcreek-gateway:phase3` is the exclusive product API entry point. The offline build also creates `phase2` and `phase1` compatibility aliases for existing local environment files.

For the current product release, prefer the explicit Phase 3 profile. It uses
the same pinned seven-image runtime manifest but writes a versioned archive:

```sh
make phase3-images-pull
make phase3-images-build
make phase3-images-list
make phase3-images-save
```

The native output is `archives/mindcreek-phase3-<architecture>.tar` with
matching `.sha256` and `.manifest.txt` files. The checksum contains a portable
relative filename. Use the Stage 1 commands only when reproducing an older
archive name.

The original Phase 0 profile is also supported:

```sh
./images/manage.sh pull phase0
./images/manage.sh list phase0
```

## Build for an AMD64 server from an ARM64 Mac

Docker Desktop supplies the required CPU emulation. Pull and build every image
for the target platform explicitly:

```sh
make images-pull-amd64
make images-build-amd64
make images-list-amd64
make images-save-amd64
```

The result is `archives/mindcreek-stage1-amd64.tar`. Cross-platform builds are
slower than native builds because the frontend test and Vite build run under
emulation.

For the current Phase 3 bundle use `make phase3-images-pull-amd64`,
`make phase3-images-build-amd64`, `make phase3-images-list-amd64`, and
`make phase3-images-save-amd64`. The output is
`archives/mindcreek-phase3-amd64.tar`.

## Transfer to an offline server

Build on a machine with the same CPU architecture as the target server, then:

```sh
make images-save
./images/manage.sh load images/archives/mindcreek-stage1-<architecture>.tar
```

The save command refuses to overwrite an existing archive and writes a SHA-256
checksum next to it. Files under `archives/` are intentionally ignored by Git:
container archives are large, generated, and platform-specific. Transfer them
through your approved internal artifact store or removable media.

When WeKnora or a base image changes, update the manifests and pinned Dockerfile
together, then run `make stage1-check` and the runtime checks before deployment.
