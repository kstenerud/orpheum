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

# Pinned tool versions. Changing one is an owner action (the pre-edit hook asks), because a
# linter upgrade changes what the gate means.
GOLANGCI_LINT_VERSION := 2.13.2
GO_MINOR := 1.26
NODE_MAJOR := 22

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BASE ?= origin/main
PYTHON := python3

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
go-lint: ## golangci-lint with the project's configuration
	@golangci-lint run ./...

.PHONY: go-vet
go-vet: ## go vet for the shipped platform, and for macOS so a developer's machine still builds
	@go vet ./...
	@GOOS=darwin GOARCH=arm64 go vet ./...

.PHONY: go-test
go-test: ## Unit and architecture tests
	@$(HERMETIC) go test ./...

.PHONY: go-tidy
go-tidy: ## Fail if go.mod or go.sum is not tidy
	@go mod tidy -diff

.PHONY: go-speculative
go-speculative: ## Fail on declarations whose only callers are tests
	@$(PYTHON) scripts/lint_speculative_api.py

.PHONY: check-generated
check-generated: ## Fail if committed generated code differs from what its source generates
	@if ls internal/store/queries/*.sql >/dev/null 2>&1; then \
	  $(MAKE) generate && git diff --exit-code -- internal/store/sqlcgen || \
	  { echo "generated code is stale or hand-edited: run make generate and commit the result"; exit 1; }; fi

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
check-pr: check-revert-red check-test-weakening lint-commits ## Branch gates against BASE (default origin/main)

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
	@$(HERMETIC) go test -race -count=1 ./...

# ---------------------------------------------------------------------------------------------
# Building and generating
# ---------------------------------------------------------------------------------------------

.PHONY: build
build: web-deps ## Build the server binary into bin/ and the web apps into their dist/
	@npm --prefix web run --silent build
	@go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/orpheum ./cmd/orpheum

.PHONY: generate
generate: ## Regenerate committed generated code (sqlc; the OpenAPI document and TS types once the API exists)
	@if ls internal/store/queries/*.sql >/dev/null 2>&1; then \
	  command -v sqlc >/dev/null || { echo "sqlc is not installed; run: make tools-sqlc"; exit 1; }; \
	  sqlc generate && sqlc vet; \
	else echo "generate: no queries yet (internal/store/queries/*.sql); nothing to do"; fi

.PHONY: fmt
fmt: ## Format Go and web sources in place
	@golangci-lint fmt ./...
	@npm --prefix web run --silent format

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

.PHONY: setup
setup: web-deps ## One-time setup of a clone: git hooks and web dependencies
	@git config core.hooksPath .githooks
	@echo "setup: git hooks now run from .githooks/"

.PHONY: clean
clean: ## Remove build output
	@rm -rf bin web/apps/*/dist
