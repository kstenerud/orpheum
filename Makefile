# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: The project's one entry point. `make check` is the gate: the Stop hook runs it at the end of
# every agent turn that touched a file, the pre-commit hook runs it, and CI runs it. A change is
# not done until it passes. Standards: docs/contributors/standards/makefile.md.

SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help
MAKEFLAGS += --no-print-directory
.DELETE_ON_ERROR:

# Tests run hermetically (D190): the same clock zone, locale and git configuration on every
# machine, so a test cannot pass on the developer's box and fail in CI for reasons of taste.
# Only test targets get it: git itself may need the machine's config (safe.directory).
HERMETIC := TZ=UTC LC_ALL=C.UTF-8 GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1

# The capabilities a job may run without (testing.md 3): set per CI step or lab job, never in a
# profile. Every Go test run prints it, so a test that skipped for want of one is never silent.
UNCONTROLLED := echo "ORPHEUM_TEST_UNCONTROLLED=$${ORPHEUM_TEST_UNCONTROLLED:-(none: a missing capability fails)}"

# Pinned tool versions. Changing one is an owner action (the pre-edit hook asks), because a
# linter upgrade changes what the gate means.
GOLANGCI_LINT_VERSION := 2.13.2
GO_MINOR := 1.26
NODE_MAJOR := 22

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BASE ?= origin/main
PYTHON := python3

# Tools that `go install` puts in GOPATH/bin (sqlc, golangci-lint, dlv) are found by every
# target whatever the machine's profile says; a fresh container's PATH often lacks it.
export PATH := $(shell go env GOPATH)/bin:$(PATH)

.PHONY: help
help: ## List the targets
	@grep -E '^[a-z][a-z-]*:.*## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*## "} {printf "  %-22s %s\n", $$1, $$2}'

# ---------------------------------------------------------------------------------------------
# The gate
# ---------------------------------------------------------------------------------------------

.PHONY: check
check: check-tools check-docs check-scripts check-source check-deps check-claims check-go check-generated check-web ## The gate: everything that runs on every change
	@echo "make check: all gates passed"

.PHONY: check-tools
check-tools: ## Fail if a pinned tool is missing or at the wrong version
	@command -v go >/dev/null || { echo "go is not installed (need $(GO_MINOR).x)"; exit 1; }
	@case "$$(go env GOVERSION)" in go$(GO_MINOR)|go$(GO_MINOR).*) ;; \
	  *) echo "go is $$(go env GOVERSION); this project pins $(GO_MINOR).x"; exit 1;; esac
	@command -v golangci-lint >/dev/null || { echo "golangci-lint is not installed; run: make tools"; exit 1; }
	@v="$$(golangci-lint version --short)"; [ "$$v" = "$(GOLANGCI_LINT_VERSION)" ] || \
	  { echo "golangci-lint is $$v; this project pins $(GOLANGCI_LINT_VERSION). Run: make tools"; exit 1; }
	@command -v node >/dev/null || { echo "node is not installed (need $(NODE_MAJOR).x)"; exit 1; }
	@v="$$(node --version)"; case "$$v" in v$(NODE_MAJOR).*) ;; \
	  *) echo "node is $$v; this project pins $(NODE_MAJOR).x"; exit 1;; esac
	@command -v $(PYTHON) >/dev/null || { echo "python3 is not installed"; exit 1; }
	@command -v shellcheck >/dev/null || { echo "shellcheck is not installed"; exit 1; }

.PHONY: check-docs
check-docs: ## Links, anchors, decision citations and the decision index
	@$(PYTHON) scripts/check_docs.py

.PHONY: check-scripts
check-scripts: ## The gate scripts' own tests, and shellcheck on every shell script
	@$(HERMETIC) $(PYTHON) -m unittest discover -s scripts/tests -q
	@files="$$(git ls-files --cached --others --exclude-standard | grep -E '(\.sh$$|^\.githooks/)' || true)"; \
	  if [ -n "$$files" ]; then shellcheck $$files; fi

.PHONY: check-source
check-source: ## SPDX licence headers match their directory; files stay under 800 lines
	@$(PYTHON) scripts/check_source.py

.PHONY: check-deps
check-deps: ## Every dependency is on the owner's allowlist with a reason
	@$(PYTHON) scripts/check_deps.py

.PHONY: check-claims
check-claims: ## TestArch_ citations, decision IDs in code, and ORPHEUM_TEST_ variables resolve
	@$(PYTHON) scripts/check_claims.py

