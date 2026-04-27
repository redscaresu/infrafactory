SHELL := /bin/bash

GO ?= go
COMPOSE ?= docker compose
MOCKWAY_URL ?= http://127.0.0.1:8080
MOCKWAY_IMAGE ?= ghcr.io/redscaresu/mockway
MOCKWAY_CONTAINER ?= infrafactory-mockway
MOCKWAY_BIN ?= mockway
MOCKWAY_PORT ?= 8080
MOCKWAY_REPO ?= ../mockway
FAKEGCP_PORT ?= 8081
FAKEGCP_URL ?= http://127.0.0.1:$(FAKEGCP_PORT)
FAKEGCP_REPO ?= ../fakegcp
FAKEAWS_PORT ?= 8082
FAKEAWS_URL ?= http://127.0.0.1:$(FAKEAWS_PORT)
FAKEAWS_REPO ?= ../fakeaws
MOCKS_RUN_DIR ?= /tmp/infrafactory-mocks
HOST_ARCH ?= $(shell uname -m)

ifeq ($(HOST_ARCH),arm64)
LINUX_GOARCH := arm64
else ifeq ($(HOST_ARCH),aarch64)
LINUX_GOARCH := arm64
else ifeq ($(HOST_ARCH),x86_64)
LINUX_GOARCH := amd64
else
LINUX_GOARCH := $(HOST_ARCH)
endif

.PHONY: help deps-up deps-down deps-ps deps-logs deps-pull deps-recreate deps-clean test-unit test-all test \
	bench-check smoke-validate smoke-mockway smoke-mockway-manual smoke-mockway-local smoke check \
	ui-install ui-build ui-test ui-test-e2e ui-dev ui-clean ui-api-linux-build ui-stack-up ui-stack-logs ui-stack-down build run \
	mocks-up mocks-down mocks-status mocks-logs mockway-up mockway-down fakegcp-up fakegcp-down fakeaws-up fakeaws-down

help:
	@echo "Targets:"
	@echo "  deps-up         Start dependency containers (mockway)."
	@echo "  mocks-up        Start mockway (:$(MOCKWAY_PORT)) AND fakegcp (:$(FAKEGCP_PORT)) from source siblings."
	@echo "  mocks-down      Stop both mocks."
	@echo "  mocks-status    Show running state of mockway / fakegcp."
	@echo "  mocks-logs      Tail the last 20 log lines of each mock."
	@echo "  mockway-up / fakegcp-up   Start one mock at a time."
	@echo "  mockway-down / fakegcp-down  Stop one mock at a time."
	@echo "  deps-down       Stop dependency containers."
	@echo "  deps-ps         Show dependency container status."
	@echo "  deps-logs       Tail dependency logs."
	@echo "  deps-pull       Pull latest dependency images."
	@echo "  deps-recreate   Recreate dependency containers from scratch."
	@echo "  deps-clean      Stop and remove dependency containers + volumes."
	@echo "  test-unit       Run hermetic Go package tests."
	@echo "  ui-test         Run frontend unit tests."
	@echo "  ui-test-e2e     Build UI and run Playwright e2e tests."
	@echo "  test            Run all tests (Go unit + UI unit + Playwright e2e)."
	@echo "  test-all        Run full local checks (go test + doc hygiene)."
	@echo "  bench-check     Run env-gated benchmark regression checks."
	@echo "  smoke-validate  Run opt-in real-tool validate smoke test."
	@echo "  smoke-mockway   Run opt-in real-tool test smoke against Mockway."
	@echo "  smoke-mockway-manual  Run manual docker+curl+smoke sequence."
	@echo "  smoke-mockway-local   Run smoke test against local mockway binary."
	@echo "  smoke           Run both real-tool smoke targets."
	@echo "  check           Alias for test-all."
	@echo "  ui-install      Install frontend dependencies."
	@echo "  ui-dev          Run frontend dev server locally (:5173)."
	@echo "  ui-build        Build frontend static assets."
	@echo "  ui-clean        Remove frontend build/dependency artifacts."
	@echo "  ui-api-linux-build  Build Linux UI binary for Docker dev stack."
	@echo "  ui-stack-up     Start API + frontend dev stack in Docker Compose."
	@echo "  ui-stack-logs   Tail API + frontend docker logs."
	@echo "  ui-stack-down   Stop and remove API + frontend docker services."
	@echo "  build           Build frontend + Go binary into bin/infrafactory."
	@echo "  run             Build and start the UI server (http://127.0.0.1:4173)."

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

