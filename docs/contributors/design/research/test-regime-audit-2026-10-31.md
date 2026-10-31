# Test regime audit: where check-pr's time goes, and what to change (2026-10-31)

Read-only audit of the gate and the test tiers. **[M]** = measured here (logs, kept timings, or a run
timed for this audit on an idle 8-vCPU sandbox); **[I]** = inferred from code and measurements;
everything under `scripts/` and `Makefile` is the owner's, so every change there is a proposal.

The owner's framing, which this audit follows:

> "Whenever tests and checks get slow, it's important to look at them architecturally, because most
> processes we develop those tests under will only work for a particular size range before they fall
> afoul of resource constraints, parallelism, duplication, and algorithms no longer suited to the task.
> We should also be looking holistically at the testing regime, deciding if the tests being run are
> being run in the most efficient manner and even in the right phase (for example go tests vs
> integration tests vs browser tests). Often, moving a test to a faster tier gives a similar level of
> protection for less cost. Or splitting what is tested into fast and slow tier tracks could help."

Prior work this builds on, not repeats: `gate-throughput-2026-10-26.md` (probe CPU shares, G6),
`test-architecture-2026-10-26.md` (Go per-test coverage map, vitest per-file cost),
`caching-audit-2026-10-28.md` (green records, "the test that caught it before", web keys by modules
read — all built, D378). The caching audit ended: "Caching alone does not bring check-pr to its 300 s:
revert-red probing ~180 files, each with several runs, is the structural cost." This audit is that
structural look.

---

## 0. Summary

1. **check-pr is revert-red.** Last run 2788 s: `check-revert-red-code` 2480 s; every other step
   together ≈ 260 s on the critical path (browser 127, lint-full 71, acceptance 44, integration 18,
   revert-red-browser 12) **[M]**. Without revert-red, check-pr is at its budget already.
2. **18 of 178 probed files are 80 % of the 5003 s of probing**; 122 files cost 752 s together (the
   hint-first and green records work) **[M]**. The expensive 18 fall in four algorithmic patterns:
   - **Claimed files probed to the end** (`Verified-By`/`Refactor-Only`): ~1640 s, whose verdict
     passes whatever the probe finds (§2.1).
   - **Stylesheet halving where every half re-runs the whole browser-mode suite**, serially, untimed:
     the inferred critical path, ~1500 s of the 2477 s wall (§2.2).
   - **TypeScript probes that run a 110–140 s `vitest related` that stays green, then a ~30 s type check
     that goes red**: 8 files, 972 s (§2.3).
   - **An added file's transitive importers (≈160 test files) run before its one direct test**
     (~1.3 s alone): 4 files, 416 s (§2.3).
3. **The import graph has a hub in the test helpers.** `test-dom.tsx` imports `later-screens.tsx` (for
   one reset function), whose dynamic imports reach every screen, so **151 of 306 vitest files** are
   "related" to any screen component, and **21 of 24** browser-mode files are too **[M]**. `vitest related
   film-player.tsx` = 141 files, 51 s alone at 2 workers, 110–200 s inside check-pr; the whole suite
   is 42 s at full width **[M]**. "Related" has stopped meaning narrow.
4. **The shape is quadratic.** Uncached probe cost ≈ (changed files) × (fraction of the suite each one
   relates to, ~½) × (suite time). Vitest files 101 → 306 and browser-mode files 0 → 24 in three weeks
   (10-10 → 10-31) **[M]**, while batches grew from ~30 to ~150 commits.
5. **Parallelism is at the CPU wall.** Four trees × 2 workers each made every vitest run 2.2–3.9× slower
   than alone, so four trees deliver about one full-width run's throughput **[M]/[I]**. More trees will
   not help. Only less work will.

Top proposals (§4): R1 claimed files take only cheap runs (−1640 s probing, incl. the 825 s serial
browser tier); R2 per-sheet narrow runs before halving (−~1300 s wall); R3 TS probe order: direct
importers, then the type check, then transitive related (−~1370 s probing); R4 cut the test-helper hub,
not a gate change (shrinks every remaining related run 3–6× by the static graph); R5 name the failed
tests of browser-mode reds and share clean runs across trees (−300 to −1000 s). Together **[I]**:
revert-red ≈ 450–600 s wall, check-pr ≈ 700–850 s. **That is still over 300 s.** Fitting the budget
needs R6: revert-red moved off check-pr's synchronous path (incremental after each commit), or a
separate budget for it, which is the owner's decision (§25).

