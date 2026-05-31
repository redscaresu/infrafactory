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
	ui-install ui-build ui-test ui-test-e2e ui-dev ui-clean ui-api-linux-build ui-stack-up ui-stack-logs ui-stack-down build run up down \
	mocks-up mocks-down mocks-status mocks-logs mockway-up mockway-down fakegcp-up fakegcp-down fakeaws-up fakeaws-down

help:
	@echo "Targets:"
	@echo "  up              One-shot bring-up: all mocks + SeaweedFS + UI (most common starter)."
	@echo "  down            Symmetric tear-down: stop all mocks (UI stops on Ctrl-C)."
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
	@pidfile=$(MOCKS_RUN_DIR)/mockway.pid; \
	if [ -f $$pidfile ]; then \
		pid=$$(cat $$pidfile); \
		kill $$pid 2>/dev/null || true; \
		wait $$pid 2>/dev/null || true; \
		rm -f $$pidfile; \
	fi; \
	port_pid=$$(lsof -nP -iTCP:$(MOCKWAY_PORT) -sTCP:LISTEN -t 2>/dev/null); \
	if [ -n "$$port_pid" ]; then \
		echo "killing stale process(es) on port $(MOCKWAY_PORT): $$port_pid"; \
		kill $$port_pid 2>/dev/null || true; \
		sleep 1; \
		port_pid=$$(lsof -nP -iTCP:$(MOCKWAY_PORT) -sTCP:LISTEN -t 2>/dev/null); \
		[ -n "$$port_pid" ] && kill -9 $$port_pid 2>/dev/null || true; \
	fi; \
	echo "mockway stopped"

fakegcp-down:
	@pidfile=$(MOCKS_RUN_DIR)/fakegcp.pid; \
	if [ -f $$pidfile ]; then \
		pid=$$(cat $$pidfile); \
		kill $$pid 2>/dev/null || true; \
		wait $$pid 2>/dev/null || true; \
		rm -f $$pidfile; \
	fi; \
	port_pid=$$(lsof -nP -iTCP:$(FAKEGCP_PORT) -sTCP:LISTEN -t 2>/dev/null); \
	if [ -n "$$port_pid" ]; then \
		echo "killing stale process(es) on port $(FAKEGCP_PORT): $$port_pid"; \
		kill $$port_pid 2>/dev/null || true; \
		sleep 1; \
		port_pid=$$(lsof -nP -iTCP:$(FAKEGCP_PORT) -sTCP:LISTEN -t 2>/dev/null); \
		[ -n "$$port_pid" ] && kill -9 $$port_pid 2>/dev/null || true; \
	fi; \
	echo "fakegcp stopped"

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
	@pidfile=$(MOCKS_RUN_DIR)/fakeaws.pid; \
	if [ -f $$pidfile ]; then \
		pid=$$(cat $$pidfile); \
		kill $$pid 2>/dev/null || true; \
		wait $$pid 2>/dev/null || true; \
		rm -f $$pidfile; \
	fi; \
	port_pid=$$(lsof -nP -iTCP:$(FAKEAWS_PORT) -sTCP:LISTEN -t 2>/dev/null); \
	if [ -n "$$port_pid" ]; then \
		echo "killing stale process(es) on port $(FAKEAWS_PORT): $$port_pid"; \
		kill $$port_pid 2>/dev/null || true; \
		sleep 1; \
		port_pid=$$(lsof -nP -iTCP:$(FAKEAWS_PORT) -sTCP:LISTEN -t 2>/dev/null); \
		[ -n "$$port_pid" ] && kill -9 $$port_pid 2>/dev/null || true; \
	fi; \
	echo "fakeaws stopped"

# seaweedfs-up / -down — M94. AWS scenarios depend on SeaweedFS
# (S3-compatible) on :9090 for sub-resource Read flows; without it
# every AWS scenario fails at `s3 reset: connection refused` before
# the LLM is even invoked. Requires Docker. Container name is
# pinned so seaweedfs-down can find it reliably.
SEAWEEDFS_PORT ?= 9090
SEAWEEDFS_CONTAINER ?= infrafactory-seaweedfs
SEAWEEDFS_IMAGE ?= chrislusf/seaweedfs:latest

seaweedfs-up:
	@if curl -sSf http://127.0.0.1:$(SEAWEEDFS_PORT)/ >/dev/null 2>&1; then \
		echo "seaweedfs already listening on http://127.0.0.1:$(SEAWEEDFS_PORT)"; \
	elif ! command -v docker >/dev/null 2>&1; then \
		echo "WARN: docker not found — AWS scenarios will fail at s3 reset"; \
	else \
		echo "starting seaweedfs on http://127.0.0.1:$(SEAWEEDFS_PORT) (container=$(SEAWEEDFS_CONTAINER))"; \
		docker run -d --name $(SEAWEEDFS_CONTAINER) --rm \
			-p 127.0.0.1:$(SEAWEEDFS_PORT):8333 \
			$(SEAWEEDFS_IMAGE) server -s3 -s3.port=8333 -s3.allowEmptyFolder=true >/dev/null; \
		until curl -sSf http://127.0.0.1:$(SEAWEEDFS_PORT)/ >/dev/null 2>&1; do sleep 1; done; \
		echo "seaweedfs ready on http://127.0.0.1:$(SEAWEEDFS_PORT)"; \
	fi

