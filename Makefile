.PHONY: help images-pull images-build images-list images-save images-pull-amd64 images-build-amd64 images-list-amd64 images-save-amd64 phase3-images-pull phase3-images-build phase3-images-list phase3-images-save phase3-images-pull-amd64 phase3-images-build-amd64 phase3-images-list-amd64 phase3-images-save-amd64 phase3-upgrade-compose-config phase3-upgrade-up phase3-upgrade-ps phase3-upgrade-down phase0-check phase0-compose-config phase0-up phase0-ps phase0-runtime-check phase0-probe phase0-down phase1-route-policy-check phase1-gateway-test phase1-check phase1-compose-config phase1-build phase1-gateway-build-offline phase1-up phase1-ps phase1-probe phase1-runtime-check phase1-migration-probe phase1-gate-b-probe phase1-gate-b phase1-gate-c-probe phase1-gate-c phase1-gate-d-probe phase1-gate-d phase1-down phase2-sharing-model-check phase2-route-actions-check phase2-gate-a-static-check phase2-gate-b-static-check phase2-gate-c-static-check phase2-gate-d-static-check phase2-upstream-contract-check phase2-clean-copy-check phase2-check phase2-gate-a phase2-gate-b-probe phase2-gate-b phase2-gate-c phase2-gate-d phase3-gate-a-static-check phase3-gate-b-static-check phase3-gate-c-static-check phase3-gate-d-static-check phase3-upstream-contract-check phase3-clean-copy-check phase3-check phase3-gate-a phase3-gate-b-probe phase3-gate-b phase3-gate-c phase3-gate-d stage1-check stage1-compose-config stage1-ui-build stage1-up stage1-ps stage1-runtime-check stage1-down upstream-status upstream-test upstream-test-go upstream-test-frontend upstream-test-mcp
.PHONY: phase4-images-pull phase4-images-build phase4-images-list phase4-images-save phase4-images-pull-amd64 phase4-images-build-amd64 phase4-images-list-amd64 phase4-images-save-amd64 phase4-compose-config phase4-build phase4-up phase4-ps phase4-down phase4-upgrade-compose-config phase4-upgrade-up phase4-upgrade-ps phase4-upgrade-down phase4-gate-a-static-check phase4-gate-b-static-check phase4-gate-c-static-check phase4-gate-d-static-check phase4-upstream-contract-check phase4-clean-copy-check phase4-check phase4-gate-a phase4-gate-b-probe phase4-gate-b phase4-gate-c-probe phase4-gate-c phase4-gate-d
.PHONY: phase5-images-pull phase5-images-build phase5-images-list phase5-images-save phase5-images-pull-amd64 phase5-images-build-amd64 phase5-images-list-amd64 phase5-images-save-amd64
.PHONY: phase5-models-render phase5-compose-config phase5-production-compose-config phase5-build phase5-build-offline phase5-up phase5-ps phase5-down phase5-runtime-check phase5-gate-a-static-check phase5-gate-a-probe phase5-gate-a phase5-gate-b-static-check phase5-gate-b-probe phase5-gate-b phase5-backup phase5-recovery-drill phase5-observability-probe phase5-load-probe phase5-migration-probe phase5-failure-recovery-probe phase5-security-scan phase5-gate-c-static-check phase5-gate-c phase5-pilot-probe phase5-upstream-contract-check phase5-clean-copy-check phase5-gate-d-static-check phase5-check phase5-gate-d phase5-upgrade-compose-config phase5-upgrade-up phase5-upgrade-ps phase5-upgrade-down

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
	@echo "  make phase3-images-build   Build current native Phase 3 product images"
	@echo "  make phase3-images-save    Export the native Phase 3 offline bundle"
	@echo "  make phase3-images-build-amd64 Cross-build Phase 3 product images for AMD64"
	@echo "  make phase3-images-save-amd64 Export the AMD64 Phase 3 offline bundle"
	@echo "  make phase3-upgrade-compose-config Validate Phase 0 volume reuse for Phase 3"
	@echo "  make phase3-upgrade-up       Start Phase 3 against stopped Phase 0 volumes"
	@echo "  make phase3-upgrade-ps       Show the upgraded Phase 3 services"
	@echo "  make phase3-upgrade-down     Stop Phase 3 and preserve reused volumes"
	@echo "  make phase4-images-build     Build current native Phase 4 product images"
	@echo "  make phase4-images-save      Export the native Phase 4 offline bundle"
	@echo "  make phase4-images-build-amd64 Cross-build Phase 4 product images for AMD64"
	@echo "  make phase4-images-save-amd64 Export the AMD64 Phase 4 offline bundle"
	@echo "  make phase5-images-build     Build current native Phase 5 product images"
	@echo "  make phase5-images-save      Export the native Phase 5 offline bundle"
	@echo "  make phase5-images-build-amd64 Cross-build Phase 5 product images for AMD64"
	@echo "  make phase5-images-save-amd64 Export the AMD64 Phase 5 offline bundle"
	@echo "  make phase4-up               Start the fresh Phase 4 runtime"
	@echo "  make phase4-upgrade-up       Start Phase 4 against stopped Phase 0 volumes"
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
	@echo "  make phase3-check           Run inherited checks and all Phase 3 static/unit contracts"
	@echo "  make phase3-gate-a          Run publication-domain and eight-migration acceptance"
	@echo "  make phase3-gate-b          Run publication, catalog, subscription, and revocation acceptance"
	@echo "  make phase3-gate-c          Verify and build the Phase 3 product UI"
	@echo "  make phase3-upstream-contract-check Verify Phase 3 against pinned/candidate WeKnora"
	@echo "  make phase3-clean-copy-check Reconstruct and validate a clean Phase 3 checkout"
	@echo "  make phase3-gate-d          Run the Phase 3 release and clean-copy contract"
	@echo "  make phase4-check           Run inherited and all Phase 4 static/unit contracts"
	@echo "  make phase4-gate-a          Run scope, audit, and nine-migration acceptance"
	@echo "  make phase4-gate-b          Run Web agent, revocation, and retrieval baseline acceptance"
	@echo "  make phase4-gate-c          Run hosted MCP protocol and security acceptance"
	@echo "  make phase4-gate-d          Run the Phase 4 release and clean-copy contract"
	@echo "  make phase5-models-render   Validate and render secret-free managed model declarations"
	@echo "  make phase5-up              Start Phase 5 with managed model defaults"
	@echo "  make phase5-build-offline   Build Phase 5 product images from local dependencies"
	@echo "  make phase5-runtime-check   Verify the private managed-model runtime"
	@echo "  make phase5-gate-a          Run zero-key model onboarding acceptance"
	@echo "  make phase5-gate-b          Run corporate SSO and closed-registration acceptance"
	@echo "  make phase5-gate-c          Run operational hardening and recovery acceptance"
	@echo "  make phase5-gate-d          Run pilot, compatibility, and clean-copy release acceptance"
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