---

## 1. Where the time goes

### 1.1 Per step (check-pr 2026-10-31, base an earlier commit, 155 commits) [M]

| Step | s | Note |
| --- | ---: | --- |
| check-revert-red-code | 2480 | 178 files, 4 trees, 5003 s of probing |
| test-browser | 127 | 36 tests; 135 s of test time summed over two browsers |
| web-lint-full | 71 | in the parallel group |
| test-acceptance-home | 44 | serial by design |
| test-integration | 18 | |
| check-revert-red-browser | 12 | |
| the rest | 5 | |

The previous run (2026-10-30, 136 commits, failed): revert-red 3286 s, 10891 s of probing **[M]**.

### 1.2 Per probe kind (latest, then failed run) [M]

| Kind (as the report labels it) | Latest s / runs | Failed run s / runs |
| --- | ---: | ---: |
| vitest related | 1954 / 18 | 5489 / 89 |
| vitest, "an added file's importers" | 946 / 90 | 873 / 21 |
| browser tier | 825 / 5 | 942 / 9 |
| web type check | 436 / 15 | 492 / 16 |
| "nothing reverted" confirmations, all kinds | ~343 / 34 | ~1195 / 94 |
| Go (naming, package, dependents, tags) | ~315 / 57 | ~1255 / 79 |
| catalogue / readers run | 101 / 18 | 320 / 24 |

**Two labels mislead** (`revert_red_timings.kind_of`): a browser-mode `vitest related` run is counted as
"vitest related" (film-player's second "related" run, 200 s, was the browser-mode one; so were
filing-report's 221 s and cast-asked's 173 s), and every plain `vitest run` (including the hint runs of
"the test that caught it before") is "an added file's importers". The 90 such runs average 10.5 s; one
test file takes 1.3 s alone **[M]**, so most of that is contention.

**One kind is not timed at all:** the browser-mode `related` run that `stylesheets_together` makes
before each halving group's browser-tier run (`check_revert_red.py:439`, `run_whole_group` called
without `timed`). See 2.2.

### 1.3 Per file (latest) [M]

| s | File | Runs | Verdict |
| ---: | --- | --- | --- |
| 825 | 11 stylesheets, halving | 5 browser-tier runs, 123–228 s (+16 untimed browser-mode runs) | 4 red, 5 claimed, 2 other |
| 535 | cast-asked.tsx | related 112 green → type 22 green → browser-mode related 173 red → clean re-run 224 | red |
| 381 | film-player.tsx | related 135, readers 10, type 35, browser-mode related 200, all green | claimed (VQ176) |
| 370 | filing-report.tsx | the same pattern, 119 + 6 + 23 + 221 | claimed (VQ161) |
| 180–128 | rtc-request, use-together, drop-down, together-follow-options, services, later-screens, together-notes, server-settings | related 109–142 green, then the type check red | red (build) |
| 177 | internal/core/together/links.go | naming 1 + 48, dependents 69, extra tags 56, all green | claimed (VQ193) |
| 119–84 | tray-dialog-code, tray-dialog, together-dialog, cast-dialog (added) | transitive importers 83–118, red | red |

### 1.4 Wall versus probing

Utilisation of the four trees: 10-21 97 % (754 s wall / 2921 s probing), 10-30 83 %, 10-31 **50 %**
(2477 / 5003) **[M]**. Simulating the latest run's per-file times on 4 workers gives 1289 s in path
order and 1251 s longest-first **[M]**, so scheduling is not the gap. The gap, ~1190 s, matches
the stylesheet halving's untimed browser-mode runs: 16 of them, run one after another in one tree, each
≈ the whole browser-mode suite (measured below at 107 s alone). That chain, ~825 + ~1500 s, is the
critical path **[I]**. Recording each run's start offset would make this a measurement next time.

### 1.5 How it scales [M]

