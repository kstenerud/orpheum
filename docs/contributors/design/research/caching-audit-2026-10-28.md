> **ABOUTME:** Where the gates and tests redo work whose inputs did not change, measured and ranked,
> with a sound key for each cache proposed. A report for the owner: nearly every change lands in
> `Makefile`, `scripts/` and the vitest configurations, which are the owner's (AGENTS.md rule 2);
> the one exception is `.dockerignore` (finding 7).

# Caching audit: work redone though its inputs did not change

- **Date:** 2026-10-28.
- **Requested by** the owner, 2026-10-28: "run an audit to find any other wasted caching
  opportunities. Keeping testing run time down is very important."
- **Not repeated here:** what the lead is building tonight: a per-run scratch `TMPDIR` deleted at
  exit, size caps on the Go build cache, Node's compile cache and Playwright's browsers, a disk
  guard before `make check`, `check-pr` and `./staging`, `docker builder prune` after builds, the
  gate-script tests' leaks, and a shared cache of generated test media keyed by the generator's
  parameters and the pinned FFmpeg. Nor what [gate-throughput-2026-10-26.md](gate-throughput-2026-10-26.md)
  already proposed; where one of its items is still unbuilt and bears on caching, it is named.
- **Evidence:** the last `check-pr` (an earlier commit against an earlier commit, 42 commits, 2701 s, budget
  300 s; revert-red's code part 2355 s on 4 trees, 8579 s of probing) in
  `.git/gate-logs/check-pr-verbose.log`; the `check-pr-failed` after it
  (`.git/revert-red/last-timings`); the last `make check` log; the kept suite timings in
  `.dev/test-timings/`; the revert-red records in `.git/revert-red/`; and measurements in a fresh
  agent worktree on this sandbox's host.
- **Labels:** **[M]** measured here on 2026-10-28; **[R]** read in the code at an earlier commit;
  **[I]** inference or estimate, to confirm before building on it.
- **The rule kept**, as gate-throughput's: a run is skipped only when its result is already known
  for the same inputs in the same environment, keyed on everything that affects it, or when a
  real run decides the same verdict sooner. `make check-everything` stays the backstop to every
  record.

## Summary, ranked

Savings are wall time. "First" is the first full `check-pr` at a base; "re-run" is a later one at
the same base after a fix, which makefile.md §4 step 3 requires. Revert-red ran 4 probe trees, so a
second of probing is about 0.27 s of its wall time (2355 / 8579) **[M]**.

| # | Finding | Saving per check-pr | Per make check | Effort | Risk |
| --- | --- | --- | --- | --- | --- |
| 1 | Revert-red keeps no green record, so a re-run re-probes every green and claimed file | re-run: ~400–600 s (the last re-run: 602 s, all 9 files green again) | — | small | low |
| 2 | A probe tries none of the tests that turned the same file red before | first: ~500–1000 s **[I]** | — | medium | none |
| 3 | known-red keys a web red on the whole `web/` tree, a Go red on the named directories only | re-run after a web fix: up to ~1200 s | — | medium | lowers it |
| 4 | Go's test results are keyed by the exact flags and the cache; `make check`, `make spot` and revert-red never share one | first: part of #5 | up to the slowest touched package, 60–80 s, after a `make spot` | small | none |
| 5 | Revert-red's clean runs are untimed, per tree and kind, and kept by no run | first: ~300–450 s **[I]** | — | small–medium | low |
| 6 | The `-tags dev` probe run re-tests packages no `dev` file changes | ~100–150 s | — | small | none |
| 7 | `./staging` sends 1.8 GB of probe trees to Docker | — (each full `./staging`: ~2 GB of build cache, tens of seconds) | — | trivial | none |
| 8 | The browser-mode suite runs whole on any web change, serially at the end | — | up to ~50 s on a web turn | small | low |
| 9 | A new agent worktree starts with every stamp and lint cache cold | — | ~60–120 s on its first make check | small–medium | low |
| 10 | check-pr's pure steps keep no result by their inputs | re-run: 25–300 s | — | medium | key completeness; **owner's call** (makefile.md §4) |
| 11 | The jsdom suite has kept no timings since the projects split; make check keeps no step times | measurement | measurement | trivial | none |
| 12 | The dev image's tag hashes all of `go.mod` and the lockfile | — (minutes, per dependency change) | — | small | none |

Together, #2, #5 and #6 take roughly 900–1600 s off a first check-pr; #1, #3 and #10 most of a
re-run's. Caching alone does not bring check-pr to its 300 s: revert-red probing ~180 files, each
with several runs, is the structural cost, and its scope is the owner's.

---

## 1. Revert-red keeps no green record

**Evidence.** `revert_red_known.py` records a red only: "A green is never kept, so no file passes
on a record" **[R]**. The `check-pr-failed` after the last check-pr re-probed its 9 failures: every
one green again, 2230 s of probing, 602 s wall **[M]**. Seven of them were green with a
`Verified-By:` naming an unticked VQ item; the fix for those is to tick the item, a documentation
change, after which the full check-pr re-probes them once more (152–380 s each), only to reach
CLAIMED. The five Go files alone were 1438 s of that re-run's probing.

**Why it is safe.** A green record can only lead to a failure (no claim) or to CLAIMED (a valid
claim), and a fresh probe of a claimed file ends RED or CLAIMED: both pass. So the record never
passes a file a fresh probe would fail, provided its key holds what the probe read: adding the
missing test changes the test directories in the key, and the file is probed again.

**Proposal.** Record GREEN beside RED, under the same key (or #3's). `--trust-no-record`, the
nightly, ignores both.

## 2. A probe tries none of the tests that turned the same file red before

**Evidence.** Of this run's 145 reds, 88 files had a red recorded for the same path at an earlier
head (an earlier batch's revert of the same file) **[M]**; known-red cannot use them, as its key
holds the base's blob and the base moves with every passing check-pr. The slowest reds paid every
run before the red one: `film-volume.ts` 97 s of vitest related, 47 s of type check, then 92 s of
browser-mode related (red); `devices.go` 176 s of its package (red); `forget.go` 101 s of package
and 72 s of dependents (red) **[M]**.

**Proposal.** When a probe goes red, record the test that failed: the Go test's name (`--- FAIL:`
or the `-json` event) and package, the vitest file and project. A later probe of the same path, at
any base, first runs only those, in the same kind of run (`go test -run '^(TestX)$' pkg`,
`vitest run --project … file`). Red there settles it, as any red does; green, and the probe goes
on as today, a few seconds lost.

**Why it is safe.** The verdict always comes from a real run of the reverted code; the hint only
orders the runs. **Saving** [I]: with the 88 hits, ~40–80 s of probing each, 3500–7000 s, i.e.
~500–1000 s of wall time on a first check-pr; less if the earlier test seldom notices the new
revert, which the first runs will show.

## 3. known-red's key is too coarse for the web and too narrow for Go

**Evidence.** `dirs_read` keys a vitest or npm red on the git tree of `web/` as a whole, a Go red on
the directories its command names, the Python ones on `scripts` and `.claude/hooks` **[R]**. Of the
1206 records, 530 are keyed on `web` **[M]**. So any change under `web/` (a test fix, a stylesheet)
voids every web red at once: this run's 79 web reds, about half of all probing (the web kinds:
4473 s of 8579), ~1200 s of wall time, are all redone on a re-run after any web fix. The Go key is
the other way round: a package the red run imported but did not name may have changed, which
`revert_red_known.py`'s own docstring admits.

**Proposal.** Key each red (and #1's green) on what the run read: for Go, the trees of every
module directory in `go list -deps -test` of the packages run; for vitest, the blobs of the
failing (or, for a green, every run) test file's module closure, which vitest's graph gives, plus
the configuration and lockfile; for Python, the modules `python_tests_reaching` names. Narrower
for the web, wider and sound for Go.

## 4. Go's test results are never shared between the gate's runs

**Evidence** [M], one package run in turn: plain `go test` cached; `-json` the first time, run;
`-failfast` the first time, run; `-tags dev` over files no tag changes, run; `GOMAXPROCS=2`,
cached. The flags are part of the result's key. `make check` runs `go test -json`
(`Makefile:190`), `make spot` plain `go test` (`spot_check.py:139`), revert-red `-failfast -p N`
(`revert_red_commands.py`) **[R]**. And `make spot` runs on the host, whose Go cache is not the gate
container's volume (`dev:66-67`). So `make check` re-runs every package a `make spot` just passed
on the same tree, and the kept timings show what that costs when a package is not cached:
`internal/core/api` 78 s and `cmd/orpheum` 63 s on average, under the gate's load **[M]**.
Within one environment the cache works: 96 of 99 packages were `(cached)` in the last make check,
the three others reading every source by design (`archtest`, `storesources`) or changed (`store`;
it re-ran in 18 of the 20 kept passes, from before F1 put every gate in one container) **[M]**.

**Proposal.** One flag set and one cache: `make spot` runs `go test -json` (through
`go_test_timings.py`, which prints the plain text) with the gate's environment, in the gate's
container or with the host pointed at the same `GOCACHE`; revert-red's clean runs (#5) use the
same flags, so a clean Go run at HEAD is served from make check's results. The host's Go and the
image's are the same 1.26.10 **[M]**, and the cache keys on the toolchain anyway.

## 5. Revert-red's clean runs are untimed and kept by no run

**Evidence.** A red counts only once the same command passes in that tree with nothing reverted
(`revert_red_clean.py`, A45), once per tree and kind, in memory **[R]**: up to 4 trees × ~11 kinds
per run, each as heavy as the probe's run (vitest related on a central file: ~95 s). It is not
wrapped in `timed()`, so the report's run kinds miss it: they sum to 6912 s of the 8579 s of
probing, a gap of ~1670 s, most of it these runs **[M]**/**[I]**.

**Proposal.** Time them (their own kind in the report). Serve the Go ones from #4's cache. Keep a
passed clean run on disk under the tree's head tree, the command and the lockfile, so
`check-pr-failed` and the full re-run do not repeat it. Or, the owner's call since it narrows
A45's check: one small canary test per tree and kind, seconds, which catches a broken
environment, the thing the check exists for ("not a test that fails at HEAD, which make check
refuses").

## 6. The `-tags dev` probe run re-tests packages no `dev` file changes

**Evidence.** `other_builds` runs `go test -tags dev` over the file's package and every dependent
(`check_revert_red.py:349-351`) **[R]**. Only `cmd/orpheum` has `//go:build dev` files (eight)
**[M]**; every other package compiles the same program under the tag, yet its result is not reused
(#4). It took 445 s in 5 runs of the first check-pr, and 556 s of the re-run's 2221 s (decide.go:
113 s after 138 s of the same dependents untagged) **[M]**.

**Proposal.** Run the tagged pass only over packages whose file lists differ under the tag, found
exactly by `go list -tags dev -deps -test` against the untagged list: today `cmd/orpheum` alone.
**Saving:** most of those seconds, ~100–150 s of wall time per check-pr.

## 7. `./staging` sends the probe trees to Docker

**Evidence.** `./staging`'s full build sends the checkout as its context (`staging:130`), keeping
`.git` so the binary records its commit (`.dockerignore`) **[R]**. But `.dev` and `.tools` there
match the root only, so the context holds `.git/revert-red/wt-*` (4 × ~400 MB without
`node_modules`, each with its own `.dev`: films, a 57 MB `orpheum-browser`), `.git/check-pr/tree`
(207 MB) and any agent worktree under `.claude/worktrees/`: `.git` without `node_modules` is 2.0
GB, of which the objects are 143 MB; the rest of the checkout is 124 MB **[M]**. Each full build
copies all of it (`Containerfile:168`, `COPY . .`) into a layer, and `.git` changes with every
commit, so each one is a new ~2 GB layer in the build cache.

**Proposal.** Add `.git/revert-red`, `.git/check-pr`, `.git/worktrees`, `.git/gate-logs`,
`.claude/worktrees`, `**/.dev` and `**/.tools` to `.dockerignore` (not a gate file). Separately,
the owner's call: the full build's `build` stage keeps a third Go and npm cache in BuildKit;
staging could take the binary from `./dev make build`'s warm caches as `./staging quick` does,
keeping the from-source stage for release.

## 8. The browser-mode suite runs whole on any web change

**Evidence.** `web-test-browser` is behind an area stamp on all of `web/` and runs after every
parallel step, never beside them (`Makefile:86`, `:231`) **[R]**: so it is on make check's critical
path, whole, on every turn that touched the web. Its kept passes grew from 4.6 s to 65.9 s over
the last 20 (34 files), `input-player.browser.test.tsx` alone ~60 s per browser **[M]**. The jsdom
suite already runs only what a change reaches (`web_tests.py`); revert-red already runs
`vitest related` in browser mode (`BROWSER_MODE_RELATED`) **[R]**.

**Proposal.** The same as `web_tests.py`, in the browser-mode configuration: `vitest related` with
the changes since the stamp, the whole suite on a manifest, lockfile, configuration or deletion.
**Saving:** most of the 66 s when a change does not reach `input-player`, little when it does. That
file itself belongs in test-weight.md: the suite grew fourteenfold in a day.

## 9. A new agent worktree starts cold

**Evidence.** The area stamps, the web-test stamp and the check stamp live at
`git rev-parse --git-path`, which in a worktree is `.git/worktrees/<name>/` **[M]**, so an agent's
first make check runs the whole jsdom suite (51–59 s), the whole browser-mode suite (66 s) and the
gate scripts' tests. Measured in a fresh worktree, cold then warm **[M]**: eslint 48 s (86 s CPU)
then 1.6 s; `tsc -b` 12.6 s then 0.13 s; prettier 9.4 s then 3.8 s; golangci-lint over
`./internal/core/...` 22 s then 0.9 s (its package key includes the files' paths). `npm ci` is
not the cost: 3 s from the npm cache.

**Proposal.** Keep the stamps in the git common directory: each names a git tree, content not
place, so a pass on a tree holds in any worktree of the same image (add the image's tag to the
record to keep it so). Seed a new worktree's `.tsbuild` from the main checkout as
`revert_red_trees.seed_type_builds` seeds the probe trees; give eslint a cache location that does
not depend on the directory, if its entries are relative (to check). **Saving:** ~60–120 s on each
agent's first make check, with two or three agents a batch.

## 10. check-pr's pure steps keep no result by their inputs

**Evidence.** Every full check-pr re-runs `web-lint-full` (72 s), `test-integration` (25 s,
`-count=1`), `test-browser` (157 s) and `test-acceptance-home` (46 s) **[M]**, whatever changed
since the same step passed at the same base. makefile.md §4 step 3: "A targeted green is never the
gate's: the steps it skipped passed on an older commit"; gate-throughput §2.9 left it there.

**For the owner.** A record keyed on a step's whole input is not a targeted green: it says the
inputs are the same, as Go's test cache says inside make check. `web-lint-full` is a function of
`web/` and the lockfile; the integration tier of its packages' `go list -deps` closure, the pinned
FFmpeg and MKVToolNix; the browser tier of nearly everything, so it would rarely hit. The commit
steps (`lint-commits`, `check-test-weakening`, `check-decision-status`) have no file inputs and
are never recorded. A re-run after a Go-only fix would skip `web-lint-full` (72 s); after a
web-only one the integration tier (25 s); after a VQ tick all four (~300 s).

## 11. Two timings are missing

- The jsdom suite's last kept pass is 2026-10-26T21:40Z; the projects split
  (`7bc7f476`, `vitest.config.ts`) landed at 22:27Z, and make check has run the suite since
  **[M]**. `SHARED` puts `reporters` inside each project, where vitest takes no reporter, so
  `VitestTimings` never runs. Moving it to the root `test` fixes it. Whether `fsModuleCache` takes
  effect inside a project is worth the same check (two runs, the transform share compared).
- `make check` keeps no step durations (gate-throughput G0, unbuilt). Without both, #4, #8 and #9
  cannot be measured after they are built.

## 12. The dev image's tag hashes all of `go.mod` and the lockfile

**Evidence.** `./dev` hashes `go.mod` and `web/package-lock.json` whole into the image's tag
(`dev:54-61`, `:96-102`); the Containerfile reads only `go.mod`'s toolchain line and playwright-core's URL and
integrity from them, and copies `go.mod` before the Go download (`Containerfile:78`) **[R]**. A
dependency bump rebuilds from the Go download on (Go, Node, Playwright's system packages) and
replaces the gate container.

**Proposal.** Hash, and copy into the build, only those lines. Rare, but minutes each.

## Smaller, and checked

- **check-pr runs `browsers` three times** and builds the client and `orpheum-browser` twice on one
  commit (`test-acceptance-home-here`, `test-browser`): ~5–10 s; gate-throughput §2.8 has the first.
- **A deleted web file runs the whole jsdom suite** (`web_tests.py`), 51–59 s: `tsc -b` already
  fails any TypeScript importer left, so related tests plus the files naming its basename suffice.
- **The gate scripts' tests run all 51 modules** on any change outside Go, TypeScript and docs;
  `python_tests_reaching` already narrows them for revert-red.
- **`storetest` migrates its template once per test process**; a template on disk keyed by
  `migrations/SHA256SUMS` would save seconds of CPU per uncached run. Small.
- **Space:** each tree's `.vitest-cache` is unbounded (main 140 MB, probe trees 121 and 103 MB);
  the host keeps a second golangci-lint cache (340 MB) and Playwright's browsers (964 MB) beside
  the container's volume. Add the first to the lead's caps; one location for the others.
- **Checked, nothing to gain:** `npm ci` (3 s), `go run ./cmd/orpheum openapi` in
  check-generated (1.8 s cold, 0.15 s warm), a warm `tsc -b` (0.13 s), check-pr's tree set-up
  (0 s, kept), the FFmpeg and MKVToolNix fetches (hash-pinned, baked into the image), the browser
  tier's films (film cache, test-weight.md §2), and Go's test cache inside make check.

## Order

1. #11 (the timings), so the rest can be measured; #7 (no gate file).
2. #4 and #6, small and with no risk to a verdict.
3. #1, #2 and #3 together in `revert_red_known.py`; #5.
4. #8 and #9.
5. #10 and #12, and #5's canary, after the owner decides.