# Multi-cloud mock orchestration: mockway (Scaleway, :8080) + fakegcp
# (GCP, :8081) running side-by-side from sibling source repos. The
# resulting URLs match the Mockway.URL / Fakegcp.URL keys
# infrafactory.yaml expects, so a `cloud: gcp` scenario routes to
# fakegcp via the cloudMockStateRouter without any extra config.
#
# pids and logs land under $(MOCKS_RUN_DIR) so the down targets can
# reliably stop the right processes.
$(MOCKS_RUN_DIR):
	mkdir -p $(MOCKS_RUN_DIR)

mockway-up: $(MOCKS_RUN_DIR)
	@if [ -f $(MOCKS_RUN_DIR)/mockway.pid ] && kill -0 $$(cat $(MOCKS_RUN_DIR)/mockway.pid) 2>/dev/null; then \
		echo "mockway already running (pid=$$(cat $(MOCKS_RUN_DIR)/mockway.pid)) on $(MOCKWAY_URL)"; \
	else \
		echo "starting mockway on $(MOCKWAY_URL) ($(MOCKWAY_REPO))"; \
		cd $(MOCKWAY_REPO) && $(GO) run ./cmd/mockway --port $(MOCKWAY_PORT) > $(MOCKS_RUN_DIR)/mockway.log 2>&1 & \
		echo $$! > $(MOCKS_RUN_DIR)/mockway.pid; \
		until curl -sSf $(MOCKWAY_URL)/mock/state >/dev/null 2>&1; do \
			sleep 1; \
		done; \
		echo "mockway ready on $(MOCKWAY_URL) (pid=$$(cat $(MOCKS_RUN_DIR)/mockway.pid))"; \
	fi

fakegcp-up: $(MOCKS_RUN_DIR)
	@if [ -f $(MOCKS_RUN_DIR)/fakegcp.pid ] && kill -0 $$(cat $(MOCKS_RUN_DIR)/fakegcp.pid) 2>/dev/null; then \
		echo "fakegcp already running (pid=$$(cat $(MOCKS_RUN_DIR)/fakegcp.pid)) on $(FAKEGCP_URL)"; \
	else \
		echo "starting fakegcp on $(FAKEGCP_URL) ($(FAKEGCP_REPO))"; \
		cd $(FAKEGCP_REPO) && $(GO) run ./cmd/fakegcp --port $(FAKEGCP_PORT) > $(MOCKS_RUN_DIR)/fakegcp.log 2>&1 & \
		echo $$! > $(MOCKS_RUN_DIR)/fakegcp.pid; \
		until curl -sSf $(FAKEGCP_URL)/mock/state >/dev/null 2>&1; do \
			sleep 1; \
		done; \
		echo "fakegcp ready on $(FAKEGCP_URL) (pid=$$(cat $(MOCKS_RUN_DIR)/fakegcp.pid))"; \
	fi

mockway-down:
	@if [ -f $(MOCKS_RUN_DIR)/mockway.pid ]; then \
		pid=$$(cat $(MOCKS_RUN_DIR)/mockway.pid); \
		kill $$pid 2>/dev/null || true; \
		wait $$pid 2>/dev/null || true; \
		rm -f $(MOCKS_RUN_DIR)/mockway.pid; \
		echo "mockway stopped"; \
	else \
		echo "mockway pidfile not found"; \
	fi

fakegcp-down:
	@if [ -f $(MOCKS_RUN_DIR)/fakegcp.pid ]; then \
		pid=$$(cat $(MOCKS_RUN_DIR)/fakegcp.pid); \
		kill $$pid 2>/dev/null || true; \
		wait $$pid 2>/dev/null || true; \
		rm -f $(MOCKS_RUN_DIR)/fakegcp.pid; \
		echo "fakegcp stopped"; \
	else \
		echo "fakegcp pidfile not found"; \
	fi