seaweedfs-down:
	@if docker ps --format '{{.Names}}' 2>/dev/null | grep -q "^$(SEAWEEDFS_CONTAINER)$$"; then \
		docker stop $(SEAWEEDFS_CONTAINER) >/dev/null 2>&1 || true; \
		echo "seaweedfs stopped"; \
	else \
		echo "seaweedfs not running"; \
	fi

# mocks-up starts all three mocks. Run from the infrafactory repo root
# with ../mockway, ../fakegcp, ../fakeaws checked out as siblings.
# M94: seaweedfs-up added — AWS scenarios silently fail without
# port 9090. Docker required for that piece only.
mocks-up: mockway-up fakegcp-up fakeaws-up seaweedfs-up
	@echo "all mocks ready: $(MOCKWAY_URL) (Scaleway), $(FAKEGCP_URL) (GCP), $(FAKEAWS_URL) (AWS), http://127.0.0.1:$(SEAWEEDFS_PORT) (S3/SeaweedFS)"

mocks-down: mockway-down fakegcp-down fakeaws-down seaweedfs-down
	@echo "all mocks stopped"

mocks-status:
	@for entry in "mockway:$(MOCKWAY_PORT)" "fakegcp:$(FAKEGCP_PORT)" "fakeaws:$(FAKEAWS_PORT)" "seaweedfs:$(SEAWEEDFS_PORT)"; do \
		name=$${entry%:*}; \
		port=$${entry##*:}; \
		pidfile=$(MOCKS_RUN_DIR)/$$name.pid; \
		port_pid=$$(lsof -nP -iTCP:$$port -sTCP:LISTEN -t 2>/dev/null | head -1); \
		if [ -n "$$port_pid" ]; then \
			echo "$$name: up (port=$$port pid=$$port_pid)"; \
		elif [ -f $$pidfile ] && kill -0 $$(cat $$pidfile) 2>/dev/null; then \
			echo "$$name: up (pid=$$(cat $$pidfile), port $$port not listening?)"; \
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

# ----- Containerized multi-mock path (alternative to mocks-up) -----
#
# mocks-up-containers brings up all three mocks via the published
# GHCR images orchestrated by docker-compose.mocks.yml. Use this
# when you don't have Go installed locally (CI, contributor
# machines), or want a reproducible per-version image set. The
# port allocation is identical to mocks-up (8080/8081/8082) so
# scenarios + infrafactory.yaml don't care which path is active.
#
# To bring up s3mock alongside (once M59-T1 lands), edit
# docker-compose.mocks.yml and re-run mocks-up-containers.

MOCKS_COMPOSE := $(COMPOSE) -f docker-compose.mocks.yml

mocks-up-containers:
	$(MOCKS_COMPOSE) up -d
	@echo "all three mocks ready (containers):"
	@echo "  mockway → http://127.0.0.1:8080  (Scaleway)"
	@echo "  fakegcp → http://127.0.0.1:8081  (GCP)"
	@echo "  fakeaws → http://127.0.0.1:8082  (AWS)"

mocks-down-containers:
	$(MOCKS_COMPOSE) down
	@echo "all mock containers stopped"

mocks-pull:
	$(MOCKS_COMPOSE) pull
	@echo "GHCR images refreshed"

mocks-status-containers:
	$(MOCKS_COMPOSE) ps

mocks-logs-containers:
	$(MOCKS_COMPOSE) logs --tail 50

test-unit:
	$(GO) test ./internal/... ./cmd/...

# test-ci-parity reproduces the CI test conditions that caused the
# May 2026 "passes on Mac, fails on Linux" regression in
# TestCommandOutputGoldenSnapshots/run_json. Linux schedules t.Parallel
# subtests aggressively enough that two run subtests with the same
# scenario name + same wall-clock-second produced identical runIDs and
# raced on shared relative-path defaults (`./output`,
# `.infrafactory/runs`). Running with `-count=3` surfaces this class of
# parallel-subtest races locally before they reach the CI badge. Run
# this before pushing any change that touches the CLI test harness.
test-ci-parity:
	$(GO) test -count=3 ./internal/cli/

ui-test:
	cd ui && npm test

ui-test-e2e: ui-build
	cd ui && npx playwright test

# demo-ui records a fresh docs/demo/ui-walkthrough.webm by driving the
# embedded UI through the full-stack-paris scenario via Playwright. No
# LLM credits required — the spec only navigates the UI; the actual
# run is the matching CLI cast at docs/demo/infrafactory.cast. Run
# after a UI change that affects the recorded surface.
demo-ui: ui-build
	cd ui && npx playwright test --config=playwright-demo.config.ts -g "full-stack-paris"
	cp docs/demo/walkthrough/ui-walkthrough-UI-walkthrough-full-stack-paris-chromium/video.webm docs/demo/ui-walkthrough.webm
	@echo "Updated docs/demo/ui-walkthrough.webm"
	@if command -v gifski >/dev/null 2>&1; then \
	  gifski --output docs/demo/ui-walkthrough.gif --fps 15 --width 900 --quality 85 docs/demo/ui-walkthrough.webm; \
	  echo "Updated docs/demo/ui-walkthrough.gif"; \
	else \
	  echo "WARN: gifski not installed; skipping GIF render. brew install gifski, then:"; \
	  echo "      gifski --output docs/demo/ui-walkthrough.gif --fps 15 --width 900 --quality 85 docs/demo/ui-walkthrough.webm"; \
	fi

# demo-ui-run records docs/demo/ui-walkthrough-run.webm — the
# live-run variant of demo-ui. Drives an actual `infrafactory run`
# of gcp-pubsub (Pub/Sub topic + subscription against fakegcp;
# converges in 2 iterations — iter 1 fails because fakegcp doesn't
# model google_project_service, LLM corrects and iter 2 succeeds)
# through the UI: scenario page → click Run → Live page populates
# with iteration stages → success banner → per-run IaC preview
# shows the AI's converged HCL. REQUIRES: fakegcp running on :8081
# (via `make mocks-up`) + Claude CLI authenticated (or
# OPENROUTER_API_KEY exported). End-to-end ~150–180s.
demo-ui-run: ui-build
	cd ui && npx playwright test --config=playwright-demo.config.ts -g "live run of gcp-pubsub"
	cp docs/demo/walkthrough/ui-walkthrough-run-UI-walkthrough-live-run-of-gcp-pubsub-chromium/video.webm docs/demo/ui-walkthrough-run.webm
	@echo "Updated docs/demo/ui-walkthrough-run.webm"
	@if command -v gifski >/dev/null 2>&1; then \
	  gifski --output docs/demo/ui-walkthrough-run.gif --fps 15 --width 900 --quality 85 docs/demo/ui-walkthrough-run.webm; \
	  echo "Updated docs/demo/ui-walkthrough-run.gif"; \
	else \
	  echo "WARN: gifski not installed; skipping GIF render. brew install gifski, then:"; \
	  echo "      gifski --output docs/demo/ui-walkthrough-run.gif --fps 15 --width 900 --quality 85 docs/demo/ui-walkthrough-run.webm"; \
	fi

# ui-baseline-update refreshes the Playwright visual-regression
# baselines under ui/e2e/visual.spec.ts-snapshots/. The masks in
# visual.spec.ts hide volatile content (sidebar scenario lists,
# home-page grid, mock-status pill) but they DON'T constrain
# natural-flow layout height — adding a scenario YAML still grows
# the sidebar and (therefore) the page by ~28px per entry. Refresh
# the baselines whenever a scenarios/training/*.yaml file is added
# or removed; stage the regenerated PNGs alongside the scenario in
# the same commit. See project_visual_regression_masking.md for the
# full history.
ui-baseline-update: ui-build
	cd ui && npx playwright test e2e/visual.spec.ts --update-snapshots
	@echo "Refreshed ui/e2e/visual.spec.ts-snapshots/*.png"

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

# up: one-shot bring-up — every mock + SeaweedFS + UI/API in one command.
# Use when you sit down to a fresh terminal and want the full stack hot.
# Idempotent: each mocks-* target checks for an existing pid/listener
# before starting. SeaweedFS needs Docker running (Docker Desktop on
# macOS) — the target will report which step failed if anything's down.
#
#   make up   # bring everything up
#   make down # tear everything down (mocks-down + ui-stack-down)
#
# Layout afterwards:
#   :8080  mockway (Scaleway)
#   :8081  fakegcp (GCP)
#   :8082  fakeaws (AWS)
#   :9090  SeaweedFS (S3-compatible)
#   :4173  infrafactory UI/API (served by `infrafactory ui`)
up: mocks-up build
	@echo "==> mocks ready: mockway :8080, fakegcp :8081, fakeaws :8082, seaweedfs :9090"
	@echo "==> starting infrafactory UI on :4173 (Ctrl-C to stop)"
	./bin/infrafactory ui

# down — symmetric tear-down for `make up`. Mocks shut down, UI shuts
# itself down when interrupted; nothing else lingers.
down: mocks-down
	@echo "==> mocks stopped. UI process is foreground-only — exit it manually if still running."

# install-hooks wires the tracked hook installer at .githooks/ via
# core.hooksPath so the gitleaks + auto-baseline-refresh + make test
# pre-commit gate runs locally on every commit. Mirrors fakegcp /
# fakeaws / mockway pattern. Run once per clone.
install-hooks:
	git config core.hooksPath .githooks
	chmod +x .githooks/pre-commit
	@echo "Hooks installed: pre-commit will run gitleaks, refresh visual baselines if scenarios changed, and run make test."