| Date | Batch | Probed files | Probing s | s per file | Wall s |
| --- | --- | ---: | ---: | ---: | ---: |
| 10-08 | small | – | – | – | check-pr 349 |
| 10-13 | – | 164 | – | – | revert-red 2222 |
| 10-21 | 24 commits | 78 | 2921 | 37 | 754 |
| 10-30 | 136 commits | ~180 | 10891 | ~60 | 3283 |
| 10-31 | 155 commits | 178 | 5003 | 28 (hints) | 2477 |

| Date | vitest files | browser-mode files | Go test files | web source files |
| --- | ---: | ---: | ---: | ---: |
| 10-10 | 101 | 0 | 302 | 140 |
| 10-21 | 161 | 4 | 430 | 221 |
| 10-31 | 306 | 24 | 579 | 394 |

The algorithmic shapes, named:

- **Per-file × related-suite, with related ≈ half the suite** (hub fan-in, §3.1). Cost ∝ files × suite:
  quadratic in the project's growth. The known-red hints make *repeat* files cheap; new files, greens
  and claimed files still pay the product every run.
- **Expensive-before-cheap ordering**: a 110–140 s related run before a ~30 s type check; a transitive
  run of ~160 test files before the one direct test.
- **Halving whose unit run is not narrow**: designed when one run was a 160 s browser tier; its
  pre-run became browser-mode `related`, which for any sheet is ~the whole browser-mode suite (42 of 48
  file instances for the 9 sheets, 108 s; the whole suite is 73 s) **[M]**. ~2k·log₂n of those, serial.
- **Confirmation doubling**: a red whose failing tests cannot be named (browser-mode output) re-runs
  its whole command with nothing reverted (cast-asked 224 s; ui-gallery 218 s on 10-30). Kind-level
  clean runs are cached per *worktree*, so each kind is paid up to 4 times.
- **Records too coarse for greens**: a web green is kept by the whole web tree (`revert_red_known.py`),
  which every web commit changes, so the same claimed greens (film-player, filing-report) are
  re-probed in full every run: ~750 s each time.
- **CPU contention**: `CPU_SHARE` = 8 / 4 = 2 per tree. `vitest related film-player.tsx` alone at 2
  workers: 51 s; at full width: 35 s; inside check-pr: 135 s, 129 s, 200 s **[M]**. Four 2-worker
  runs at ~150 s ≈ one run per 37 s, about what a single full-width run gives.
- **Per-file fixed cost in the suites themselves**: vitest 42 s wall for 1917 tests, 240 s of file time
  summed with environment 34 % and import 28 % **[M]**; browser-mode "worker 94 %, tests 5 %" **[M]**: a
  browser-mode file's cost is almost all setup, so any probe that runs many of them pays setup many times.

---

## 2. Revert-red, specifically

### 2.1 Claimed files are probed to the end for a verdict that cannot change

`probe()` consults a claim only after the probe goes green (`check_revert_red.py:258-268`), and since
D378 an unticked VQ item passes. So for a file with a valid `Verified-By`/`Refactor-Only` the outcome
is RED (pass) or CLAIMED (pass). Only an *invalid* claim (no such VQ item) can fail, and that is decided
by reading the queue, with no probe. Latest run: film-player 381 s, filing-report 370 s, links.go 177 s,
client-main 49 s, and the five claimed stylesheets' browser-tier runs, 825 s: **~1640 s of probing,
including the whole serial browser tier, spent on category labels** **[M]**. What the probe adds is
"also covered by a test", useful to review but not to the gate.

### 2.2 Stylesheets: halving with a whole-suite unit

`settle()` halves 11 sheets; each group first runs `vitest related --config vitest.browser.config.ts
<sheets>`, then (if green) the browser tier. Latest run: 11 failing browser-mode runs and 5 passing ones
(16 untimed runs), then 5 browser-tier runs (825 s) for cast.css, icon-button.css, play.css,
status-tray.css and together-viewer.css, **all of which ended claimed** **[M]**. Every red sheet was found
by a browser-mode test that imports it directly (app-shell.browser.test.tsx, modal-dialog.browser.test.tsx,
...). Halving minimises the number of runs. When each sheet's own direct tests take seconds, minimising
runs is the wrong objective.

