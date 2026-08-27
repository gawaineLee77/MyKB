.PHONY: help images-pull images-build images-list images-save images-pull-amd64 images-build-amd64 images-list-amd64 images-save-amd64 phase0-check phase0-compose-config phase0-up phase0-ps phase0-runtime-check phase0-probe phase0-down phase1-route-policy-check phase1-gateway-test phase1-check phase1-compose-config phase1-build phase1-gateway-build-offline phase1-up phase1-ps phase1-probe phase1-runtime-check phase1-migration-probe phase1-gate-b-probe phase1-gate-b phase1-gate-c-probe phase1-gate-c phase1-gate-d-probe phase1-gate-d phase1-down phase2-sharing-model-check phase2-route-actions-check phase2-gate-a-static-check phase2-gate-b-static-check phase2-gate-c-static-check phase2-gate-d-static-check phase2-upstream-contract-check phase2-clean-copy-check phase2-check phase2-gate-a phase2-gate-b-probe phase2-gate-b phase2-gate-c phase2-gate-d stage1-check stage1-compose-config stage1-ui-build stage1-up stage1-ps stage1-runtime-check stage1-down upstream-status upstream-test upstream-test-go upstream-test-frontend upstream-test-mcp

help:
	@echo "MindCreek repository commands"
	@echo "  make images-pull           Pull Stage 1 runtime and UI build dependencies"
	@echo "  make images-build          Build the local mindcreek-ui:stage1 image"
	@echo "  make images-list           Check final Stage 1 runtime image availability"
	@echo "  make images-save           Export Stage 1 runtime images for offline transfer"
	@echo "  make images-pull-amd64     Pull AMD64 runtime and UI build dependencies"
	@echo "  make images-build-amd64    Cross-build the AMD64 MindCreek UI image"
	@echo "  make images-list-amd64     Check AMD64 runtime image availability"
	@echo "  make images-save-amd64     Export the AMD64 runtime bundle"
	@echo "  make phase0-check          Verify design and immutable upstream boundary"
	@echo "  make phase0-compose-config Validate the isolated Phase 0 Compose profile"
	@echo "  make phase0-up             Start the isolated Phase 0 runtime"
	@echo "  make phase0-ps             Show Phase 0 container status"
	@echo "  make phase0-runtime-check  Verify services, health, version, and revision"
	@echo "  make phase0-probe          Run synthetic authorization/retrieval probes"
	@echo "  make phase0-down           Stop Phase 0 containers (preserve volumes)"
	@echo "  make phase1-route-policy-check Verify the complete Phase 1 route classification"
	@echo "  make phase1-gateway-test    Run gateway unit and WeKnora contract tests"
	@echo "  make phase1-check           Run Phase 1 static, boundary, and gateway checks"
	@echo "  make phase1-compose-config  Validate the gateway-only Phase 1 topology"
	@echo "  make phase1-build           Build the MindCreek UI and gateway images"
	@echo "  make phase1-gateway-build-offline Build the native gateway image without registry bases"
	@echo "  make phase1-up              Start the seven-service Phase 1 runtime"
	@echo "  make phase1-probe           Probe capabilities and disabled routes through the UI"
	@echo "  make phase1-runtime-check   Verify private upstream and the gateway policy"
	@echo "  make phase1-migration-probe Verify empty/repeat/rollback/forward migrations"
	@echo "  make phase1-gate-b-probe    Run the two-user negative authorization matrix"
	@echo "  make phase1-gate-b          Run every live Gate B acceptance check"
	@echo "  make phase1-gate-c          Run Personal Notes CRUD, quota, and recovery acceptance"
	@echo "  make phase1-gate-d          Run Plain RAG ingestion, retrieval, chat, and citation acceptance"
	@echo "  make phase1-down            Stop Phase 1 containers (preserve volumes)"
	@echo "  make phase2-sharing-model-check Verify the Phase 2 upstream sharing-model map"
	@echo "  make phase2-route-actions-check Verify every Phase 2 KB route action"
	@echo "  make phase2-check           Run inherited Phase 1 checks and current Phase 2 checks"
	@echo "  make phase2-gate-a          Run Gate A static, unit, and migration lifecycle checks"
	@echo "  make phase2-gate-b          Run private-sharing, role, revocation, and audit acceptance"
	@echo "  make phase2-gate-c          Verify and build the authorized sharing UI"
	@echo "  make phase2-upstream-contract-check Verify the pinned or candidate WeKnora seams"
	@echo "  make phase2-clean-copy-check Reconstruct and validate a clean Phase 2 checkout"
	@echo "  make phase2-gate-d          Run the Phase 2 release and clean-copy contract"
	@echo "  make stage1-check          Verify the MindCreek overlay and upstream boundary"
	@echo "  make stage1-compose-config Validate the Stage 1 Compose distribution"
	@echo "  make stage1-ui-build       Build the branded MindCreek UI image"
	@echo "  make stage1-up             Start Stage 1 (build the UI first when needed)"
	@echo "  make stage1-ps             Show Stage 1 container status"
	@echo "  make stage1-runtime-check  Verify the branded UI and service health"
	@echo "  make stage1-down           Stop Stage 1 containers (preserve volumes)"
	@echo "  make upstream-status       Show the pinned WeKnora identity"
	@echo "  make upstream-test         Run Go, frontend, and MCP upstream suites"
	@echo "  make upstream-test-go      Run the upstream Go suite"
	@echo "  make upstream-test-frontend Run upstream frontend tests/type-check/build"
	@echo "  make upstream-test-mcp     Run upstream MCP unit tests"

