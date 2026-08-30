# Phase 4 Operations and Upgrade Guide

## Release contents

MindCreek `0.5.0-phase4` keeps WeKnora v0.7.2 unmodified and adds the authorized Web Ask workspace and hosted read-only MCP facade. The runtime uses seven images listed in `images/manifests/phase4-runtime.txt`; no Neo4j, SearXNG, upstream MCP server, CLI, IM, or Mini Program image is required.

## Build and start

```sh
git submodule update --init --recursive
make phase4-check
make phase4-images-pull
make phase4-images-build
make phase4-compose-config
make phase4-up
make phase4-ps
```

Edit `.local/mindcreek.env` before shared use. Preserve the existing database, Redis, JWT, storage, and especially `SYSTEM_AES_KEY` values. Existing ignored environments must select:

```dotenv
MINDCREEK_UI_IMAGE=mindcreek-ui:phase4
MINDCREEK_UI_VERSION=0.5.0
MINDCREEK_GATEWAY_IMAGE=mindcreek-gateway:phase4
MINDCREEK_VERSION=0.5.0-phase4
```

Only publish the frontend port. To retain a LAN override:

```sh
./scripts/phase4-compose.sh -f .local/lan.override.yml up -d
```

## Upgrade from Phase 0 or Phase 3

Back up PostgreSQL and uploaded files, stop the old stack without `-v`, update the source and submodule, load/build Phase 4 images, then reuse the stopped Phase 0 volumes:

```sh
make phase0-down                 # or stop the Phase 3 stack
make phase4-upgrade-compose-config
make phase4-upgrade-up
make phase4-upgrade-ps
make phase1-migration-probe
make phase4-gate-b
make phase4-gate-c
```

Migration 9 only adds `mindcreek.agent_operation_audit_events`. It does not change WeKnora's `public` schema. Never start two stacks against the same volumes and never run `docker compose down -v` during an upgrade.

## Web agent

Open `/platform/mindcreek/ask`. Default scope includes owned, explicitly shared, and actively subscribed KBs. Organization-public KBs require explicit selection. Grant revocation, unsubscribe, audience loss, and unpublish take effect on the next request and when a citation is opened.

Quick Answer uses the model bound to each selected KB. To enable Smart Reasoning, an administrator must configure both a KnowledgeQA model and a Rerank model on WeKnora's built-in `builtin-smart-reasoning` agent. The Ask workspace keeps that mode disabled until both assignments are present.

## MCP clients

Use `POST https://mindcreek.example/mcp` with the same Bearer token or approved API key used for MindCreek. The current protocol requires `MCP-Protocol-Version: 2026-07-28`, `Mcp-Method`, request `_meta`, and `Mcp-Name` for tool calls. MCP is read-only and rate-limited. Put TLS and the organization's authentication proxy in front of the frontend during Phase 5; do not expose the gateway container directly.

## Rollback

Stop Phase 4 and restart the prior application images against the preserved volumes. Older releases ignore the isolated migration-9 table. Retain backups through the observation period; schema rollback is unnecessary for normal application rollback.