### 2.3 Run order for TypeScript

`attempts_for` orders: hints → `vitest related` (transitive) → readers → other builds (type check) →
browser mode → browser tier. Measured consequences:

- 8 files: related green 109–142 s, then the type check red: **972 s** for a verdict the type check
  gives alone (RED_BUILD both ways) **[M]**.
- 4 added files: transitive importers 83–118 s, red. Their direct test, alone, ~1.3 s **[M]**.
- `importing_tests` (Python, for added files) counts `import type` lines, which are erased before any
  test runs and so cannot turn one red; removing them takes film-player's set from 160 to 26 once the
  hub edge is cut (§3.1) **[M, static graph]**.

### 2.4 The questions asked

- **Batch reverts of files with disjoint related sets into one run?** Not now. With related ≈ half the
  suite, almost no two sets are disjoint **[M]**. After R3/R4 the narrow sets are 0–12 test files and a run
  costs ~1–10 s, so batching would save only vitest startup (~0.5–1 s a run) at the price of masking
  and attribution logic. Not worth it.
- **Coverage-guided selection instead of import-graph "related"?** Yes, as the structural end state
  (R7): tests that *executed* a changed function, from a per-test-file coverage map at HEAD, first;
  import-graph related only as the green fallback. It needs no new dependency (`node:inspector`'s
  `Profiler.takePreciseCoverage` in a setup file; Go's `-coverprofile`, already proposed in
  test-architecture #1). Ranked below R1–R5 because those take most of the time with far less machinery.
- **Confirmation once per test set rather than per file?** It is already per failing test when the
  output names them (`failed_pass_clean`). The waste is (a) browser-mode output not parsed, so the
  whole command re-runs, and (b) the kind-level clean run cached per worktree. R5.
- **Drop or sample confirmation when the same tests just passed at HEAD?** `make check` did run the
  suite green at HEAD, but the confirmation exists for the *probe tree's* environment and for tests that
  fail under load (A45, the 10-26 oversubscription). Once named (R5) it costs seconds. Keep it; do not
  sample.
- **Probe per commit at commit time (cached) vs per batch?** A record is keyed by the base→head revert's
  blobs. A file touched by two commits of the batch has a different revert per commit, so commit-time
  results transfer only for files touched once. Doing it *in* the commit spends the 30 s spot budget. The
  version that works is R6: probe the batch-so-far (base→HEAD) in the background after each commit,
  so check-pr mostly reads records.
- **Are the known-red/green keys too coarse?** Reds: no, since D378 they are keyed by the failing tests'
  modules. Greens: yes, the whole web tree (§1.5). With R1 the main victims (claimed greens) stop
  mattering. A real unclaimed green fails the gate and gets fixed, so it is rarely re-probed.

---

## 3. The regime as a whole

### 3.1 The hub in the test helpers [M]

Static import closure (Python's graph, as revert-red's added-file scope uses), test files reaching a
module:

| Module | Test files reaching it | Note |
| --- | ---: | --- |
| i18n.ts | 181 | the catalogue: an inherent hub |
| router.ts | 179 | |
| services.tsx | 178 | |
| test-services.tsx | 169 | test helper |
| test-dom.tsx | 151 | test helper; imports later-screens.tsx for `resetLazyScreensForTests` |

Any screen component → later-screens.tsx (dynamic `import()`) → test-dom.tsx → 151 test files. The
same path takes 14 of 24 browser-mode files (they import test-dom). Cutting that one edge, and ignoring
`import type`:

| File | now | edge cut | + type-only ignored |
| --- | ---: | ---: | ---: |
| film-player.tsx | 160 | 160 | 26 |
| series.tsx | 160 | 23 | 16 |
| title.tsx | 163 | 42 | 36 |
| status-tray.tsx | 160 | 39 | 38 |
| filing-report.tsx | 163 | 45 | 44 |
| drop-down.tsx | 165 | 73 | 72 |
| cast-asked.tsx | 160 | 160 | 51 |
| modal-dialog.tsx | 165 | 165 | 93 |

vitest's own graph strips type imports but follows dynamic imports. So for vitest the edge cut is the part
that counts, and the expected effect is in the "edge cut" column for files reached through screens
**[I]**: measure `vitest related` after the change. This is web test code, not a gate: an agent can
make it. It also narrows `make check`'s and the spot check's related runs.

### 3.2 Tiers: what is slow, and whether it sits in the right one [M]

- **vitest** (306 files, 1917 tests): 42 s wall at full width, 240 s of file time. The slowest file is
  4.5 s (rtc-media). There is no outlier to move. The cost is per-file environment and import (62 %),
  already partly addressed by the `fast` project. The tier is right. The fan-in (3.1) is the problem.
- **browser-mode** (24 files × Chromium + Firefox): 73 s whole, "worker 94 %, tests 5 %". Its tests are
  cheap and its setup is not. The tier is right for what it checks (layout, a real engine), but it is
  the wrong *probe* for a component whose change no layout test can see. Run it for a TS file only when
  the file's emitted JS changes what a browser-mode test imports *directly* (the probe already skips
  the browser for unchanged emitted JS). Today it is run for any transitively related file.
