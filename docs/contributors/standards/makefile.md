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
# SPDX-License-Identifier: AGPL-3.0-or-later
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
(`scripts/check_stamp.py`), since the Stop hook has just run it. Within it, the web tests, the
web dependency rules and the gate scripts' tests run again only when what each reads changed
since it last passed (`scripts/area_stamp.py`): a Go-only turn does not re-run the web suite. A
change of documentation alone (`docs/` and Markdown, `scripts/documentation_only.py`) runs
`make check-documentation` instead, in both hooks, and check-pr skips its integration tier, its
browser tiers and its uncached lint for it: no test of the code.
Both hooks run it where the
tools are: directly in the development image or on a host with Go and Node, through `./dev`
otherwise (`scripts/run_gate.py`, D235). Its checks run in parallel after
two that must come first, the tools and the generated code, and a target added to it must not
write files another check reads. **Read its prerequisite lines in the Makefile; do not copy them
into a document**, where it would go stale the
next time a check is added. yoloAI's copy was stale three times out of four
([principles §8](../principles/08-factual-accuracy.md)).

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

### The fix-and-re-run loop

`make check-pr` runs its steps **in tiers, quickest first** (the owner, 2026-10-15): the second's
checks (test weakening, decision status, commit lint), then the medium ones in parallel (the
integration tier, the full web lint), **revert-red on the code** with its cheap runs alone (Go,
TypeScript, Python: a package's tests each), the acceptance under `home` alone, the browser tier,
and last **revert-red's browser runs**: the stylesheets', and those of the code files no cheap run
noticed (`BROWSER_OWED`), read against the browser map the browser tier has just recorded. No
expensive run holds back a cheap one (the owner, 2026-10-17; test-weight.md §14).

A branch of **documentation alone** (`scripts/documentation_only.py`) cannot change what the code
does, so `make check-pr` and `make check-pr-failed` hand it to `make check-pr-documentation`: what
reads the documents (links, citations, the architecture map) and the commits' quick checks, in
seconds, marking HEAD checked as check-pr does (the owner, 2026-10-17).

**Every night, CI runs `make check-everything`** on main: the backstop to all of check-pr's
narrowing. It clears the stamps and runs every tier whole and uncached, plus revert-red over the
day's commits with every browser run in turn, each step to its end. Run it by hand with
`SINCE="3 days ago"` to cover more commits (the owner, 2026-10-17).

check-pr is the last run of a block of work, which is expected to pass, since its pieces' tests
have run already: **every tier runs to its end**, a failed one too, so one run shows every
failure, and check-pr fails at the end. The piecework runs fail fast: `make check-pr-failed` stops
at a failed tier, and each revert-red probe's browser run stops at its first failing test (the
owner, 2026-10-17: "fail fast on piecework… run to completion on the final test run";
[principles §24](../principles/24-cheap-checks-first.md)).

After each commit, `make check-commit-red` runs revert-red on that commit's code against its
parent, keeping no record: a minute's check, while the change is fresh, so check-pr at a batch's
end confirms rather than discovers. It takes no browser run: it lists the files that owe one, which
check-pr's last tier probes. The slow tiers never share the machine. A passing run is no shorter
for it; the tiers' order is in the Makefile's `PR_FAST_STEPS`, `PR_MEDIUM_STEPS` and `PR_SLOW_STEPS`.
The acceptance under `home` runs only in a batch that changed what it crosses: Watch Together, voice,
the WebRTC transport and the path to the core, the player, playback's chunks and decisions
(`scripts/browser_tier_needed.py --acceptance`). The owner's standing rule (2026-10-15): a batch that
changes those areas is tested so, and one that does not is spared its minutes.

`make check-pr` has a budget of **five minutes** (`CHECK_PR_BUDGET_S`, the owner's): its one line
on a pass says how long it took, and says so when it took longer. A step or test added to it
states what it adds, and one that would take it past the budget goes to the milestone's runs
instead (agent-failures A24: it doubled unnoticed). Most of its minutes are in steps that passed.
While fixing a failure, re-run only what failed, then run the whole gate once:

1. **Loop on the narrowest run that shows the failure**: `go test -run <Test> ./<pkg>/`,
   `npm --prefix web test -- <file>`, or revert-red on chosen files with
   `make check-revert-red BASE=<base> ONLY="<path> <path>"`. `ONLY` still judges each path
   against the base, and says why it skips one (exempt by commit type, test code, unchanged).
2. **`make check-pr-failed`** re-runs the check-pr steps that failed at the last `make check-pr`
   against the same base, or never ran, and names the ones it skipped with the commit they passed
   on. Revert-red re-runs only the files it failed on (`make check-revert-red-failed`). The head
   may have moved, since the fixes are new commits; another base is refused.
3. **One full `make check-pr`** when the targeted runs pass, before the branch is done. A
   targeted green is never the gate's: the steps it skipped passed on an older commit. A pure
   step (web-lint-full, the integration tier, the browser tier) whose files, base and toolchain
   are the same as at a pass it kept is not run again, even here: that is not a targeted green but
   the same inputs, as Go's test cache is inside make check (`check_pr_steps.STEP_INPUTS`; the
   owner, 2026-10-28). The commit steps always run; `make check-everything` trusts no kept pass.

The records the re-runs read live in the git common directory (`revert-red/last-failures`,
`check-pr/last-results`; `scripts/last_run.py`). No hook reads them, and nothing the loop runs
writes the check stamp.

**Enforced by:** `make check` itself and `check-tools`; the budget is **review-only**. The
re-runs by `scripts/tests/test_gate_reruns.py`; running the full `make check-pr` last is
**review-only**.

## 5. Tools are pinned, and a derived list beats a written one

- **Every tool's version is pinned in one place.** Go's and Node's are variables at the top of
  the Makefile; golangci-lint's and sqlc's are `scripts/fetch_tools.py`'s, a version and one
  SHA-256 per platform for each project's own release binary, as FFmpeg is pinned ([D235](../decisions/working-notes.md#d235--development-and-deployment-run-in-containers-one-image-family-one-command-each-built-by-goreleaser)
  §2). `check-tools`, the first prerequisite of `make check`, fetches those two into `.tools/`
  when they are missing and fails when another tool is missing or at another version, naming the
  command that fixes it. Changing a pin is an owner action, because a linter upgrade changes what
  the gate means.
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