phase3-images-pull:
	./images/manage.sh pull phase3

phase3-images-build:
	./images/manage.sh build phase3

phase3-images-list:
	./images/manage.sh list phase3

phase3-images-save:
	./images/manage.sh save phase3

phase3-images-pull-amd64:
	./images/manage.sh pull phase3 linux/amd64

phase3-images-build-amd64:
	./images/manage.sh build phase3 linux/amd64

phase3-images-list-amd64:
	./images/manage.sh list phase3 linux/amd64

phase3-images-save-amd64:
	./images/manage.sh save phase3 linux/amd64

phase4-images-pull:
	./images/manage.sh pull phase4

phase4-images-build:
	./images/manage.sh build phase4

phase4-images-list:
	./images/manage.sh list phase4

phase4-images-save:
	./images/manage.sh save phase4

phase4-images-pull-amd64:
	./images/manage.sh pull phase4 linux/amd64

phase4-images-build-amd64:
	./images/manage.sh build phase4 linux/amd64

phase4-images-list-amd64:
	./images/manage.sh list phase4 linux/amd64

phase4-images-save-amd64:
	./images/manage.sh save phase4 linux/amd64

phase5-images-pull:
	./images/manage.sh pull phase5

phase5-images-build:
	./images/manage.sh build phase5

phase5-images-list:
	./images/manage.sh list phase5

phase5-images-save:
	./images/manage.sh save phase5

phase5-images-pull-amd64:
	./images/manage.sh pull phase5 linux/amd64