images-pull:
	./images/manage.sh pull stage1

images-build:
	./images/manage.sh build stage1

images-list:
	./images/manage.sh list stage1

images-save:
	./images/manage.sh save stage1

images-pull-amd64:
	./images/manage.sh pull stage1 linux/amd64

images-build-amd64:
	./images/manage.sh build stage1 linux/amd64

images-list-amd64:
	./images/manage.sh list stage1 linux/amd64

images-save-amd64:
	./images/manage.sh save stage1 linux/amd64

phase0-check:
	./scripts/verify-upstream.sh
	./scripts/check-design-docs.sh

phase0-compose-config:
	./scripts/phase0-compose.sh config --quiet

phase0-up:
	./scripts/phase0-compose.sh up -d

phase0-ps:
	./scripts/phase0-compose.sh ps

phase0-runtime-check:
	./scripts/phase0-runtime-check.sh

phase0-probe:
	python3 ./scripts/phase0-probe.py

phase0-down:
	./scripts/phase0-compose.sh down

phase1-route-policy-check:
	./scripts/check-phase1-route-policy.sh

phase1-gateway-test:
	mkdir -p .local/gateway-go-build
	cd services/gateway && GOCACHE=$(CURDIR)/.local/gateway-go-build go test ./...

phase1-check: phase0-check phase1-route-policy-check phase1-gateway-test

phase1-compose-config:
	./scripts/phase1-compose.sh config --quiet

phase1-build:
	./scripts/phase1-compose.sh build frontend gateway

phase1-gateway-build-offline:
	./scripts/build-gateway-image-offline.sh

phase1-up:
	./scripts/phase1-compose.sh up -d

phase1-ps:
	./scripts/phase1-compose.sh ps

phase1-probe:
	python3 ./scripts/phase1-policy-probe.py

phase1-runtime-check:
	./scripts/phase1-runtime-check.sh

phase1-migration-probe:
	python3 ./scripts/phase1-migration-probe.py

phase1-gate-b-probe:
	python3 ./scripts/phase1-gate-b-probe.py

phase1-gate-c-probe:
	python3 ./scripts/phase1-gate-c-probe.py

phase1-gate-c:
	./scripts/phase1-runtime-check.sh
	python3 ./scripts/phase1-gate-c-probe.py

phase1-gate-d-probe:
	python3 ./scripts/phase1-gate-d-probe.py

phase1-gate-d:
	./scripts/phase1-runtime-check.sh
	python3 ./scripts/phase1-gate-d-probe.py

phase1-gate-b:
	./scripts/phase1-runtime-check.sh
	python3 ./scripts/phase1-migration-probe.py
	python3 ./scripts/phase1-gate-b-probe.py

phase1-down:
	./scripts/phase1-compose.sh down

phase2-sharing-model-check:
	./scripts/check-phase2-sharing-model.sh

phase2-route-actions-check:
	./scripts/check-phase2-route-actions.sh

phase2-gate-a-static-check:
	./scripts/check-phase2-gate-a.sh

phase2-gate-b-static-check:
	./scripts/check-phase2-gate-b.sh

phase2-gate-c-static-check:
	./scripts/check-phase2-gate-c.sh

phase2-gate-d-static-check:
	./scripts/check-phase2-gate-d.sh

phase2-upstream-contract-check:
	./scripts/check-phase2-upstream-contract.sh

phase2-clean-copy-check:
	./scripts/phase2-clean-copy-check.sh

phase2-check: phase1-check phase2-sharing-model-check phase2-route-actions-check phase2-gate-a-static-check phase2-gate-b-static-check phase2-gate-c-static-check phase2-gate-d-static-check phase2-upstream-contract-check

phase2-gate-a: phase2-check
	python3 ./scripts/phase1-migration-probe.py

phase2-gate-b-probe:
	python3 ./scripts/phase2-gate-b-probe.py

phase2-gate-b:
	./scripts/phase1-runtime-check.sh
	python3 ./scripts/phase1-migration-probe.py
	python3 ./scripts/phase2-gate-b-probe.py

phase2-gate-c: phase2-gate-c-static-check
	./scripts/mindcreek-compose.sh build frontend

phase2-gate-d: phase2-check phase2-clean-copy-check

stage1-check: phase0-check
	./tools/frontend-overlay/check.sh

stage1-compose-config:
	./scripts/mindcreek-compose.sh config --quiet

stage1-ui-build:
	./scripts/mindcreek-compose.sh build frontend

stage1-up:
	./scripts/mindcreek-compose.sh up -d

stage1-ps:
	./scripts/mindcreek-compose.sh ps

stage1-runtime-check:
	./scripts/stage1-runtime-check.sh

stage1-down:
	./scripts/mindcreek-compose.sh down

upstream-status:
	@git submodule status upstream/weknora
	@git -C upstream/weknora describe --tags --exact-match
	@git -C upstream/weknora status --short --branch

upstream-test: upstream-test-go upstream-test-frontend upstream-test-mcp

upstream-test-go:
	./scripts/test-upstream-go.sh

upstream-test-frontend:
	cd upstream/weknora/frontend && npm ci --no-audit --no-fund && npm test && npm run type-check && npm run build

upstream-test-mcp:
	cd upstream/weknora/mcp-server && uv sync --python 3.12 --extra test && uv run --python 3.12 python -m unittest discover -s . -p "test_*.py" -v
