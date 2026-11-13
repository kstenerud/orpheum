> **ABOUTME:** Where `make check`'s time went on 2026-11-12, measured from the Stop hook's run log
> and the kept test timings, and the ways to cut it, ranked by saving per cost. A proposal for the
> owner: every change to the gate itself lands in `Makefile`, `scripts/` or the vitest
> configurations, which are the owner's (AGENTS.md rule 2); options 1 and 4 change test code only.

# `make check`'s time: where it goes and what would cut it

- **Date:** 2026-11-12.
- **Requested by** the owner, the 11-24 batch's item X4: "Ways to cut `make check`'s time."
- **Builds on:** [gate-throughput-2026-10-26.md](gate-throughput-2026-10-26.md) (G0, B and §2.8
  are still unbuilt), [caching-audit-2026-10-28.md](caching-audit-2026-10-28.md) (finding 8, the
  incremental browser-mode run, is built), [test-regime-audit-2026-10-31.md](test-regime-audit-2026-10-31.md),
  principles §23 and §25, [makefile.md](../../standards/makefile.md) §4, D279, and
  [test-weight.md](../../test-weight.md) §26.
- **Labels:** **[M]** measured here; **[R]** read from the code; **[I]** inference, not measured.
- **Sources:** `.claude/.cache/gate-runs.jsonl` (each Stop-hook run: start, seconds including the
  lock wait, exit, load before and after); `.dev/test-timings/{go,go-dev,vitest,vitest-browser}/`
  (the last twenty runs of each suite, per package or file and per test); `.git/gate-holds.jsonl`
  (the test lock's holders); and one pass of each static step, timed alone in the gate container
  (`python3 scripts/in_dev_image.py <step>`), cold in a new worktree and then warm. No whole
  `make check` was run for this: the logs already held 28 of them.

## 1. The runs

**[M]** The Stop hook ran `make check` 28 times on 2026-11-12; 20 passed. The passing runs took 35,
43, 46, 47, 47, 49, 52, 91, 122, 123, 146, 189, 217, 268, 282, 354, 361, 365, 392 and 485 s: a median
of about 135 s, and 7 of 20 inside principles §23's 90 s. `make check-documentation`, run after
the 27 documentation-only turns, took 7 to 19 s.

**The lock queue is not where the time goes [M].** Of 4820 s in those runs, 195 s (4%) was the wait
for the test lock; two runs waited, 101 s and 84 s, each behind a spot check.

**Outside load is [M].** The same kind of run, a core change and a web change, took twice as long
at a load of 42 as at a load of 9: `internal/core/api` 244 s against 136 s, the jsdom suite 227 s
against 97 s. The gate alone drives the load to 8–15 on 8 vCPUs: three tools each sized to the
whole machine run at once (`go test`'s default `-p`, vitest's default workers, golangci-lint's
default concurrency) **[R]**.

### 1.1 One run, laid out

`make check` is a short prelude (tools, `npm ci` when needed, `go-warm`, the disk guard, the
stamp), a parallel phase at `-j8` (the static checks, `go-test`, and `check-web-parallel` with the
jsdom vitest run), then the browser-mode run alone, after everything else (`Makefile:251`,
`web-test-browser`; it runs alone since 2026-10-20, when Firefox failed to connect in vitest's 60 s
beside a cold Go build) **[R]**. Offsets are from the run's start **[M]**:

| Run (UTC) | Total | Prelude | `go test` (api) | jsdom vitest | Browser mode |
| --- | --- | --- | --- | --- | --- |
| 19:54 | 361 s | 101 s lock wait + 12 s | +113, 153 s (142) | +132, 97 s, 130 files | +270, 91 s, 46 files |
| 20:23 | 268 s | 25 s | +25, 150 s (136) | +38, 97 s, 55 files | +179, 88 s, 38 files |
| 21:10 | 485 s | 84 s lock wait + 31 s | +115, 262 s (244) | +132, 227 s, 212 files | +382, 103 s, 74 files |
| 21:58 | 392 s | 31 s | +31, 248 s (233) | +49, 217 s, 230 of 400 files | +286, 105 s, 92 of 106 files |
| 21:45 | 189 s | — | 156 s (142) | none | none |
| 23:22 | 91 s | — | 84 s (79) | none | none |
| 15:29 | 122 s | — | 19 s | none | +30, 89 s, 40 files |

- **The parallel phase ends when `go test` ends [M].** In the four runs with both, the Go run ended
  last: at +266, +175, +377 and +279 s, against the jsdom run's +229, +135, +358 and +266 s.
- **Then the browser-mode run, alone: 88 to 105 s whenever it runs 32 or more files [M]**, with
  nothing after it but the stamp (0–1 s).
- **The floor:** a run with the Go tests cached and no web change takes 35 to 52 s **[M]**.

So a turn that changed both core and web code waits about `prelude + max(go, jsdom) + browser`, and
the two terms worth cutting are the Go api package and the browser-mode tail.

### 1.2 `internal/core/api`: one package, two tests

**[M]** The api package is the longest Go package on every run that tests it, and it re-runs on
nearly any core change (it imports the assembly) **[R]**. It has 107 test files and 442 tests, 441 of
them `t.Parallel()`. Idle it took 79 s of wall time for 481 s of test time; under load, 136 s for
832 s. Its wall is about max(its longest test, its test time ÷ 8):

| Test | Idle | Under load |
| --- | --- | --- |
| `TestNeedsAttention_CountsASeriesUnplacedEpisodesPastFiveThousand` | 44.8 s | 81.3 s |
| `TestNeedsAttention_ListsAHundredAndCountsThemAll` | — | 63.2 s |
| the next longest | 11.4 s | — |

Both seed their rows one transaction at a time **[R]**: `ListsAHundredAndCountsThemAll` seeds 520
films, each through `librarytest.Title`, `Version`, `File` and `Stream`, each its own
`database.Write`; `PastFiveThousand` seeds 1001 titles the same way. The episode seeding beside it
(`seedEpisodeFiles`) already batches into one transaction. Spot checks ran the package for 50 to
111 s as well.

### 1.3 The browser-mode run: two files, in Firefox

**[M]**, from the run of 21:58 (106 files, 105 s wall), per file and browser:

| File | Firefox | Chromium |
| --- | --- | --- |
| `page-walk.browser.test.tsx` (28 tests) | 78 s (longest test 9.7 s) | 47 s |
| `input-pages.browser.test.tsx` (40 tests) | 64 s (6.4 s) | 44 s |
| `drop-down.browser.test.tsx` (21 tests) | 32 s | 6 s |

Over the last runs Firefox's page walk took 78 to 92 s and its input pages 64 to 78 s; Firefox's whole
test time is 264 to 341 s a run, Chromium's 183 to 203 s, at about five files at once. A file's
tests run one after another in one page, so **the run's wall is its longest file, Firefox's page
walk, plus the browsers' start [I]**: 88–105 s against 78–92 s. Below that bound sits the total work
over vitest's page budget: `maxWorkers`, shared by the two browsers' instances
(`createBrowserPool` in `vitest/dist/chunks/index.*.js`) **[R]**, and unset here, so sized to the
machine, taken as 8 **[I]**: (450–540 s) ÷ 8 ≈ 56–68 s.

### 1.4 The static steps

**[M]**, each alone in the gate container at a load of 14 to 28 (other agents' work), first cold in
this new worktree, then again at once, warm. The cold pass called the steps directly, so it ran
without `seed_worktree.py`, which gives a new worktree's first `make check` the main checkout's
`.tsbuild` (the caching audit's finding 9) **[R]**: its `tsc -b` is not what a gate pays.

| Step | Cold | Warm | Kept from the last run |
| --- | --- | --- | --- |
| `web-deps` (`npm ci`) | 17.9 s | 0.4 s | stamped on the lock file |
| `go-warm` | 4.2 s | 2.2 s | Go's cache |
| `go-fmt` | 11.9 s | 7.7 s | golangci's cache |
| `go-lint` | 15.3 s | 5.0 s | golangci's cache |
| `go-vet` (six passes) | 9.4 s | 8.3 s | Go's cache |
| `go-tidy` | 0.4 s | 0.5 s | nothing |
| `go-speculative` | 3.3 s | 3.3 s | nothing |
| `check-generated` (`make generate`) | 36.8 s | **37.3 s** | **nothing** |
| `check-docs` | 5.8 s | 5.5 s | nothing |
| `check-source` | 1.0 s | 0.8 s | nothing |
| `check-claims` | 5.7 s | 5.5 s | nothing |
| `check-media-names` | 4.3 s | 3.4 s | nothing |
| `check-architecture` | 0.6 s | 0.3 s | nothing |
| `web-typecheck` (`tsc -b`) | 73.5 s unseeded | 0.6 s | its build info, seeded into a new worktree |
| `web-lint` (eslint) | **260.2 s** | 4.3 s | eslint's cache, in the worktree's `node_modules/.cache`, **not seeded** |
| `web-format-check` (prettier) | **58.9 s** | 18.5 s | prettier's cache, in `node_modules/.cache`, **not seeded** |
| `web-deps-check` | 0.9 s | 1.1 s | stamped on `web/` |

Three things stand out:

- **`check-generated` is the longest warm step, 37 s, and runs on every `make check`** **[M]**: it
  runs the whole of `make generate` (sqlc generate and vet, two `go run ./cmd/orpheum`, which link
  the binary, and openapi-typescript) and compares the tree **[R]**. The floor runs (35–52 s, Go
  cached, no web change) are about the prelude plus this one step **[I]**.
- **A new worktree's first `make check` lints cold: 260 s of eslint and 59 s of prettier** **[M]**,
  longer than the api package's Go tests, so on that run eslint is the critical path **[I]**. Every
  agent now works in a new worktree (gate-throughput G4), so every agent's first gate pays it.
- **prettier stays at 18.5 s warm** **[M]**, against eslint's 4.3 s and `tsc -b`'s 0.6 s.

The steps run beside `go test` in the parallel phase, so on a run that tests the api package none
but a cold eslint is on the critical path; what they cost such a run is CPU taken from it on an
oversubscribed machine **[I]**. On a short run (no api change) the longest of them, `check-generated`,
is the run.

## 2. Options, ranked by saving per cost

Savings are in wall time, per turn of the kind named, at the loads measured. "Files" are what would
change: options 1 and 2 change only test code, which an agent may change, though option 2 needs the
owner's go (test-weight §26); the rest change gate files, the owner's.

| # | Option | Saving | Cost | Protection given up | Files |
| --- | --- | --- | --- | --- | --- |
| 1 | **Batch the two NeedsAttention tests' seeding into one transaction each**, as `seedEpisodeFiles` already does | the api package's wall from 79 s to about 50 s idle, 136 to about 90 s loaded **[I]**: 30–45 s on a core-only turn, 13–40 s on a core-and-web turn (down to the jsdom run's end), and 100–145 s of CPU on every run that tests the package | an hour: a batch form of `librarytest`'s seeders, or the seeding loop inside one `database.Write` | none: the same rows, the same assertions | `internal/core/api/needs_attention_test.go`, `internal/core/library/librarytest/catalog.go` (test code) |
| 2 | **Stamp `check-generated` on its inputs** (the queries, migrations, `sqlc.yaml`, `cmd/orpheum` and what the OpenAPI document and compose files are built from); short of that, build `cmd/orpheum` once rather than `go run` twice | 37 s of CPU on every run; on a short run most of the floor, 35–52 s down to about 15–20 s **[I]** | half a day: the key must cover every input or a stale generated file passes (principles §14); the handlers change often, so the OpenAPI part may still run on most core turns | none if the key is sound | `Makefile` (`generate`, `check-generated`), `scripts/check_generated.py`, `scripts/area_stamp.py` |
| 3 | **Seed eslint's and prettier's caches into a new worktree**, as `seed_worktree.py` seeds `.tsbuild` | a new worktree's first gate: eslint 260 s to about 4 s and prettier 59 s to about 18 s **[M]** for warm, if a copied cache hits: eslint keys its entries by file path, so whether a cache made in the main checkout hits in a worktree must be tried first **[I]** | an hour, after that try | none: `--cache-strategy content` re-lints any file whose content differs | `scripts/seed_worktree.py` |
| 4 | **Split `page-walk` and `input-pages` into several files each** (by slicing `pages`; by `screens`) | the browser tail from 88–105 s to about 60–70 s, then bounded by total work ÷ the page budget **[I]**: 25–40 s on every turn that runs 32 or more browser files | an hour; `check-test-weakening` must see the tests moved, not deleted **[I]** | none | `web/apps/web/src/page-walk.browser.test.tsx`, `input-pages.browser.test.tsx` (test code); test-weight §26 left this "until one fails again", so the owner's go |
| 5 | **Gate-throughput's B: area-stamp `web-typecheck`, `web-lint` and `web-format-check` on `web/`**, as `web-deps-check` already is | on a Go-only turn their 23 s warm (prettier's 18.5 s of it) of CPU; wall on a short Go-only run only once option 2 is built **[I]** | small | none: they read only `web/` | `Makefile:235-240`, `scripts/area_stamp.py` |
| 6 | **G0: `quiet_gate.py` records each step's time** | none itself; it measures option 7 and every later cut, and would have made this audit a read | small | none | `scripts/quiet_gate.py` |
| 7 | **Size the three tools to share the machine**: `go test -p`, vitest's `maxWorkers`, golangci's `concurrency`, together near the CPU count | unknown: the runs at a load of 9 against 42 say oversubscription doubles a run, and the gate alone reaches 8–15 **[I]** | measure with G0 first, then a line each | none | `Makefile` (`go-test`), `web/vitest.config.ts`, `web/vitest.browser.config.ts`, `.golangci.yml` |
| 8 | **Browser mode: Firefox only in `check-pr`**, Chromium in `make check` | the tail from 88–105 s to Chromium's longest file, about 45–55 s; with option 4, about 25 s **[I]** | a line | a Firefox-only failure is found at check-pr, not at the turn; Firefox is the browser that failed under load (test-weight §26) | `web/vitest.browser.config.ts` (`instances`), `scripts/web_tests.py`, `Makefile` (`web-test-browser`, `check-pr`) |
| 9 | **§2.8: drop the `go vet` passes golangci's `govet` subsumes** (amd64, and `-tags integration`) | a few seconds: the six passes take 8.3 s warm | small, after showing the same findings | none if subsumed | `Makefile:194-200` |
| 10 | **Start the browser-mode run when the jsdom run ends**, beside the Go tests | up to the gap between the two ends, 13–40 s | small | brings back the load that failed Firefox's connect on 2026-10-20 | `Makefile` (`check-verbose`) |
| 11 | **Fewer agents gating at once** | about half a run's time at the loads seen (42 against 9) | other work's throughput | none | `docs/contributors/lead.md` §1–2 (an operating rule, not a gate) |

**Option 1, built** (2026-11-13): `needs_attention_test.go` seeds both tests' titles in two statements
(items, then works) and the films' files, versions and streams in one `database.Write`, through the
one `seedListedFile` that `seedEpisodeFiles` now shares. Measured on a loaded machine, other agents
testing, so the numbers move with it. The two tests alone: `ListsAHundredAndCountsThemAll` 23.3 s to
10.9 s, `PastFiveThousand` 56.3 s to 51.2 s. The whole package: 124 s to 82 s, with the films test 50.9
s to 27.5 s and `PastFiveThousand` 91 s to 74 s. **`PastFiveThousand` was not title-bound:** a timed
run put its 1001 titles and positions at 0.5 s and its one-transaction `seedEpisodeFiles` (5005 files)
at 34 s of its 37 s, so the longest test stays the package's wall, and the 79 s to 50 s the option
expected needs that seeding cut too (the per-file triggers are the next place to look).

**Options 2, 5 and 6, built** (2026-11-13, the owner's go): `check-generated` and the web's
typecheck, lint and format check are stamped on what they read (`scripts/area_stamp.py`; a second
`check-generated` on an unchanged tree went from 16 s to none), and every run of `make check`
writes each step's time, the longest first, to `.git/gate-logs/check-verbose.steps`
(`scripts/step_clock.sh` as make's shell under `quiet_gate.py`). Option 3 waits for its try: whether
eslint's cache, keyed by file path, hits in another worktree.

**Considered and not proposed:**

- **The lock queue:** 4% of the time; nothing to win.
- **The disk guard's trim of Go's build cache** (5.3 GiB in one run, oldest first) may evict test
  results **[I]**; Go's test cache held in every run read (60–84 of 126 packages ran), so not before
  it is seen costing one.
- **Documentation-only turns** already run `check-documentation`, 7–19 s.
- **The Stop hook's skip of a passed tree, stamps shared across worktrees, the incremental vitest
  runs:** built.

## 3. Recommendation

1. **Build option 1 now:** test code alone, no protection lost, 30–45 s off every core-only turn and
   100+ s of CPU off every run that tests the api package.
2. **One gate change for options 2, 3, 5 and 6:** `check-generated` stamped (the floor from about 45
   s to about 20 s), the lint caches seeded (a new worktree's first gate no longer 260 s of eslint),
   the web's static steps stamped, and each step's time recorded. Then measure option 7 with those
   times before sizing the tools.
3. **Give the go for option 4** (test-weight §26's deferral): 25–40 s off every turn that reaches the
   browser walks, the run's one serial tail.

Option 8 is the largest single cut to the tail, but it moves Firefox, the browser that has failed,
to check-pr; take it only if options 1 to 4 leave the median over principles §23's 90 s.
