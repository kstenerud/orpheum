# SPDX-License-Identifier: AGPL-3.0-or-later
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

# Pinned tool versions. Changing one is an owner action (the pre-edit hook asks). golangci-lint's
# and sqlc's pins are in scripts/fetch_tools.py, which the development image is built from.
GO_MINOR := 1.26
NODE_MAJOR := 22

BASE ?=
PYTHON := python3

# The pinned golangci-lint and sqlc (scripts/fetch_tools.py links them into TOOLS_BIN) come first
# on every target's PATH, before any other copy the machine has. GOPATH/bin holds what `go install`
# put there (dlv); a fresh container's PATH often lacks it.
TOOLS_BIN := $(or $(ORPHEUM_TOOLS_DIR),$(CURDIR)/.tools)/bin
export PATH := $(TOOLS_BIN):$(shell go env GOPATH)/bin:$(PATH)

.PHONY: help
help: ## List the targets
	@grep -E '^[a-z][a-z-]*:.*## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*## "} {printf "  %-22s %s\n", $$1, $$2}'

# ---------------------------------------------------------------------------------------------
# The gate
# ---------------------------------------------------------------------------------------------

# The gate's checks run in parallel after two that must come first: the tools are there, and the
# generated code is current (check-generated rewrites generated files the builds then read).
CHECK_JOBS ?= $(shell nproc 2>/dev/null || echo 4)

.PHONY: check
check: ## The gate: everything that runs on every change, quietly (VERBOSE=1 prints all; the log is in .git/gate-logs)
	@$(PYTHON) scripts/quiet_gate.py --fail-fast "make check" check-verbose

# The spot check, while working: the tests of what changed and of what imports it, about 30 s; not
# the gate, which make check still is at the commit (D279, testing.md §10). BASE=<rev> checks commits.
.PHONY: spot
spot: ## Spot check while working: tests of what changed since BASE (default HEAD, so uncommitted) and its importers, ~30 s
	@$(PYTHON) scripts/spot_check.py $(if $(BASE),--base '$(BASE)') $(if $(SPOT_JOBS),--jobs $(SPOT_JOBS))

.PHONY: check-verbose
# web-deps runs before the parallel steps: check-generated and check-web both need it, and two
# installs at once raced each other (2026-10-01). It does nothing when the install is current.
check-verbose: check-tools web-deps go-warm ## The gate with every step's output
	@$(MAKE) --no-print-directory -j$(CHECK_JOBS) --output-sync=target check-parallel
	@$(PYTHON) scripts/check_stamp.py write
	@echo "make check: all gates passed"

.PHONY: check-parallel
# check-generated runs beside the rest: nothing reads what it generates, which goes to a temporary
# directory, and as a prerequisite it held every other step back by 7.6 s (2026-10-01).
check-parallel: check-generated check-docs check-scripts check-source check-deps check-claims check-architecture check-go check-web

.PHONY: check-tools
check-tools: ## Fetch the pinned golangci-lint and sqlc if missing; fail if another pinned tool is missing or at the wrong version
	@$(PYTHON) scripts/fetch_tools.py
	@command -v go >/dev/null || { echo "go is not installed (need $(GO_MINOR).x)"; exit 1; }
	@case "$$(go env GOVERSION)" in go$(GO_MINOR)|go$(GO_MINOR).*) ;; \
	  *) echo "go is $$(go env GOVERSION); this project pins $(GO_MINOR).x"; exit 1;; esac
	@command -v node >/dev/null || { echo "node is not installed (need $(NODE_MAJOR).x)"; exit 1; }
	@v="$$(node --version)"; case "$$v" in v$(NODE_MAJOR).*) ;; \
	  *) echo "node is $$v; this project pins $(NODE_MAJOR).x"; exit 1;; esac
	@command -v $(PYTHON) >/dev/null || { echo "python3 is not installed"; exit 1; }
	@command -v shellcheck >/dev/null || { echo "shellcheck is not installed"; exit 1; }

