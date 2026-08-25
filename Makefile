.PHONY: help phase0-check phase0-compose-config phase0-up phase0-ps phase0-runtime-check phase0-probe phase0-down upstream-status upstream-test upstream-test-go upstream-test-frontend upstream-test-mcp

help:
	@echo "MyKB repository commands"
	@echo "  make phase0-check          Verify design and immutable upstream boundary"
	@echo "  make phase0-compose-config Validate the isolated Phase 0 Compose profile"
	@echo "  make phase0-up             Start the isolated Phase 0 runtime"
	@echo "  make phase0-ps             Show Phase 0 container status"
	@echo "  make phase0-runtime-check  Verify services, health, version, and revision"
	@echo "  make phase0-probe          Run synthetic authorization/retrieval probes"
	@echo "  make phase0-down           Stop Phase 0 containers (preserve volumes)"
	@echo "  make upstream-status       Show the pinned WeKnora identity"
	@echo "  make upstream-test         Run Go, frontend, and MCP upstream suites"
	@echo "  make upstream-test-go      Run the upstream Go suite"
	@echo "  make upstream-test-frontend Run upstream frontend tests/type-check/build"
	@echo "  make upstream-test-mcp     Run upstream MCP unit tests"

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