.PHONY: check-go
check-go: go-fmt go-lint go-vet go-test go-tidy go-speculative ## All Go checks

.PHONY: go-fmt
go-fmt: ## Fail if gofumpt or gci would change a file (fix with: make fmt)
	@golangci-lint fmt --diff ./...

.PHONY: go-lint
go-lint: ## golangci-lint with the project's configuration, for release and dev builds and the integration tier
	@golangci-lint run --build-tags integration ./...
	@golangci-lint run --build-tags dev ./cmd/...

.PHONY: go-vet
go-vet: ## go vet for the shipped platform, for arm64 Linux, for macOS, for the dev build and for the integration tier
	@go vet ./...
	@GOOS=linux GOARCH=arm64 go vet ./...  # D205: arm64 stays open
	@GOOS=darwin GOARCH=arm64 go vet ./...
	@go vet -tags dev ./...
	@go vet -tags integration ./...

.PHONY: go-test
go-test: ## Unit and architecture tests, for release and dev builds
	@$(UNCONTROLLED)
	@$(HERMETIC) go test ./...
	@$(HERMETIC) go test -tags dev ./cmd/...

.PHONY: go-tidy
go-tidy: ## Fail if go.mod or go.sum is not tidy
	@go mod tidy -diff

.PHONY: go-speculative
go-speculative: ## Fail on declarations whose only callers are tests
	@$(PYTHON) scripts/lint_speculative_api.py

.PHONY: check-generated
check-generated: ## Fail if committed generated code differs from what its source generates
	@$(PYTHON) scripts/check_generated.py

.PHONY: check-web
check-web: web-deps ## Typecheck, lint, format, dependency rules and tests for web/
	@npm --prefix web run --silent check

.PHONY: web-deps
web-deps: ## Install web dependencies from the lockfile when they are missing or stale
	@if [ ! -f web/node_modules/.package-lock.json ] || [ web/package-lock.json -nt web/node_modules/.package-lock.json ]; then \
	  npm --prefix web ci --no-audit --no-fund; fi

# ---------------------------------------------------------------------------------------------
# Branch gates: they compare a branch against its base, so they run in CI on a pull request and
# locally with `make check-pr` before pushing.
# ---------------------------------------------------------------------------------------------

.PHONY: check-pr
check-pr: test-integration check-revert-red check-test-weakening lint-commits ## Branch gates against BASE (default origin/main), after the integration tier

.PHONY: check-revert-red
check-revert-red: ## Every behaviour change can turn a test red when reverted (D190)
	@$(PYTHON) scripts/check_revert_red.py --base $(BASE)

.PHONY: check-test-weakening
check-test-weakening: ## No test got weaker without a Test-Weakened: trailer
	@$(PYTHON) scripts/check_test_weakening.py --base $(BASE)

.PHONY: lint-commits
lint-commits: ## Commit subjects, bodies and attribution trailers
	@$(PYTHON) scripts/lint_commits.py --base $(BASE)

# ---------------------------------------------------------------------------------------------
# Longer runs
# ---------------------------------------------------------------------------------------------

.PHONY: test-all
test-all: check ## The gate, then the race detector and uncached tests
	@$(UNCONTROLLED)
	@$(HERMETIC) go test -race -count=1 ./...

.PHONY: test-integration
test-integration: ## The integration tier: tests that run the pinned FFmpeg and MKVToolNix (fetched as make ffmpeg and make mkvtoolnix do)
	@$(UNCONTROLLED)
	@$(HERMETIC) ORPHEUM_TEST_FFMPEG_DIR="$(FFMPEG_DIR)" ORPHEUM_TEST_MKVTOOLNIX_DIR="$(MKVTOOLNIX_DIR)" \
		go test -tags integration -count=1 ./...

# ---------------------------------------------------------------------------------------------
# Building and generating
# ---------------------------------------------------------------------------------------------

# The binary embeds whatever is in $(WEBCLIENT_DIST) (D216 §2). `make build` fills it for the one
# go build and empties it after, so no other build or test embeds a client by accident; the
# committed .keep keeps the directory, so every build compiles.
WEBCLIENT_DIST := internal/core/webclient/dist
WEBCLIENT_CLEAR := find $(WEBCLIENT_DIST) -mindepth 1 -maxdepth 1 ! -name .keep -exec rm -rf {} +