- **Browser tier** (36 tests, 127 s step, 123–228 s per probe run): already lean. The longest tests are
  multi-client discovery and Watch Together journeys (8–12 s) that need the server and two browsers.
  Nothing here duplicates unit coverage cheaply enough to move. Its probe cost is fixed setup, so the
  fix is fewer probe runs (R1, R2), not fewer tests.
- **Go unit** (2496 tests; package test time summed: identify 424 s, api 286, library 263,
  cmd/orpheum 188): scale- and scenario-shaped tests sit in the unit tier.
  `identify TestPass_OverAnUnchangedLibraryRunsTheSameStatementsAtTwiceItsSize…` 30.6 s,
  `identify TestPass_FromTheSnapshot…` 23.6 s, `assembly TestScenario_S25…` 6 s median and 74 s worst,
  backup tests 5–20 s, `waits TestEstimator_…` up to 21 s. They set the floor of any spot check or Go
  probe touching those packages. **Slow-track candidates (R8)**: same protection at check-pr, cheaper
  spot.
- **Go integration** (15 tests, 18 s) and **acceptance-home** (44 s): right-sized.

### 3.3 Duplication across tiers

The duplication that costs is in the *probes*, not the suites: one revert of a TS file can run related
(jsdom), the readers run, the type check, browser-mode related and the browser tier, each re-running
largely the same tests in another harness. A claimed file runs all of them. Within the suites, nothing
measured shows a browser-tier test whose protection a vitest test already gives. The 36 tier tests are
journeys.

---

## 4. Proposals, ranked by time saved × risk to protection

Savings use the 2026-10-31 run. Probing seconds convert to wall at about ¼ (four trees) unless on the
critical path.

### R1 — A file with a valid claim takes only its cheap runs. **Gate change.**

- **Mechanism.** In `probe()`, check the claim first (`refactor_only` / `verified_out_of_suite` +
  `verification_queue_problem`). If it is valid, run only the attempts up to and including the type check:
  hints, direct importers, naming tests, the type check. A red there → covered as today; else → CLAIMED.
  An invalid claim → the full probe as today. `stylesheets_together`: a sheet with a valid claim
  joins no browser-tier group. `make check-everything` (nightly, `--trust-no-record`) keeps probing
  claimed files fully, so the "is it also covered?" answer still comes daily.
- **Files.** `scripts/check_revert_red.py` (`probe`, `stylesheets_together`), `revert_red_claims.py`.
- **Saving.** 825 (browser tier) + film-player 381→~45 + filing-report 370→~29 + links.go 177→~49 ≈
  **1630 s probing**. The 825 s is serial in one tree, so most of it is wall.
- **Protection given up.** None to pass/fail (a claimed file passes red or green). The report shows "on
  the author's word" for files a wide run would have shown covered, until the nightly.

### R2 — Stylesheets: narrow per-sheet runs first, halving only for the tier. **Gate change.**

- **Mechanism.** Per changed sheet, in parallel trees: revert it alone and run its known-red hint tests,
  then the browser-mode tests that import it directly (depth 1). Red → settled. Sheets still green and
  unclaimed go into `settle()`'s halving over the browser tier, as today. Time every run (the
  browser-mode pre-runs are untimed today) and record start offsets.
