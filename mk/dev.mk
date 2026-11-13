# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: The Makefile's development and debugging targets, included where they stood in it: one
# ABOUTME: concern, out of the Makefile once it passed check_source's 800 lines (2026-11-13).

# ---------------------------------------------------------------------------------------------
# Development and debugging. Opinionated on purpose: every choice is printed by `make dev` and
# written in deploy/dev/core.dev.toml, deploy/dev/node.dev.toml and deploy/dev/seed.toml, never
# hidden (D183). The core and the node are instances of their own, .dev/core and .dev/node (D242). All state lives in
# .dev/, which is disposable.
# ---------------------------------------------------------------------------------------------

DEV_DIR := .dev
# The browser map the browser tier records and revert-red reads (scripts/browser_map.py).
BROWSER_MAP_DIR := $(DEV_DIR)/browser-map
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
ffmpeg: ## Fetch the pinned FFmpeg (ffmpeg-fdproxy on Linux) into .tools/ and verify its hash
	@echo "ffmpeg: $(FFMPEG_DIR)"

.PHONY: mkvtoolnix
mkvtoolnix: ## Fetch the pinned MKVToolNix, which the integration tier edits files with, into .tools/
	@echo "mkvtoolnix: $(MKVTOOLNIX_DIR)"

.PHONY: dev
dev: hooks web-deps ## Run the app: server + Vite (hot reload), seeded users and library, rebuilds on change
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
	@GOFLAGS= dlv debug ./cmd/orpheum --build-flags="-tags=dev" --headless --listen=127.0.0.1:2345 \
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