.PHONY: build
build: web-deps ## Build the web apps into their dist/, then the server binary, with the web client in it, into bin/
	@npm --prefix web run --silent build
	@$(WEBCLIENT_CLEAR)
	@cp -r web/apps/web/dist/. $(WEBCLIENT_DIST)/
	@go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/orpheum ./cmd/orpheum
	@$(WEBCLIENT_CLEAR)

.PHONY: generate
generate: web-deps ## Regenerate committed generated code (sqlc for library.db and cache.db, the OpenAPI document, and the SDK's TS types)
	@if ls internal/store/queries/*.sql >/dev/null 2>&1; then \
	  command -v sqlc >/dev/null || { echo "sqlc is not installed; run: make tools-sqlc"; exit 1; }; \
	  sqlc generate && sqlc vet; \
	else echo "generate: no queries yet (internal/store/queries/*.sql); nothing to do"; fi
	@go run ./cmd/orpheum openapi > api/openapi.json.new && mv api/openapi.json.new api/openapi.json
	@npm --prefix web exec --workspace @orpheum/sdk -- openapi-typescript ../../../api/openapi.json \
	  --output src/gen/api.ts --alphabetize --empty-objects-unknown >/dev/null

.PHONY: fmt
fmt: ## Format Go and web sources in place
	@golangci-lint fmt ./...
	@npm --prefix web run --silent format

# ---------------------------------------------------------------------------------------------
# Development and debugging. Opinionated on purpose: every choice is printed by `make dev` and
# written in dev/orpheum.dev.toml and dev/seed.toml, never hidden (D183). All state lives in
# .dev/, which is disposable.
# ---------------------------------------------------------------------------------------------

DEV_DIR := .dev
DEV_CONFIG := $(DEV_DIR)/orpheum.toml
DEV_LISTEN ?=
FFMPEG_DIR = $(shell $(PYTHON) scripts/fetch_ffmpeg.py)
MKVTOOLNIX_DIR = $(shell $(PYTHON) scripts/fetch_mkvtoolnix.py)
PKG ?= ./...
RUN ?= .
FILE ?=
DELVE_VERSION := 1.27.2

.PHONY: ffmpeg
ffmpeg: ## Fetch the pinned FFmpeg (jellyfin-ffmpeg) into .tools/ and verify its hash
	@echo "ffmpeg: $(FFMPEG_DIR)"

.PHONY: mkvtoolnix
mkvtoolnix: ## Fetch the pinned MKVToolNix, which the integration tier edits files with, into .tools/
	@echo "mkvtoolnix: $(MKVTOOLNIX_DIR)"

.PHONY: dev
dev: web-deps ## Run the app: server + Vite (hot reload), seeded users and library, rebuilds on change
	@$(PYTHON) scripts/dev.py loop $(if $(DEV_LISTEN),--listen $(DEV_LISTEN))

.PHONY: dev-server
dev-server: ## Run the server alone with the dev config (no Vite), rebuilding on change
	@$(PYTHON) scripts/dev.py loop --no-web $(if $(DEV_LISTEN),--listen $(DEV_LISTEN))

.PHONY: dev-web
dev-web: web-deps ## Run only the Vite dev server, proxying to a server started elsewhere
	@npm --prefix web --workspace @orpheum/web run dev -- --host 127.0.0.1 --port 5173 --strictPort

.PHONY: dev-lan
dev-lan: ## Like dev-server, but listening on the LAN so a phone or TV can reach it (no microphone: that needs HTTPS)
	@$(PYTHON) scripts/dev.py loop --no-web --listen 0.0.0.0:8432

.PHONY: dev-config
dev-config: ## Render and print the dev config (.dev/orpheum.toml) — the settings dev runs with
	@$(PYTHON) scripts/dev.py config

.PHONY: dev-media
dev-media: ## Generate the synthetic dev library in .dev/media (FORCE=1 regenerates it)
	@$(PYTHON) scripts/dev_media.py --ffmpeg-dir "$(FFMPEG_DIR)" --dest $(DEV_DIR)/media $(if $(FORCE),--force)

.PHONY: dev-seed
dev-seed: ## Seed the dev database from dev/seed.toml, even if it is not empty
	@$(PYTHON) scripts/dev.py seed

.PHONY: dev-reset
dev-reset: ## Delete the dev database and caches (keeps the media and FFmpeg); next `make dev` re-seeds
	@rm -rf $(DEV_DIR)/state $(DEV_DIR)/cache $(DEV_DIR)/orpheum.log
	@echo "dev-reset: removed .dev/state, .dev/cache and the log"

.PHONY: dev-clean
dev-clean: ## Delete everything under .dev/, including the generated media
	@rm -rf $(DEV_DIR)

.PHONY: dev-logs
dev-logs: ## Follow the dev server's log
	@touch $(DEV_DIR)/orpheum.log && tail -F $(DEV_DIR)/orpheum.log

.PHONY: dev-db
dev-db: ## Open the dev database in an SQL shell (python3 -m sqlite3; stop the server before writing)
	@test -f $(DEV_DIR)/state/library.db || { echo "no dev database yet: run make dev"; exit 1; }
	@$(PYTHON) -m sqlite3 $(DEV_DIR)/state/library.db

.PHONY: dev-doctor
dev-doctor: ## Run `orpheum doctor` against the dev config
	@go build -tags dev -o $(DEV_DIR)/bin/orpheum ./cmd/orpheum
	@$(DEV_DIR)/bin/orpheum doctor --config $(DEV_CONFIG)

.PHONY: dev-debug
dev-debug: ## Run the dev server under delve, headless on 127.0.0.1:2345, for an IDE to attach
	@command -v dlv >/dev/null || { echo "delve is not installed; run: make tools-dlv"; exit 1; }
	@$(PYTHON) scripts/dev.py config >/dev/null
	@dlv debug ./cmd/orpheum --build-flags="-tags=dev" --headless --listen=127.0.0.1:2345 \
	  --api-version=2 --accept-multiclient -- run --config $(DEV_CONFIG)

.PHONY: pprof
pprof: ## Take a 30 s CPU profile of the running dev server and open it in the browser
	@go tool pprof -http=127.0.0.1:8433 "http://127.0.0.1:8432/debug/pprof/profile?seconds=30"

.PHONY: pprof-heap
pprof-heap: ## Open the running dev server's heap profile in the browser
	@go tool pprof -http=127.0.0.1:8433 "http://127.0.0.1:8432/debug/pprof/heap"

.PHONY: probe
probe: ## Show what the node would record about FILE: streams, format and chapters, as JSON
	@test -n "$(FILE)" || { echo 'usage: make probe FILE="path/to/film.mkv"'; exit 1; }
	@"$(FFMPEG_DIR)/ffprobe" -v error -show_format -show_streams -show_chapters -of json "$(FILE)"

.PHONY: test-one
test-one: ## Run Go tests verbosely, uncached, with the race detector: PKG=./internal/core/... RUN=Regex
	@$(UNCONTROLLED)
	@$(HERMETIC) go test -race -count=1 -v -run '$(RUN)' $(PKG)

.PHONY: test-web-watch
test-web-watch: web-deps ## Run the web tests in watch mode
	@npm --prefix web exec -- vitest

.PHONY: cover
cover: ## Go coverage for PKG, as an HTML report at .dev/coverage.html
	@mkdir -p $(DEV_DIR)
	@$(HERMETIC) go test -count=1 -coverprofile=$(DEV_DIR)/coverage.out $(PKG)
	@go tool cover -html=$(DEV_DIR)/coverage.out -o $(DEV_DIR)/coverage.html
	@echo "cover: $(DEV_DIR)/coverage.html"

# ---------------------------------------------------------------------------------------------
# Setting up a machine
# ---------------------------------------------------------------------------------------------

SQLC_VERSION := 1.31.1

.PHONY: tools
tools: ## Install the pinned gate tools into $(go env GOPATH)/bin
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$(GOLANGCI_LINT_VERSION)

.PHONY: tools-sqlc
tools-sqlc: ## Install the pinned sqlc (needs cgo and a C compiler); only `make generate` uses it
	@CGO_ENABLED=1 go install github.com/sqlc-dev/sqlc/cmd/sqlc@v$(SQLC_VERSION)

.PHONY: tools-dlv
tools-dlv: ## Install the pinned delve debugger (make dev-debug uses it)
	@go install github.com/go-delve/delve/cmd/dlv@v$(DELVE_VERSION)

.PHONY: setup
setup: web-deps ## One-time setup of a clone: git hooks and web dependencies
	@git config core.hooksPath .githooks
	@echo "setup: git hooks now run from .githooks/"

.PHONY: clean
clean: ## Remove build output
	@rm -rf bin web/apps/*/dist
	@$(WEBCLIENT_CLEAR)