phase5-images-build-amd64:
	./images/manage.sh build phase5 linux/amd64

phase5-images-list-amd64:
	./images/manage.sh list phase5 linux/amd64

phase5-images-save-amd64:
	./images/manage.sh save phase5 linux/amd64

phase3-upgrade-compose-config:
	./scripts/phase3-compose-from-phase0.sh config --quiet

phase3-upgrade-up:
	./scripts/phase3-compose-from-phase0.sh up -d

phase3-upgrade-ps:
	./scripts/phase3-compose-from-phase0.sh ps

phase3-upgrade-down:
	./scripts/phase3-compose-from-phase0.sh down

phase4-compose-config:
	./scripts/phase4-compose.sh config --quiet

phase4-build:
	./scripts/phase4-compose.sh build frontend gateway

phase4-up:
	./scripts/phase4-compose.sh up -d

phase4-ps:
	./scripts/phase4-compose.sh ps

phase4-down:
	./scripts/phase4-compose.sh down

phase4-upgrade-compose-config:
	./scripts/phase4-compose-from-phase0.sh config --quiet

phase4-upgrade-up:
	./scripts/phase4-compose-from-phase0.sh up -d

phase4-upgrade-ps:
	./scripts/phase4-compose-from-phase0.sh ps

phase4-upgrade-down:
	./scripts/phase4-compose-from-phase0.sh down

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

phase3-gate-a-static-check:
	./scripts/check-phase3-gate-a.sh

phase3-gate-b-static-check:
	./scripts/check-phase3-gate-b.sh

phase3-gate-c-static-check:
	./scripts/check-phase3-gate-c.sh

phase3-gate-d-static-check:
	./scripts/check-phase3-gate-d.sh

phase3-upstream-contract-check:
	./scripts/check-phase3-upstream-contract.sh

phase3-clean-copy-check:
	./scripts/phase3-clean-copy-check.sh

phase3-check: phase2-check phase3-gate-a-static-check phase3-gate-b-static-check phase3-gate-c-static-check phase3-gate-d-static-check phase3-upstream-contract-check

phase3-gate-a: phase3-check
	python3 ./scripts/phase1-migration-probe.py

phase3-gate-b-probe:
	python3 ./scripts/phase3-gate-b-probe.py

phase3-gate-b:
	./scripts/phase1-runtime-check.sh
	python3 ./scripts/phase1-migration-probe.py
	python3 ./scripts/phase3-gate-b-probe.py
	./scripts/check-phase3-gate-b.sh

phase3-gate-c: phase3-gate-c-static-check
	./scripts/mindcreek-compose.sh build frontend

phase3-gate-d: phase3-check phase3-clean-copy-check

phase4-gate-a-static-check:
	./scripts/check-phase4-gate-a.sh

phase4-gate-b-static-check:
	./scripts/check-phase4-gate-b.sh

phase4-gate-c-static-check:
	./scripts/check-phase4-gate-c.sh

phase4-gate-d-static-check:
	./scripts/check-phase4-gate-d.sh

phase4-upstream-contract-check:
	./scripts/check-phase4-upstream-contract.sh

phase4-clean-copy-check:
	./scripts/phase4-clean-copy-check.sh

phase4-check: phase2-check phase3-gate-a-static-check phase3-gate-b-static-check phase3-gate-c-static-check phase4-gate-a-static-check phase4-gate-b-static-check phase4-gate-c-static-check phase4-gate-d-static-check phase4-upstream-contract-check

phase4-gate-a: phase4-gate-a-static-check
	mkdir -p .local/gateway-go-build
	cd services/gateway && GOCACHE=$(CURDIR)/.local/gateway-go-build go test ./internal/access ./internal/agentscope ./internal/agentaudit ./internal/database
	python3 ./scripts/phase1-migration-probe.py

phase4-gate-b-probe:
	python3 ./scripts/phase4-gate-b-probe.py

phase4-gate-b: phase4-gate-b-static-check
	python3 ./scripts/phase4-gate-b-probe.py

phase4-gate-c-probe:
	python3 ./scripts/phase4-gate-c-probe.py

phase4-gate-c: phase4-gate-c-static-check
	mkdir -p .local/gateway-go-build
	cd services/gateway && GOCACHE=$(CURDIR)/.local/gateway-go-build go test ./internal/mcp
	python3 ./scripts/phase4-gate-c-probe.py