- **Files.** `check_revert_red.py` (`stylesheets_together`), `revert_red_stylesheets.py`,
  `revert_red_timings.py`.
- **Saving.** 16 browser-mode runs ≈ 1500 s serial **[I]** → 11 sheets × ~10–15 s spread over 4 trees ≈
  40 s wall. **~1300–1450 s wall**, since this chain is the inferred critical path.
- **Protection given up.** None: a narrow red is a red, and greens go on to the same halving.

### R3 — TypeScript run order: direct, type check, then transitive. **Gate change.**

- **Mechanism.** Hints → tests importing the file directly (value imports only, depth 1; for added
  files from `importing_tests(depth=1)`, for modified ones the same reader) → type check → transitive
  related → readers → browser mode → tier. `importing_tests` ignores `import type` lines.
- **Files.** `check_revert_red.py` (`attempts_for`, `scope_for`), `revert_red_web.py`.
- **Saving.** 972 s (8 green related runs ahead of a red type check) + ~400 s (4 added files' transitive
  runs → direct tests) ≈ **1370 s probing, ~340 s wall**.
- **Protection given up.** None to pass/fail. Some files may be labelled RED_BUILD where a later test
  would have shown RED_TEST. Direct importers run before the type check, so most RED_TESTs still
  surface first.

### R4 — Cut the test-helper hub; keep it cut with a test. **Not a gate change.**

- **Mechanism.** `test-dom.tsx` stops importing `later-screens.tsx`: the lazy screens register a reset
  in a small registry module (screen-load.tsx or new `lazy-reset.ts`) that test-dom calls. Add a vitest
  test that bounds fan-in: no shared test helper's static closure includes a screen module, and no
  non-allowlisted source is reached by more than N % of test files (i18n, services and router named as
  inherent hubs). Rule 10's `TestArch_` form, in the web suite.
- **Files.** `web/apps/web/src/test-dom.tsx`, `later-screens.tsx`, `screen-load.tsx`, one new test.
- **Saving.** Every related run that remains after R1–R3 (greens, files without hints) shrinks ~3–6×
  for screen-reached files by the static graph (series 160→23, title 163→42, status-tray 160→39)
  **[M static, I for vitest]**. It also narrows `make check`'s and `make spot`'s related runs and the
  browser-mode related runs (14 of 24 browser files import test-dom).
- **Protection given up.** None. "Related" stays the import graph, and the graph becomes more accurate.
  The lazy-screen cold-start guarantee is kept by the registry.

### R5 — Name browser-mode failures; share clean runs across trees. **Gate change.**

- **Mechanism.** (a) `revert_red_known.failed_tests` reads vitest browser-mode output (`FAIL chromium
  apps/…browser.test.tsx`) so the confirmation re-runs those files only. (b) `revert_red_clean._key`
  and `_CLEAN` drop the tree: (head, kind, toolchain). Keep the per-red failed-tests re-run.
- **Files.** `revert_red_known.py`, `revert_red_clean.py`.
- **Saving.** cast-asked 224→~15 s. Kind-level clean runs ÷4. Latest ≈ 300 s probing. On a first run at
  a base (10-30) up to ~1000 of 1195 s.
- **Protection given up.** (b) a broken environment in one tree but not another goes unnoticed by the
  kind-level run. The trees are made identically, and the per-red re-run still runs in the red's own tree.

### R6 — Take revert-red off check-pr's synchronous path. **Gate change + owner's budget decision.**

- **Mechanism.** After each commit, at idle and niced on ≤2 trees, probe base→HEAD for files without a
  current record (`--part code`, records kept as now). check-pr then reads mostly records and probes only
  what changed since. Pause while staging is being tested (memory: no gates during live tests). The
  alternative is a separate revert-red budget.
- **Saving.** Up to all of revert-red at check-pr time for a batch committed over hours.
- **Why needed.** After R1–R5 the estimate is ~450–600 s wall for revert-red **[I]**, plus ~260 s of the
  other steps: still more than twice 300 s.
