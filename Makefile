SHELL := /bin/bash

GO ?= go
COMPOSE ?= docker compose
MOCKWAY_URL ?= http://127.0.0.1:8080
MOCKWAY_IMAGE ?= ghcr.io/redscaresu/mockway
MOCKWAY_CONTAINER ?= infrafactory-mockway
MOCKWAY_BIN ?= mockway

.PHONY: help deps-up deps-down deps-ps deps-logs deps-pull deps-recreate deps-clean test-unit test-all bench-check smoke-validate smoke-mockway smoke-mockway-manual smoke-mockway-local smoke check

help:
	@echo "Targets:"
	@echo "  deps-up         Start dependency containers (mockway)."
	@echo "  deps-down       Stop dependency containers."
	@echo "  deps-ps         Show dependency container status."
	@echo "  deps-logs       Tail dependency logs."
	@echo "  deps-pull       Pull latest dependency images."
	@echo "  deps-recreate   Recreate dependency containers from scratch."
	@echo "  deps-clean      Stop and remove dependency containers + volumes."
	@echo "  test-unit       Run hermetic package tests."
	@echo "  test-all        Run full local checks (go test + doc hygiene)."
	@echo "  bench-check     Run env-gated benchmark regression checks."
	@echo "  smoke-validate  Run opt-in real-tool validate smoke test."
	@echo "  smoke-mockway   Run opt-in real-tool test smoke against Mockway."
	@echo "  smoke-mockway-manual  Run manual docker+curl+smoke sequence."
	@echo "  smoke-mockway-local   Run smoke test against local mockway binary."
	@echo "  smoke           Run both real-tool smoke targets."
	@echo "  check           Alias for test-all."

deps-up:
	$(COMPOSE) up -d mockway

deps-down:
	$(COMPOSE) down --remove-orphans

deps-ps:
	$(COMPOSE) ps

deps-logs:
	$(COMPOSE) logs -f --tail=200 mockway

deps-pull:
	$(COMPOSE) pull mockway

deps-recreate:
	$(COMPOSE) down --remove-orphans
	$(COMPOSE) up -d --force-recreate mockway

deps-clean:
	$(COMPOSE) down --remove-orphans --volumes

test-unit:
	$(GO) test ./internal/... ./cmd/...

test-all:
	bash scripts/check_all.sh

bench-check:
	INFRAFACTORY_ENABLE_BENCHMARKS=1 bash scripts/check_benchmarks.sh

smoke-validate:
	INFRAFACTORY_ENABLE_REALTOOL_SMOKE=1 $(GO) test ./internal/cli -run TestValidateCommandRealToolSmoke

smoke-mockway: deps-up
	@until curl -sSf $(MOCKWAY_URL)/mock/state >/dev/null; do \
		echo "waiting for mockway at $(MOCKWAY_URL) ..."; \
		sleep 1; \
	done
	INFRAFACTORY_ENABLE_REALTOOL_MOCKWAY=1 INFRAFACTORY_MOCKWAY_URL=$(MOCKWAY_URL) $(GO) test ./internal/cli -run TestTestCommandRealToolMockwaySmoke

smoke-mockway-manual:
	-docker rm -f $(MOCKWAY_CONTAINER) >/dev/null 2>&1
	docker run --rm -d --name $(MOCKWAY_CONTAINER) -p 8080:8080 $(MOCKWAY_IMAGE)
	curl -sSf http://127.0.0.1:8080/mock/state >/dev/null
	INFRAFACTORY_ENABLE_REALTOOL_MOCKWAY=1 INFRAFACTORY_MOCKWAY_URL=http://127.0.0.1:8080 $(GO) test ./internal/cli -run TestTestCommandRealToolMockwaySmoke

smoke-mockway-local:
	@set -euo pipefail; \
	command -v $(MOCKWAY_BIN) >/dev/null 2>&1 || { echo "mockway binary not found: $(MOCKWAY_BIN)"; exit 127; }; \
	$(MOCKWAY_BIN) > /tmp/infrafactory-mockway.log 2>&1 & \
	pid=$$!; \
	trap 'kill $$pid >/dev/null 2>&1 || true; wait $$pid 2>/dev/null || true' EXIT; \
	until curl -sSf http://127.0.0.1:8080/mock/state >/dev/null 2>&1; do \
		echo "waiting for mockway binary at http://127.0.0.1:8080 ..."; \
		sleep 1; \
	done; \
	INFRAFACTORY_ENABLE_REALTOOL_MOCKWAY=1 INFRAFACTORY_MOCKWAY_URL=http://127.0.0.1:8080 $(GO) test ./internal/cli -run TestTestCommandRealToolMockwaySmoke

smoke: smoke-validate smoke-mockway

check: test-all