.PHONY: check-documentation
check-documentation: check-docs check-claims check-architecture ## The gate for a change of documentation alone (scripts/documentation_only.py): what reads the docs, and no test of the code
	@# The one Go test that reads a document, schema.md's owners table (the M4 closing audit).
	@$(HERMETIC) go test -run '^TestArch_EachTableHasOneWritingPackage$$' ./internal/archtest/

# Within check-pr (a BASE given), whether the branch changed documentation alone: then the tiers
# that test the code have nothing to find (the owner, 2026-10-08).
DOCUMENTATION_ONLY = [ -n '$(BASE)' ] && $(PYTHON) scripts/documentation_only.py --base '$(BASE)'

.PHONY: check-docs
check-docs: ## Links, anchors, decision citations and the decision index
	@$(PYTHON) scripts/check_docs.py

.PHONY: check-scripts
check-scripts: ## The gate scripts' own tests, and shellcheck on every shell script; not again while only Go, TypeScript or docs changed since they passed
	@$(PYTHON) scripts/area_stamp.py check-scripts $(SCRIPTS_READ) -- $(MAKE) --no-print-directory check-scripts-run

# What the gate scripts' tests read: all but the Go and TypeScript sources and the docs (scripts/
# area_stamp.py; the test audit, 2026-10-08).
SCRIPTS_READ = . ':(exclude,glob)**/*.go' ':(exclude,glob)**/*.ts' ':(exclude,glob)**/*.tsx' ':(exclude)docs'

.PHONY: check-scripts-run
check-scripts-run:
	@$(HERMETIC) $(PYTHON) scripts/py_tests.py
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

.PHONY: check-architecture
check-architecture: ## Every package has a row in the architecture map; schema.md's owners table lists every table
	@$(PYTHON) scripts/check_architecture_map.py

.PHONY: check-go
check-go: go-fmt go-lint go-vet go-test go-tidy go-speculative ## All Go checks

# go-warm compiles every package, test variants included, once, before lint, vet and the tests
# start beside each other: on a cold cache each of them compiled the same dependencies at once.
# On 4 CPUs with a cold cache, compiling first took the three from 232 s to 197 s; on a warm
# cache it costs under a second, since export data is cached and nothing is linked (2026-10-01).
.PHONY: go-warm
go-warm:
	@go list -export -deps -test ./... >/dev/null

.PHONY: go-fmt
go-fmt: ## Fail if gofumpt or gci would change a file (fix with: make fmt)
	@golangci-lint fmt --diff ./...

# golangci-lint holds one lock for every run on the machine, and refuses to start beside another
# run (make check's go-speculative, another worktree's gate, the Stop hook beside a commit's):
# --allow-serial-runners waits for the lock instead.
.PHONY: go-lint
go-lint: ## golangci-lint with the project's configuration, for release and dev builds and the integration tier
	@golangci-lint run --allow-serial-runners --build-tags integration ./...
	@golangci-lint run --allow-serial-runners --build-tags dev ./cmd/...

.PHONY: go-vet
go-vet: ## go vet for the shipped platform, for arm64 Linux, for macOS, for the dev build and for the integration tier
	@go vet ./...
	@GOOS=linux GOARCH=arm64 go vet ./...  # D205: arm64 stays open
	@GOOS=darwin GOARCH=arm64 go vet ./...
	@go vet -tags dev ./...
	@go vet -tags integration ./...