- **Protection given up.** None. Same probes, same keys, earlier. The cost is CPU taken from the dev loop.

### R7 — Coverage-guided first runs (structural). **Gate change; vitest config is a gate file.**

Per-test-file function coverage at HEAD for vitest, via `node:inspector` in a setup file (no new
dependency), and per-test Go coverage (test-architecture #1). The first run for a file is the tests that
executed its changed functions. The import-graph run stays only as the green fallback. It removes the
reliance on hints for new files and makes first probes ~2–5 s. Medium-large effort, so it ranks after
R1–R5, which remove most of today's time.

### R8 — A Go slow track. **Mostly test code.** Moving tests under `-tags integration` may need a `Test-Weakened:` note for check-test-weakening.

Move scale and scenario tests (identify's two scale passes 30.6 s and 23.6 s, assembly S25 6–74 s,
backup's production-cost restores 5–20 s) to the integration tag or their own package, which check-pr's
`test-integration` and the nightly run. Saving: 20–70 s off a spot or `make check` touching those
packages, and off Go probes of them (revert-red's "extra tags" attempt still reaches them). Protection
given up: the spot check no longer runs them. check-pr does.

### Considered and not proposed

- **More probe trees**: CPU-bound already (§1.5).
- **Longest-first scheduling**: 1289 → 1251 s simulated. Negligible.
- **Batching disjoint reverts**: §2.4.
- **Sampling confirmations**: cheap once R5 is in.
- **A warm vitest server per tree**: startup is ~0.5–1 s, and isolation still pays environment and
  import per file.

**Combined [I]:** probing 5003 → ~1500 s timed, plus the stylesheet chain ~1500 → ~150 s. Revert-red
wall 2477 → ~450–600 s, and check-pr 2788 → ~700–850 s. With R6, check-pr ≈ 260–350 s.

---

## 5. Making this audit recur, cheaply

1. **A ledger.** `check_pr_steps.py durations` appends one JSON line per check-pr to
   `.git/gate-logs/check-pr-history.jsonl`: step seconds, commits, probed files, probing seconds by kind,
   and suite sizes (vitest files, browser-mode files, Go test files), counted by `git ls-tree`, ms.
   **Gate change.**
2. **A trigger, not a failure.** check-pr's last line says "test-regime review owed" when either:
   - any step is over budget in 3 consecutive runs, or
   - probing seconds per probed file exceed 1.5× their trailing median of 5 runs. Per file, because a
     big batch is expected to cost more and a costlier file is the architectural signal.

   The handoff and Stop-hook summary list it. **Gate change.**
3. **The fan-in test (R4)** keeps the cheapest regression from returning without any gate.
4. **The kept timings already exist** (`.dev/test-timings/*`, `.git/revert-red/last-timings`). Fixing the
   two mislabels and the untimed stylesheet runs (§1.2) and adding run start offsets is what makes the
   next review a reading instead of a reconstruction ("digging means missing logs").
5. **Cadence.** At each milestone audit (rule 17), re-run §1's tables from the ledger and the related
   fan-in counts (a ten-line script over `revert_red_web.importing_tests`), and re-time three numbers:
   - `vitest related` of one screen at `--maxWorkers=2`,
   - the whole vitest suite,
   - the whole browser-mode suite.

   Ten minutes, no gate run.

---

## Appendix: measurements made for this audit (idle machine, 8 vCPU)

| Run | Files | Wall |
| --- | --- | ---: |
| `vitest run cast-dialog.test.tsx --maxWorkers=2` | 1 | 1.2–1.4 s |
| `vitest related film-player.tsx --maxWorkers=2` | 141 | 51 s |
| `vitest related film-player.tsx` (full width) | 141 | 35 s |
| `npm --prefix web test` (whole) | 309 | 42 s |
| browser-mode `related` of the 9 changed sheets, 2 workers | 42 of 48 | 110 s |
| browser-mode `related film-player.tsx`, 2 workers | 42 of 48 | 109 s |
| browser-mode whole suite (default width) | 48 | 76 s |

Static fan-in: `scripts/revert_red_web.importing_tests` over the tree at an earlier commit, with the edge-cut
variants computed by the same reader.
