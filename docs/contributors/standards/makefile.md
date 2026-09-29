> **ABOUTME:** How the one Makefile is written — targets as the only interface to building and
> checking, the shape `make check` must keep, tool pinning, derived file lists, and the recipe
> discipline that keeps it readable. Each rule names what enforces it.

# Makefile Standard

The Makefile is how a developer, CI, the lab and an agent's Stop hook all run the same checks.
If they ran different commands, "passes locally" would say nothing about CI. It orchestrates;
it holds no logic worth testing — that goes into a script under `scripts/` or a Go program.

## 1. One Makefile, and it is the interface

- **One `Makefile` at the repository root**, GNU Make. No recursive Make, no sub-Makefiles, no
  second task runner. The web workspace's npm scripts exist, and the Makefile calls them;
  nothing calls them directly in CI.
- **Everything anyone needs to run has a target**: `make check`, `make check-pr`,
  `make test-all`, `make generate`, `make fmt`, `make build`, `make tools`. Documents,
  `AGENTS.md`, CI and hooks name targets, never raw `go test` or `npm` invocations.
- **CI calls the same targets.** The gates that need a base revision to compare against —
  revert-red, the test-weakening guard, commit messages, and later the OpenAPI diff
  ([D85](../decisions/working-notes.md)) — sit behind `make check-pr`, which anyone can run with
  `BASE=<rev>`. No gate runs on pull requests only (yoloAI DF226: a release branch reached 63
  commits with no CI).

**Enforced by:** **review-only**; the pull-request-only rule gets a test that parses
`.github/workflows/` when the first workflow with a `pull_request` filter appears.

## 2. The preamble

The file opens with its licence line and ABOUTME, then settings that turn Make's silent
failure modes into loud ones:

```make
# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: The one entry point for building, generating and checking Orpheum; CI and the
# ABOUTME: agent Stop hook run these same targets, so local and CI results mean the same thing.

SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help
.DELETE_ON_ERROR:
MAKEFLAGS += --no-print-directory --warn-undefined-variables --no-builtin-rules
```

- `-eu -o pipefail` makes a failing command anywhere in a recipe line, including inside a pipe,
  fail the target.
- `.DELETE_ON_ERROR` removes a half-written output, so the next run does not treat it as
  up to date.
- `--warn-undefined-variables` turns a typo in a variable name into a message rather than an
  empty string.
- `make` with no target prints the help, so nothing runs by accident.

**Enforced by:** `scripts/check_source.py` requires the SPDX line; the rest is **review-only**.

## 3. Targets

- **Each target that is not a file is declared `.PHONY` on the line directly above it**, so
  adding or deleting a target touches one place.
- **Every public target carries a help comment** after `##` on its own line, which `make help`
  collects:

  ```make
  .PHONY: check-docs
  check-docs: ## Links, anchors, decision citations and the decision index
  	@$(PYTHON) scripts/check_docs.py
  ```

  A helper target that nobody should run directly has no `##` comment, and so no help line.

- **Lowercase kebab-case names** that say what running it does, grouped by prefix:
  `check-docs`, `go-lint`, `check-revert-red` — never an abbreviation like `gc`.
- **A target does one thing** and composes others through prerequisites. `test-all` lists
  `check` and the slow tiers; it does not copy their recipes.

**Enforced by:** **review-only**. A `make help` that omits a target is visible to anyone who
runs it.

## 4. The `make check` contract