# The dev build's own tests are those in test files only it builds (`//go:build dev`): the rest of
# ./cmd ran in the release build just before, and running all of them again took 17 s of every
# make check (2026-10-06).
DEV_ONLY_TESTS = $(shell grep -l '^//go:build dev$$' cmd/*/*_test.go 2>/dev/null | xargs -r grep -ho '^func Test[A-Za-z0-9_]*' | sed 's/^func //' | paste -sd'|' -)

.PHONY: go-test
go-test: ## Unit and architecture tests, for the release build and the dev build's own
	@$(UNCONTROLLED)
	@$(HERMETIC) go test ./...
	@test -n '$(DEV_ONLY_TESTS)' || { echo "go-test: no test file builds only with -tags dev; the dev pass would test nothing"; exit 1; }
	@$(HERMETIC) go test -tags dev -run '^($(DEV_ONLY_TESTS))$$' ./cmd/...

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
check-web: web-deps ## Typecheck, lint, format, dependency rules and tests for web/, at once
	@$(MAKE) --no-print-directory -j5 --output-sync=target web-typecheck web-lint web-format-check web-deps-check web-test

# The web checks, each one of web/package.json's scripts. check-web runs them at once: one after
# another they took 16 s, the longest step of make check (2026-10-01).
.PHONY: web-typecheck web-lint web-format-check web-deps-check web-test
web-typecheck:
	@npm --prefix web run --silent typecheck
web-lint:
	@npm --prefix web run --silent lint
web-format-check:
	@npm --prefix web run --silent format:check
# The two not again while nothing under web/ changed since they passed (scripts/area_stamp.py).
web-deps-check:
	@$(PYTHON) scripts/area_stamp.py web-deps-check web -- npm --prefix web run --silent deps:check
web-test:
	@$(PYTHON) scripts/area_stamp.py web-test web -- npm --prefix web run --silent test

.PHONY: web-deps
web-deps: ## Install web dependencies from the lockfile when they are missing or stale
	@# npm ci empties node_modules first, through a symlink too: a scratch worktree whose
	@# node_modules pointed at the main tree's emptied it twice (2026-09-30, 2026-10-01).
	@if [ -L web/node_modules ]; then echo "web-deps: web/node_modules is a symlink; npm ci would empty what it points at. Remove it and run make web-deps here."; exit 1; fi
	@if [ ! -f web/node_modules/.package-lock.json ] || [ web/package-lock.json -nt web/node_modules/.package-lock.json ]; then \
	  npm --prefix web ci --no-audit --no-fund; fi

# ---------------------------------------------------------------------------------------------
# Branch gates: they compare a branch against its base, so they run in CI on a pull request and
# locally with `make check-pr` before pushing. BASE names the base; without it the gates use the
# commit the owner last took (`make mark-applied`), then origin/main, and refuse one too far back
# to mean anything (scripts/branch_base.py).
# ---------------------------------------------------------------------------------------------

RESOLVE_BASE = $(PYTHON) scripts/branch_base.py $(if $(BASE),--base '$(BASE)')

# check-pr's steps. Each runs through pr-step-<step>, which records whether it passed and on what
# commit (scripts/check_pr_steps.py), so `make check-pr-failed` can re-run only what failed.
PR_STEPS := test-integration test-browser check-test-weakening check-decision-status lint-commits web-lint-full
# Revert-red runs after the tiers, never beside them: a timing test failing under the load of a
# probe would be counted as the probe's red, a "covered" nothing proved (audit-2026-09-29 §6).
PR_LAST_STEP := check-revert-red
# M4's acceptance under home, a few minutes (D252 §2, D261 §2): after the tiers and before
# revert-red, alone, as it shapes the container's loopback, which every tier's tests cross.
# check-pr-failed does not re-run it: `make test-acceptance-home BASE=...` does.
PR_SERIAL_STEP := test-acceptance-home
# How long check-pr may take, in seconds: the owner's five minutes (agent-failures A24). A pass
# over it says so in its one line.
CHECK_PR_BUDGET_S := 300

.PHONY: check-pr
check-pr: ## Branch gates against BASE, quietly (VERBOSE=1 prints all; the log is in .git/gate-logs)
	@$(PYTHON) scripts/quiet_gate.py "make check-pr" check-pr-verbose $(if $(BASE),BASE=$(BASE))

.PHONY: check-pr-verbose
check-pr-verbose: ## Branch gates against BASE (default: where the last pass ended, or the owner's mark), the tiers in parallel, then revert-red; a pass marks HEAD as checked
	@started="$$(date +%s)"; base="$$($(RESOLVE_BASE))"; head="$$(git rev-parse HEAD)"; \
	  state="$$(git rev-parse --git-common-dir)/check-pr/tree-state"; mkdir -p "$$(dirname "$$state")"; \
	  $(PYTHON) scripts/tree_state.py --save "$$state"; \
	  $(PYTHON) scripts/check_pr_steps.py start --base "$$base"; \
	  $(MAKE) --no-print-directory -j$(CHECK_JOBS) -k --output-sync=target BASE="$$base" \
	    $(PR_STEPS:%=pr-step-%); \
	  $(MAKE) --no-print-directory BASE="$$base" $(PR_SERIAL_STEP:%=pr-step-%); \
	  $(MAKE) --no-print-directory BASE="$$base" $(PR_LAST_STEP:%=pr-step-%); \
	  $(PYTHON) scripts/check_pr_steps.py durations $(PR_STEPS) $(PR_SERIAL_STEP) $(PR_LAST_STEP); \
	  $(PYTHON) scripts/tree_state.py --check "$$state"; \
	  $(PYTHON) scripts/branch_base.py --checked "$$head" --base "$$base" && { \
	    took=$$(( $$(date +%s) - started )); \
	    if [ "$$took" -gt $(CHECK_PR_BUDGET_S) ]; then \
	      echo "make check-pr: passed in $${took} s, over its $(CHECK_PR_BUDGET_S) s budget: find the step that grew (standards/makefile.md)"; \
	    else echo "make check-pr: passed in $${took} s"; fi; }

# The loop's re-run: never a pass of check-pr, which is still required before a branch is done.
.PHONY: check-pr-failed
check-pr-failed: ## Re-run only the check-pr steps that failed last time (revert-red: only its failed files); then run the full check-pr
	@base="$$($(RESOLVE_BASE))"; \
	  $(PYTHON) scripts/check_pr_steps.py rerun --base "$$base" --jobs $(CHECK_JOBS) --last $(PR_LAST_STEP) $(PR_STEPS)

pr-step-%:
	@$(PYTHON) scripts/check_pr_steps.py record $* -- $(MAKE) --no-print-directory BASE='$(BASE)' $*

.PHONY: mark-applied
mark-applied: ## After `yoloai apply`: mark REV (default HEAD) as what the owner took, the branch gates' base
	@$(PYTHON) scripts/branch_base.py --mark '$(or $(REV),HEAD)'

.PHONY: web-lint-full
web-lint-full: web-deps ## eslint without its cache: make check's cached lint cannot see a type another file changed; in check-pr, not for documentation alone
	@if $(DOCUMENTATION_ONLY); then echo "web-lint-full: skipped; documentation alone changed since $(BASE)"; \
	  else npm --prefix web run --silent lint:full; fi

.PHONY: check-revert-red
check-revert-red: ## Every behaviour change can turn a test red when reverted (D190); ONLY="a b" probes only those paths
	@base="$$($(RESOLVE_BASE))"; $(PYTHON) scripts/check_revert_red.py --base "$$base" $(if $(ONLY),--only $(ONLY))

.PHONY: check-revert-red-failed
check-revert-red-failed: ## Revert-red on only the files its last run against BASE failed on; then run the full check
	@base="$$($(RESOLVE_BASE))"; $(PYTHON) scripts/check_revert_red.py --base "$$base" --rerun-failed

.PHONY: check-test-weakening
check-test-weakening: ## No test got weaker, and no gate changed, without a trailer on a commit that touched it
	@base="$$($(RESOLVE_BASE))"; $(PYTHON) scripts/check_test_weakening.py --base "$$base"

.PHONY: check-decision-status
check-decision-status: ## No commit records a decision whose Status already says it is built
	@base="$$($(RESOLVE_BASE))"; $(PYTHON) scripts/check_docs.py --base "$$base"

.PHONY: lint-commits
lint-commits: ## Commit subjects, bodies and attribution trailers
	@base="$$($(RESOLVE_BASE))"; $(PYTHON) scripts/lint_commits.py --base "$$base"

# ---------------------------------------------------------------------------------------------
# Longer runs
# ---------------------------------------------------------------------------------------------

.PHONY: test-all
test-all: check ## The gate, then the race detector and uncached tests
	@$(UNCONTROLLED)
	@$(HERMETIC) go test -race -count=1 ./...

# The integration tier's tests are those in test files only it builds (`//go:build integration`):
# the rest are the unit tier's, which make check passed on the same code, and running all of them
# again, uncached, took 38 s of check-pr against 20 s for these alone (the test audit, 2026-10-08).
INTEGRATION_TEST_FILES = $(shell grep -rl --include='*_test.go' '^//go:build integration$$' cmd internal 2>/dev/null)
INTEGRATION_ONLY_TESTS = $(shell echo '$(INTEGRATION_TEST_FILES)' | xargs -r grep -ho '^func Test[A-Za-z0-9_]*' | sed 's/^func //' | paste -sd'|' -)
INTEGRATION_PACKAGES = $(sort $(patsubst %/,./%,$(dir $(INTEGRATION_TEST_FILES))))

.PHONY: test-integration
test-integration: ## The integration tier: tests that run the pinned FFmpeg and MKVToolNix (fetched as make ffmpeg and make mkvtoolnix do); in check-pr, not for documentation alone
	@if $(DOCUMENTATION_ONLY); then echo "test-integration: skipped; documentation alone changed since $(BASE)"; \
	  else $(MAKE) --no-print-directory test-integration-run; fi

.PHONY: test-integration-run
test-integration-run:
	@$(UNCONTROLLED)
	@test -n '$(INTEGRATION_ONLY_TESTS)' || { echo "test-integration: no test file builds only with -tags integration; the tier would test nothing"; exit 1; }
	@$(HERMETIC) ORPHEUM_TEST_FFMPEG_DIR="$(FFMPEG_DIR)" ORPHEUM_TEST_MKVTOOLNIX_DIR="$(MKVTOOLNIX_DIR)" \
		go test -tags integration -count=1 -run '^($(INTEGRATION_ONLY_TESTS))$$' $(INTEGRATION_PACKAGES)

.PHONY: test-browser
# Within check-pr (a BASE given), the tier is skipped when nothing it builds or runs changed: docs,
# the gates' scripts, tests alone (scripts/browser_tier_needed.py; the owner, 2026-10-06).
test-browser: browsers ## The browser tier: Chrome and Firefox against a server of its own, with the built client (D217 §4); in check-pr, only when what it builds changed
	@if [ -z '$(BASE)' ] || $(PYTHON) scripts/browser_tier_needed.py --base '$(BASE)'; then \
	  $(MAKE) --no-print-directory browser-tier; fi

# The browsers Playwright drives are fetched by Playwright itself, for its own version: the one
# web/package-lock.json pins, which PLAYWRIGHT_VERSION repeats so a lockfile bump is seen here
# (check-deps compares the two). Nothing else installed them, so a fresh machine or CI failed at
# the browser tier (audit-2026-09-29 §6). The development image holds their system libraries, so CI,
# which runs in it, needs no --with-deps. With the browsers already there, the install checks and
# does nothing.
PLAYWRIGHT_VERSION := 1.63.0
PLAYWRIGHT_INSTALL_FLAGS ?=

.PHONY: browsers
browsers: web-deps ## Install the pinned Playwright's Chromium and Firefox (outside the dev image: PLAYWRIGHT_INSTALL_FLAGS=--with-deps)
	@v="$$(npm --prefix web exec -- playwright --version)"; [ "$$v" = "Version $(PLAYWRIGHT_VERSION)" ] || \
	  { echo "playwright is $$v; this project pins $(PLAYWRIGHT_VERSION)"; exit 1; }
	@npm --prefix web exec -- playwright install $(PLAYWRIGHT_INSTALL_FLAGS) chromium firefox

# The browser tier without installing the web packages: revert-red runs it in a worktree whose
# packages are links to the working tree's, which an install would replace.
# M4's acceptance (D252): two browsers for ORPHEUM_ACCEPTANCE_MINUTES (30) under each of the four
# netem profiles, each with some four minutes of voice alone after: two and a half hours (2.4 h on
# 2026-10-08); at a milestone's end, never in check-pr. It shapes the dev
# container's loopback with tc, so it runs in the development image (./dev make test-acceptance).
.PHONY: test-acceptance
test-acceptance: browsers ## M4's acceptance: two browsers for 30 minutes under each netem profile (D252); at a milestone's end
	@$(MAKE) --no-print-directory browser-tier E2E_CONFIG=web/e2e/acceptance.config.ts

# The branch gate's short acceptance run (D252 §2, D261 §2): home alone, ACCEPTANCE_HOME_MINUTES
# long, its film with voice beside it only, measured for half a minute; in check-pr, only when what
# the browser tier builds changed. Voice alone is the full acceptance's: here it doubled check-pr's
# time (agent-failures A24).
ACCEPTANCE_HOME_MINUTES ?= 0.5

.PHONY: test-acceptance-home
test-acceptance-home: browsers ## M4's acceptance under the home profile for ACCEPTANCE_HOME_MINUTES (D252 §2); in check-pr, only when what it builds changed
	@if [ -z '$(BASE)' ] || $(PYTHON) scripts/browser_tier_needed.py --base '$(BASE)'; then \
	  ORPHEUM_ACCEPTANCE_MINUTES=$(ACCEPTANCE_HOME_MINUTES) ORPHEUM_ACCEPTANCE_PROFILES=home \
	    $(MAKE) --no-print-directory browser-tier E2E_CONFIG=web/e2e/acceptance.config.ts \
	      E2E_ARGS='--grep "stay in step"'; fi

E2E_CONFIG ?= web/e2e/playwright.config.ts
# Playwright's own arguments for one run, such as a --grep; none by default.
E2E_ARGS ?=

.PHONY: browser-tier
browser-tier:
	@npm --prefix web run --silent build
	@$(WEBCLIENT_CLEAR)
	@cp -r web/apps/web/dist/. $(WEBCLIENT_DIST)/
	@go build -tags dev -o $(DEV_DIR)/bin/orpheum-browser ./cmd/orpheum; status=$$?; $(WEBCLIENT_CLEAR); exit $$status
	@$(HERMETIC) ORPHEUM_TEST_BINARY="$(CURDIR)/$(DEV_DIR)/bin/orpheum-browser" ORPHEUM_TEST_FFMPEG_DIR="$(FFMPEG_DIR)" \
		npm --prefix web exec -- playwright test --config $(E2E_CONFIG) $(E2E_ARGS)

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
	@go build -trimpath -ldflags "-s -w" -o bin/orpheum ./cmd/orpheum
	@$(WEBCLIENT_CLEAR)

.PHONY: generate
generate: web-deps ## Regenerate committed generated code (sqlc for persistent.db and cache.db, the OpenAPI document, and the SDK's TS types)
	@if ls internal/store/queries/*.sql >/dev/null 2>&1; then \
	  $(PYTHON) scripts/fetch_tools.py; \
	  sqlc generate && sqlc vet; \
	else echo "generate: no queries yet (internal/store/queries/*.sql); nothing to do"; fi
	@# Made in the system's temporary directory and removed however the run ends: made beside
	@# api/openapi.json, a make check stopped mid-step left api/openapi.json.XXXXXX in the tree
	@# (2026-10-08). Copied over only when it differs, so an unchanged document is not touched.
	@openapi_next=$$(mktemp "$${TMPDIR:-/tmp}/openapi.json.XXXXXX") && trap 'rm -f "$$openapi_next"' EXIT INT TERM && \
	  go run ./cmd/orpheum openapi > "$$openapi_next" && \
	  { cmp -s "$$openapi_next" api/openapi.json || cp "$$openapi_next" api/openapi.json; }
	@npm --prefix web exec --workspace @orpheum/sdk -- openapi-typescript ../../../api/openapi.json \
	  --output src/gen/api.ts --alphabetize --empty-objects-unknown >/dev/null

.PHONY: fmt
fmt: ## Format Go and web sources in place
	@golangci-lint fmt ./...
	@npm --prefix web run --silent format

# ---------------------------------------------------------------------------------------------
# Development and debugging. Opinionated on purpose: every choice is printed by `make dev` and
# written in deploy/dev/core.dev.toml, deploy/dev/node.dev.toml and deploy/dev/seed.toml, never
# hidden (D183). The core and the node are instances of their own, .dev/core and .dev/node (D242). All state lives in
# .dev/, which is disposable.
# ---------------------------------------------------------------------------------------------

DEV_DIR := .dev
DEV_CORE := $(DEV_DIR)/core
DEV_LISTEN ?=
FFMPEG_DIR = $(shell $(PYTHON) scripts/fetch_ffmpeg.py)
MKVTOOLNIX_DIR = $(shell $(PYTHON) scripts/fetch_mkvtoolnix.py)
PKG ?= ./...
RUN ?= .
FILE ?=
# Delve publishes no release binaries, so `make dev-debug` installs it with `go install` (checked
# against go.sum's checksum database) the first time it runs, rather than every image building it.
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
dev-config: ## Make the dev instances (.dev/core, .dev/node) and print their full configs — the settings dev runs with
	@$(PYTHON) scripts/dev.py config

.PHONY: dev-media
dev-media: ## Generate the synthetic dev library in .dev/media (FORCE=1 regenerates it)
	@$(PYTHON) scripts/dev_media.py --ffmpeg-dir "$(FFMPEG_DIR)" --dest $(DEV_DIR)/media $(if $(FORCE),--force)

.PHONY: dev-seed
dev-seed: ## Seed the dev database from deploy/dev/seed.toml, even if it is not empty
	@$(PYTHON) scripts/dev.py seed

.PHONY: dev-reset
dev-reset: ## Delete the dev database and caches (keeps the media and FFmpeg); next `make dev` re-seeds
	@rm -rf $(DEV_DIR)/core $(DEV_DIR)/node $(DEV_DIR)/orpheum.log
	@echo "dev-reset: removed the instances .dev/core and .dev/node, and the log"

.PHONY: dev-clean
dev-clean: ## Delete everything under .dev/, including the generated media
	@rm -rf $(DEV_DIR)

.PHONY: dev-logs
dev-logs: ## Follow the dev server's log
	@touch $(DEV_DIR)/orpheum.log && tail -F $(DEV_DIR)/orpheum.log

.PHONY: dev-db
dev-db: ## Open the dev database in an SQL shell (python3 -m sqlite3; stop the server before writing)
	@test -f $(DEV_CORE)/persistent/persistent.db || { echo "no dev database yet: run make dev"; exit 1; }
	@$(PYTHON) -m sqlite3 $(DEV_CORE)/persistent/persistent.db

.PHONY: dev-doctor
dev-doctor: ## Run `orpheum doctor` on the dev core's instance (.dev/core)
	@go build -tags dev -o $(DEV_DIR)/bin/orpheum ./cmd/orpheum
	@$(DEV_DIR)/bin/orpheum doctor --instance $(DEV_CORE)

.PHONY: dev-debug
dev-debug: ## Run the dev server under delve, headless on 127.0.0.1:2345, for an IDE to attach
	@dlv version 2>/dev/null | grep -qx "Version: $(DELVE_VERSION)" || go install github.com/go-delve/delve/cmd/dlv@v$(DELVE_VERSION)
	@$(PYTHON) scripts/dev.py config >/dev/null
	@dlv debug ./cmd/orpheum --build-flags="-tags=dev" --headless --listen=127.0.0.1:2345 \
	  --api-version=2 --accept-multiclient -- core --instance $(DEV_CORE)

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

.PHONY: tools
tools: ## Fetch the pinned golangci-lint and sqlc into .tools/ (make check does it too)
	@$(PYTHON) scripts/fetch_tools.py

.PHONY: setup
setup: web-deps ## One-time setup of a clone: git hooks and web dependencies
	@git config core.hooksPath .githooks
	@echo "setup: git hooks now run from .githooks/"

.PHONY: clean
clean: ## Remove build output
	@rm -rf bin web/apps/*/dist
	@$(WEBCLIENT_CLEAR)