# fakeaws-up / fakeaws-down — S43-T9 (the AWS sibling, port 8082).
fakeaws-up: $(MOCKS_RUN_DIR)
	@if [ -f $(MOCKS_RUN_DIR)/fakeaws.pid ] && kill -0 $$(cat $(MOCKS_RUN_DIR)/fakeaws.pid) 2>/dev/null; then \
		echo "fakeaws already running (pid=$$(cat $(MOCKS_RUN_DIR)/fakeaws.pid)) on $(FAKEAWS_URL)"; \
	else \
		echo "starting fakeaws on $(FAKEAWS_URL) ($(FAKEAWS_REPO))"; \
		cd $(FAKEAWS_REPO) && $(GO) run ./cmd/fakeaws --port $(FAKEAWS_PORT) > $(MOCKS_RUN_DIR)/fakeaws.log 2>&1 & \
		echo $$! > $(MOCKS_RUN_DIR)/fakeaws.pid; \
		until curl -sSf $(FAKEAWS_URL)/mock/state >/dev/null 2>&1; do \
			sleep 1; \
		done; \
		echo "fakeaws ready on $(FAKEAWS_URL) (pid=$$(cat $(MOCKS_RUN_DIR)/fakeaws.pid))"; \
	fi

fakeaws-down:
	@if [ -f $(MOCKS_RUN_DIR)/fakeaws.pid ]; then \
		pid=$$(cat $(MOCKS_RUN_DIR)/fakeaws.pid); \
		kill $$pid 2>/dev/null || true; \
		wait $$pid 2>/dev/null || true; \
		rm -f $(MOCKS_RUN_DIR)/fakeaws.pid; \
		echo "fakeaws stopped"; \
	else \
		echo "fakeaws pidfile not found"; \
	fi

# mocks-up starts all three mocks. Run from the infrafactory repo root
# with ../mockway, ../fakegcp, ../fakeaws checked out as siblings.
mocks-up: mockway-up fakegcp-up fakeaws-up
	@echo "all three mocks ready: $(MOCKWAY_URL) (Scaleway), $(FAKEGCP_URL) (GCP), $(FAKEAWS_URL) (AWS)"

mocks-down: mockway-down fakegcp-down fakeaws-down
	@echo "all three mocks stopped"

mocks-status:
	@for name in mockway fakegcp fakeaws; do \
		pidfile=$(MOCKS_RUN_DIR)/$$name.pid; \
		if [ -f $$pidfile ] && kill -0 $$(cat $$pidfile) 2>/dev/null; then \
			echo "$$name: up (pid=$$(cat $$pidfile))"; \
		else \
			echo "$$name: down"; \
		fi; \
	done

mocks-logs:
	@for name in mockway fakegcp; do \
		log=$(MOCKS_RUN_DIR)/$$name.log; \
		echo "=== $$name ==="; \
		[ -f $$log ] && tail -n 20 $$log || echo "(no log yet)"; \
	done

test-unit:
	$(GO) test ./internal/... ./cmd/...

ui-test:
	cd ui && npm test

ui-test-e2e: ui-build
	cd ui && npx playwright test

test: test-unit ui-test ui-test-e2e

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

ui-install:
	cd ui && npm install

ui-build: ui-install
	cd ui && npm run build
	rm -rf cmd/infrafactory/ui/build
	mkdir -p cmd/infrafactory/ui
	cp -R ui/build cmd/infrafactory/ui/

ui-dev:
	cd ui && npm run dev -- --host 127.0.0.1 --port 5173

ui-clean:
	rm -rf ui/build ui/node_modules ui/.svelte-kit cmd/infrafactory/ui/build

ui-api-linux-build:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(LINUX_GOARCH) $(GO) build -o bin/infrafactory-ui-linux-$(LINUX_GOARCH) ./cmd/infrafactory

ui-stack-up: ui-build ui-api-linux-build
	UI_API_BINARY=/workspace/bin/infrafactory-ui-linux-$(LINUX_GOARCH) $(COMPOSE) --profile ui up -d infrafactory-api infrafactory-ui

ui-stack-logs:
	$(COMPOSE) logs -f --tail=200 infrafactory-api infrafactory-ui

ui-stack-down:
	-$(COMPOSE) stop infrafactory-api infrafactory-ui
	-$(COMPOSE) rm -f infrafactory-api infrafactory-ui

build: ui-build
	$(GO) build -o bin/infrafactory ./cmd/infrafactory

run: build
	./bin/infrafactory ui