`make check` is the fast gate. The Stop hook, the pre-commit hook and CI all run it; the
pre-commit hook skips a commit of exactly the tree the gate last passed on
(`scripts/check_stamp.py`), since the Stop hook has just run it. Its checks run in parallel after
two that must come first, the tools and the generated code, and a target added to it must not
write files another check reads. **Read its prerequisite lines in the Makefile; do not copy them
into a document**, where it would go stale the
next time a check is added. yoloAI's copy was stale three times out of four
([principles §8](../principles/README.md#8--factual-accuracy-in-documentation)).

A check belongs in `make check` when **all** of these hold:

- it catches a defect that would otherwise ship;
- it needs no network, no daemon, no media, no FFmpeg and no device;
- it keeps the whole target near its **90-second** budget, because every agent turn that
  touched code waits on it.

A check that needs a base revision goes in `make check-pr`; everything else goes in
`make test-all` or the lab runner
([testing.md](testing.md#10-what-runs-where)). The one exception is the integration tier, which
`make check-pr` runs first (the owner, 2026-09-27): the reference scale's test is in it, and two
queries that read whole tables reached `main` because only CI ran it.

- **A required tool that is missing fails the target.** A check never prints "skipping" and
  exits 0: *"a gate that can silently not run is not a gate"* (yoloAI's hadolint target had
  claimed a CI install step that never existed).
- **Tagged tests are compiled** (`go vet -tags integration ./...` and `go vet -tags dev ./...`)
  even though `make check` does not run them, so a deleted helper cannot hide in a file the default build skips.
- **Generated code is regenerated into place and compared**: `check-generated` records every
  generated file, runs `make generate`, and fails if any changed, appeared or disappeared, so
  sqlc output, the OpenAPI document and the TypeScript client cannot drift from their sources,
  whether or not the correct output is staged yet.

**Enforced by:** `make check` itself and `check-tools`; the budget is **review-only**.

## 5. Tools are pinned, and a derived list beats a written one

- **Every tool's version is pinned in one place**, a variable at the top of the Makefile
  (`GOLANGCI_LINT_VERSION`, the Go and Node versions), and `check-tools` — the first
  prerequisite of `make check` — fails when a tool is missing or at another version, naming the
  command that fixes it. `make tools` installs the pinned versions. Changing a pin is an owner
  action, because a linter upgrade changes what the gate means.
- **Node tools** come from the web workspace's lockfile.
- **System tools** are checked the same way: shellcheck by `check-tools`. The integration tier's
  FFmpeg and MKVToolNix are pinned and fetched by `make ffmpeg` and `make mkvtoolnix`, each
  download verified by its hash, locally and in CI; FFmpeg's version is pinned because what the
  tests observe of it depends on it. ([D111](../decisions/working-notes.md)'s `assumptions/`
  directory, which would test the pinned version's behaviour, does not exist yet.)
- **File lists are derived**, from `git ls-files --cached --others --exclude-standard` or Go's
  package patterns, never typed out. yoloAI's hand-written type-check list missed five scripts
  that then shipped for months with no check while its standard claimed they were covered.

**Enforced by:** `check-tools` for the pins; `go mod tidy -diff` in `make check`; the pre-edit
hook refuses an agent's edit to the Makefile. Derived lists are **review-only**.

## 6. Test targets set their environment

- Every test target exports the hermetic environment: `TZ=UTC`, `LC_ALL=C.UTF-8`,
  `GIT_CONFIG_GLOBAL=/dev/null`, `GIT_CONFIG_NOSYSTEM=1`. A test that passes only with the
  developer's own git identity or time zone then fails locally first. yoloAI's DF230 sat red in
  CI for eight days because every developer machine supplied the identity CI lacked.
- **`ORPHEUM_TEST_*` variables are set here, in CI workflows, or by the lab runner — nowhere
  else.** The liveness check fails on a variable the code reads that none of them sets
  ([testing.md](testing.md#3-a-missing-capability-fails-only-a-named-list-may-be-absent)).
- `ORPHEUM_TEST_UNCONTROLLED` is **never** set in the Makefile. It belongs to a CI step or lab
  job that genuinely cannot provide a capability.

**Enforced by:** `scripts/check_claims.py` for the setters; the hermetic exports are
**review-only**.

## 7. Recipes

- **One command per line.** Chain with `&&` only when the commands must succeed or fail
  together.
- **`@` only on `echo`** and on commands whose own output is the point. A recipe's commands are
  visible by default so a failure shows what ran.
- **Long logic moves to a script.** A recipe past a few lines, or with a loop or a conditional
  beyond a tool check, becomes `scripts/<name>.sh` or `.py` ([shell.md](shell.md),
  [python.md](python.md)) and is linted and tested there.
- **Variables:** `UPPER_CASE`; `:=` by default; `?=` only for what a user may override
  (`VERSION ?= dev`); recursive `=` only when late expansion is the point.
- **Tabs** indent recipes, as Make requires and `.editorconfig` sets.

**Enforced by:** **review-only**; shellcheck does not read Makefiles.