phase4-gate-d: phase4-check phase4-clean-copy-check

phase5-models-render:
	python3 ./scripts/render-phase5-models.py --env-file .local/mindcreek.env --output .local/phase5/builtin_models.yaml

phase5-compose-config:
	./scripts/phase5-compose.sh config --quiet

phase5-production-compose-config:
	./scripts/phase5-production-compose.sh config --quiet

phase5-build:
	./scripts/phase5-compose.sh build frontend gateway

phase5-build-offline:
	MINDCREEK_GATEWAY_TAG=phase5 MINDCREEK_VERSION=0.6.0-phase5 ./scripts/build-gateway-image-offline.sh
	./scripts/build-phase5-ui-offline.sh

phase5-up:
	./scripts/phase5-compose.sh up -d

phase5-ps:
	./scripts/phase5-compose.sh ps

phase5-down:
	./scripts/phase5-compose.sh down

phase5-runtime-check:
	./scripts/phase5-runtime-check.sh

phase5-gate-a-static-check:
	./scripts/check-phase5-gate-a.sh

phase5-gate-a-probe:
	python3 ./scripts/phase5-gate-a-probe.py

phase5-gate-a: phase5-gate-a-static-check
	mkdir -p .local/gateway-go-build
	cd services/gateway && GOCACHE=$(CURDIR)/.local/gateway-go-build go test ./...
	./scripts/phase5-runtime-check.sh
	python3 ./scripts/phase5-gate-a-probe.py

phase5-gate-b-static-check:
	./scripts/check-phase5-gate-b.sh

phase5-gate-b-probe:
	python3 ./scripts/phase5-gate-b-probe.py

phase5-gate-b: phase5-gate-b-static-check
	mkdir -p .local/gateway-go-build
	cd services/gateway && GOCACHE=$(CURDIR)/.local/gateway-go-build go test ./...
	./scripts/phase5-compose.sh config --quiet

phase5-backup:
	./scripts/phase5-backup.sh

phase5-recovery-drill:
	./scripts/phase5-recovery-drill.sh

phase5-observability-probe:
	python3 ./scripts/phase5-observability-probe.py

phase5-load-probe:
	python3 ./scripts/phase5-load-probe.py

phase5-migration-probe:
	python3 ./scripts/phase5-migration-probe.py

phase5-failure-recovery-probe:
	./scripts/phase5-failure-recovery-probe.sh

phase5-security-scan:
	./scripts/phase5-security-scan.sh

phase5-gate-c-static-check:
	./scripts/check-phase5-gate-c.sh

phase5-gate-c: phase5-gate-c-static-check
	mkdir -p .local/gateway-go-build
	cd services/gateway && GOCACHE=$(CURDIR)/.local/gateway-go-build go test ./...
	./scripts/phase5-runtime-check.sh
	python3 ./scripts/phase5-migration-probe.py
	python3 ./scripts/phase5-load-probe.py
	./scripts/phase5-failure-recovery-probe.sh
	./scripts/phase5-recovery-drill.sh
	python3 ./scripts/phase5-observability-probe.py
	./scripts/phase5-security-scan.sh

phase5-pilot-probe:
	python3 ./scripts/phase5-pilot-probe.py

phase5-upstream-contract-check:
	./scripts/check-phase5-upstream-contract.sh

phase5-clean-copy-check:
	./scripts/phase5-clean-copy-check.sh

phase5-gate-d-static-check:
	./scripts/check-phase5-gate-d.sh

phase5-check: phase4-check phase5-gate-a-static-check phase5-gate-b-static-check phase5-gate-c-static-check phase5-gate-d-static-check phase5-upstream-contract-check

phase5-gate-d: phase5-check phase5-clean-copy-check
	python3 ./scripts/phase5-pilot-probe.py

phase5-upgrade-compose-config:
	./scripts/phase5-compose-from-phase0.sh config --quiet

phase5-upgrade-up:
	./scripts/phase5-compose-from-phase0.sh up -d

phase5-upgrade-ps:
	./scripts/phase5-compose-from-phase0.sh ps

phase5-upgrade-down:
	./scripts/phase5-compose-from-phase0.sh down

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
