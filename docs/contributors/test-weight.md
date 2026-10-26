> **ABOUTME:** How the gates and test tiers get slower without anyone deciding they should: each
> mechanism we have found, how it crept in, how it was found and fixed, and the warning signs to
> look for, so the next slowdown is recognised early rather than measured after it hurts.

# How the tests get heavier

A test run rarely slows by one large step. It slows by many small ones, each reasonable when it
was added: a realistic timer, a bigger fixture, a check that runs "just in case". No single commit
looks guilty, and the budget is only noticed when it is broken. On 2026-10-08 check-pr took 349 s
against its 300 s budget. The test audit
([design/research/test-audit-2026-10-08.md](design/research/test-audit-2026-10-08.md)) found that
most of the time went to **waiting and regenerating, not testing**. The mechanisms below are what
it found. Add a new entry when a new one turns up, in the commit that fixes it.

Each entry says: the **mechanism**, the **instance** we found, **why it was invisible**, the
**fix**, and the **warning sign**.

## Every run keeps its timings

Expect the tests to grow heavier and slower over time. So every run records where its time went,
cheaply and always, and a run found slow is explained from what it already wrote, never by running
it again to profile it (the owner, 2026-10-20). A new suite or tier keeps its timings from the
start.

- **Every suite** keeps its last 20 passes in `.dev/test-timings/<suite>/`, one shape for all
  (`web/e2e/test-timings.ts`): `browser` and `acceptance` each test's time and each step's (a
  navigation, a click, an `expect`'s wait, a `test.step`; `web/e2e/timings.ts`); `vitest` and
  `vitest-browser` each test's and each file's, its loading apart (`web/e2e/vitest-timings.ts`);
  `go`, `go-dev` and `go-integration` each test's and package's, from `go test -json`, which
  `scripts/go_test_timings.py` turns back into go test's own text. check-pr's own tree links the
  directory, so its runs are kept too.
- `python3 scripts/test_timings.py [--suite go]` lists a suite's latest pass's slowest tests and
  files against their median over the passes before, with each test's longest steps; `--list`
  names the suites kept.
- **check-pr** prints each step's time, and revert-red each file's.

The first reading (2026-10-20) found each claim waiting about 10 s on the core's constant
`pendingEvery`, mechanism 1 below in the browser tier: it became the setting
`discovery_pending_check_every_seconds`, which the tier sets to 1 s.

## The mechanisms

### 1. A production timer in a test's config, behind a start-up race

- **Mechanism:** a test uses a realistic interval (a status tick, a retry, a poll). Normally
  nothing waits for it. But a race at start-up makes the first attempt fail, so every test waits
  one full interval.
- **Instance:** `cmd/orpheum`'s `testConfig` set the node's status tick to 10 s. The node's first
  hello beat the core's socket, so it waited for the next tick. Seven tests took about 10 s each,
  and the package took 17 s instead of 5 s. The browser tier's server had already fixed the same
  race by starting the node once the core answered (`web/e2e/server.ts`). The Go helpers never
  got the same fix.
- **Why it was invisible:** the tests ran in parallel, so the package cost about 10 s, not 70.
  It also varied with scheduling: one integration test measured 5.5 s, 7.1 s and 17.7 s.
- **Fix:** a 1 s tick in the tests' config (`cmd/orpheum/serve_test.go`).
- **Warning sign:** a test whose time is close to a round number in the config (10 s, 30 s, 60 s),
  or whose time varies widely between runs. Its CPU time is far below its wall time:
  0.87 s of CPU over 10.15 s.
- **When fixing a race, fix every copy:** grep for the other places that start the same pieces.

### 2. Deterministic fixtures regenerated on every run

- **Mechanism:** a fixture is made from fixed inputs, so it comes out the same every run, but it
  is remade each time. Each new test that needs more realism (1080p, a longer film, a new
  codec) adds seconds to every run.
- **Instance:** the browser tier encoded two 90 s 1080p timing films, about 20 s of FFmpeg each,
  one after the other, before any test ran.
- **Why it was invisible:** the time sat in global setup, which reports no test durations. Only
  the gaps between log timestamps showed it.
- **Fix:** films are kept in `.dev/browser-films/`, keyed by FFmpeg's version and the arguments
  (`web/e2e/film-cache.ts`). Films the cache lacks are made at the same time.
- **Warning sign:** a gap between "starting" and the first test in a tier's log. A setup whose
  outputs never change.

### 3. Shared setup reused by a run that needs only part of it

- **Mechanism:** a new suite calls an existing setup to get a server, and inherits everything
  that setup makes for other suites.
- **Instance:** the home acceptance called the browser tier's `startServer`, so it made the four
  spec films again, about 35 s of check-pr's serial path. Then it played only its own film.
- **Why it was invisible:** it looked like reuse, and reuse looks right.
- **Fix:** `startServer`'s `shouldMakeSpecFilms`, false for the acceptance.
- **Warning sign:** a setup function whose caller uses a small part of what it builds.

### 4. A tier defined as "everything, with a tag"

- **Mechanism:** a build tag adds a few tests, but the tier runs the whole suite with the tag
  set. The tier's cost then grows with the unit suite, not with its own tests. It runs uncached
  too, because a fresh run is its point.
- **Instance:** `make test-integration` ran `go test -tags integration -count=1 ./...`: 14 tagged
  tests, plus 1,167 tests that `make check` had already passed on the same code. That took 38 s,
  against 20 s for the 14 alone, and it took CPU from the timing-sensitive browser tier beside it.
- **Why it was invisible:** the command reads naturally, and the dev tier had already been fixed
  the same way (`DEV_ONLY_TESTS`, 2026-10-06), without anyone noticing the integration tier had
  the same shape.
- **Fix:** the tier runs only the tests in tagged files, by name, and fails if it finds none.
- **Warning sign:** `-tags X ./...` anywhere. Ask which tests only the tag builds.

### 5. A per-file cost that multiplies by the batch's size

- **Mechanism:** a branch gate does something per changed file, and each "something" is the
  whole suite. The cost is files × suite, so it grows with both, and most with a large batch.
- **Instance:** revert-red tried each changed TypeScript file against the whole web suite. That
  took about 54 s for 5 files, and was the step that pushed a 17-file batch to 349 s. Go files
  already started with their own package.
- **Why it was invisible:** small batches stayed within budget. The overrun showed only on a
  large batch, and looked like a one-off.
- **Fix:** a TypeScript file is tried first on the tests that import it (`vitest related`). Only
  one that survives widens to the whole suite.
- **Warning sign:** check-pr's time tracking the number of changed files rather than what
  changed.
- **A limit to know:** a barrel module widens "related". Everything that imports the SDK's
  `index.ts` is related to every SDK file: 65 of 95 test files for one of them.

### 6. Unconditional steps in the fast gate

- **Mechanism:** each step added to `make check` runs on every turn, whatever changed. Go's test
  cache hides this for Go. Steps without a cache pay in full every time.
- **Instance:** the web tests (35 s under load), the web dependency rules (13 s) and the gate
  scripts' tests (11 s) ran on Go-only turns, competing for CPU with the Go steps that had
  actually changed.
- **Why it was invisible:** the steps run in parallel, so each looked cheap on its own. The cost
  showed up as contention: everything else was slower.
- **Fix:** `scripts/area_stamp.py` skips a step when nothing it reads has changed since it last
  passed. An unchanged `make check` went from 36 s to 17 s.
- **Warning sign:** a new step in `check-parallel` without a cache. When adding one, say what it
  reads.

### 7. Waiting on a machine-wide lock

- **Mechanism:** a tool takes one lock for the whole machine. Two runs that look parallel queue
  behind each other, and the wait shows up as the step's own time.
- **Instance:** golangci-lint's lock. The speculative-API lint ran its two platforms one after the
  other, each behind `make check`'s `go-lint`, and took 36 s after a Go change. On its own it
  takes about 1.5 s.
- **Why it was invisible:** run alone, the step was fast. It was slow only inside the gate.
- **Fix:** the platforms run at the same time with `--allow-parallel-runners`, each on a
  `GOLANGCI_LINT_CACHE` of its own.
- **Warning sign:** a step much slower in the gate than alone. "Waiting for lock", or
  `--allow-serial-runners`, in a recipe.

### 8. Real time spent reaching a timeout

- **Mechanism:** a test reaches a time-based state (a stall, a retry, an expiry) by sleeping for
  real. Each new test of a timeout adds its full length.
- **Instance:** two web tests slept 1.05 s to reach a stall. `request-wait`'s fake timers moved
  forward in real 20 ms steps (`shouldAdvanceTime` defaults to that).
- **Why it was invisible:** a second per test does not show in a 35 s suite.
- **Fix:** fake timers advanced by the test; `advanceTimeDelta: 1`. Rule 16 already forbade the
  real clock; these slipped past review.
- **Warning sign:** `setTimeout` inside `await new Promise` in a test, `time.Sleep` outside a
  bounded poll, `shouldAdvanceTime` without `advanceTimeDelta`.

### 9. A change that cannot affect the code paying for the code's tests

- **Mechanism:** a gate classifies changes too coarsely, so a change that cannot alter what a
  test sees still runs the tests.
- **Instance:** a commit of documentation alone ran the whole `make check` in the pre-commit
  hook. check-pr ran its integration tier and uncached lint for it. The Stop hook already ran only
  the docs check, but only for `docs/`, so a package README ran everything.
- **Why it was invisible:** each tool decided for itself, and only one of them had been taught.
- **Fix:** `scripts/documentation_only.py` is the one definition (`docs/` and any Markdown file).
  Both hooks and check-pr use it, and `make check-documentation` (about 2 s) runs only what reads
  the docs (the owner, 2026-10-08).
- **Warning sign:** a classification written in more than one place, such as a hook's
  `startswith("docs/")`.

### 10. A page's first render, under the gate's contention, nearing vitest's 5 s limit

- **Mechanism:** `make check` runs the Go tests, golangci-lint and the web checks at once, the web
  typecheck, ESLint, the format check and vitest's own workers together (`check-web`'s `-j5`), on
  the same CPUs. A test that renders a whole page with React in happy-dom is slowed far more than
  the CPUs' share suggests, and vitest's default `testTimeout` of 5 s is the same alone or under
  load.
- **Instance:** `roles-section.test.tsx`'s first test, which renders the whole People page, timed
  out at 5.1 and 5.6 s in two gate runs on 2026-10-12, and passed when the commit was retried. Alone
  it takes 0.2 s; in a web-only run no test passed 1.3 s. In the same gate runs, the first test of
  `together-viewer.test.tsx` took 3.6 s, of `watch-together.test.tsx` 3.0 s, and of
  `screen-refusals.test.tsx` 2.8 s: about 25 times slower than alone.
- **Why it was invisible:** it fails only when the gate's steps overlap and the machine is busy,
  such as when staging is encoding a film beside the gate, and a retry passes.
- **Fix:** `testTimeout: 15_000` in `web/vitest.config.ts`, about three times the worst seen
  under the gate; a test that hangs still fails (the owner, 2026-10-12: *"Make the gate edits."*).
  Fewer vitest workers while the other checks run was the other way, and would slow every run.
- **Warning sign:** a web test that fails with "Test timed out in 5000ms" and passes alone; first
  tests of a file at 2 s or more in the gate's verbose log.

### 11. A fallback that widens to everything rather than to what a change can reach

- **Mechanism:** when a narrow run cannot settle a question, a gate falls back to the whole suite,
  though a change can reach only the code that imports it and the tests that read sources by path.
- **Instances** (check-pr's timings, 2026-10-13, 2,498 s against 300):
  - revert-red widened a Go file that survived its own package to `go test ./...`, and again with
    the dev tag, 55 s each; two of them were `doc.go` files no test could notice, 211 s;
  - it widened a TypeScript file that survived its importers' tests to the whole web suite, 69 s,
    which can add only the tests that read sources by path;
  - it probed the generated `api.ts`, types alone, by its importers' tests, nearly the whole suite;
  - `make check` ran the whole web suite, 60 to 125 s, for any change under `web/`;
  - the one browser run with every changed stylesheet reverted failed in 5 s for a reason nothing
    kept, and each of 13 sheets then took a 160 s run of its own.
- **Why it was invisible:** each fallback was correct, and each was cheap when it was written; the
  per-probe timings (`.git/revert-red/last-timings`) first showed where the time went.
- **Fix** (the owner, 2026-10-13: *"Our tests should be intelligent enough and compartmentalized
  enough that we can test only the parts actually affected by a change"*):
  `scripts/revert_red_dependents.py` gives a Go file's dependents (from `go list -test` under both
  tag sets) and the tests that read sources; `scripts/web_tests.py` runs `vitest related` over the
  files changed since the web tests' last pass; a package clause with comments is cosmetic; `api.ts`
  goes to the type check; `scripts/revert_red_stylesheets.py` settles stylesheets by halves and
  prints a failing run's last lines.
- **Warning sign:** a run kind named "whole suite" in revert-red's timings summary; a one-file change
  whose `make check` runs more than its importers' tests.

### 12. Every browser test run for a stylesheet only some of them show

- **Mechanism:** a stylesheet's revert was checked by the whole browser tier, both browsers, every
  spec, though a rule can only style a page that holds what its selector names, and most sheets
  style screens only some tests open. Imports cannot narrow it: every spec loads the whole client.
- **Instances** (check-pr's timings, 2026-10-13): the browser run with a batch's two changed
  stylesheets reverted took 429 s of revert-red's 431, beside the other probes; alone it takes about
  160 s.
- **Why it was invisible:** the whole tier was the only unit a browser run had.
- **Fix** (the owner, 2026-10-13, choosing a recorded map over excusing device-checked sheets):
  check-pr's full browser run records what every test's pages held, in both browsers
  (`web/e2e/browser-map.ts`), stamped with the commit only when every test passed on a clean tree.
  `scripts/browser_map.py` takes a changed sheet's rules apart, as sequences so a reorder counts, and
  a test is reached only if its pages held every token of a changed selector; revert-red then runs a
  sheet's revert in only those tests (`ORPHEUM_E2E_ONLY`), and in none when none is reached, which
  settles it as a run that turned nothing red would. Anything it cannot narrow (`:root`, `*`,
  `@keyframes`, `@font-face`, nesting, escapes) reaches every test, as does a missing or stale map.
  On the map of 2026-10-13, ten of the 33 sheets reach no test at all and most reach under half.
- **Warning sign:** revert-red's timings show a stylesheet's browser run with no `ORPHEUM_E2E_ONLY`
  while `.dev/browser-map/complete` names the commit checked; or `make test-browser` leaves no
  `complete` after a passing run.

### 13. A randomized test's seeds, raised to hunt and left raised

- **Mechanism:** a randomized test finds more the more seeds it runs, so a hunt raises its count;
  each seed costs a whole walk, and a count left raised is paid by every run after.
- **Instance:** identify's `TestPass_OverRandomLibrariesAgainWritesNothingAndMakesNoTitleEachPass`
  (2026-10-16) walks a random library through five scans per seed: 60 seeds per library kind took
  61 s, and found five faults that 6 did not; the suite runs 4 per kind, about 5 s.
- **Why it was invisible:** a seed count is a constant, which reads as cheap.
- **Fix:** the suite keeps the small count; a hunt raises it by hand
  (`for seed := range uint64(60)`), turns what it finds into a named test, and lowers it again in
  the same change.
- **Warning sign:** the identify package's test time grows by seconds in a commit that only
  changes a seed count.

### 14. A probe's browser run in the cheap tier, hanging for its whole timeout

- **Mechanism:** revert-red tried the browser tier last for each TypeScript file its unit tests did
  not notice, in check-pr's code tier, one browser run at a time. A file whose revert made the
  browser tests hang (the loader, the service worker) ran every test to its two-minute timeout,
  past the probe's own fifteen, while the cheap probes queued behind the browser lock. The timeout
  then raised out of the probe pool, ended every probe and named no file, and left the run's core,
  node and browsers running.
- **Instance** (check-pr over the D308 batch, 2026-10-17): 1632 s of revert-red ended in a
  traceback; the next run's `sw.ts` probe sat at 655 s of its browser run.
- **Why it was invisible:** a probe's runs are one row in the timings, and a run that never ends
  writes none.
- **Fix** (the owner, 2026-10-17: cheap tests first, no expensive test blocks a cheap one, fail
  fast on piecework, run to completion on the final run): the code tier takes only the cheap runs,
  and a file they leave green owes its browser run (`BROWSER_OWED`) to the last tier, after
  check-pr's own browser run. There it runs first in only the tests that loaded a chunk holding it
  (the browser map's scripts, `scripts/browser_map.py`), then, if none turned red, in every test,
  so the map never decides a green. Each browser run stops at its first failure
  (`--max-failures=1`), so a hang costs one test's timeout; a run past the probe's timeout is
  `TIMED_OUT`, a failure that settles nothing; and a probe's run is a process group of its own,
  ended whole (`scripts/revert_red_run.py`). The service worker's loader logic has unit tests
  (`sw.test.ts`), so its revert is red without a browser. A file a `Refactor-Only:` or ticked
  `Verified-By:` claim covers owes no browser run, since red or green the claim passes it; and a
  file the map narrows to more than half the tests skips the narrowed run, which a green revert
  pays on top of the whole one (`client-main.tsx` at nas.10: 441 s narrowed, then 448 s whole, then
  settled by its `Refactor-Only:`).
- **Warning sign:** a "browser tier" row in revert-red-code's timings; a `BROWSER_OWED` list that
  grows from batch to batch (a file only a browser can check is the expensive kind); a browser
  probe whose time is near its timeout.

### 15. The race detector on every core at once

- **Mechanism:** `go test -race ./...` runs as many package binaries as there are cores, and each
  runs its own tests in parallel under the race detector's five-to-tenfold cost. The packages
  starve each other, so a slow package passes go test's ten-minute default and a test with its own
  deadline misses it, though neither is slow alone.
- **Instance** (the first `make check-everything`, 2026-10-17): identify's tests ran past ten
  minutes and a `TestBackupInto` test passed its deadline; both pass alone under `-race`.
- **Why it was invisible:** the race run had been `make test-all`'s alone, run by hand on an idle
  machine; the nightly is its first regular caller.
- **Fix:** check-everything runs half the cores' packages at a time (`-p`) with a thirty-minute
  timeout, which bounds a hang and leaves room for one under contention (principles §24:
  parallelize only where it gains and starves nothing).
- **Warning sign:** a nightly race failure that is a timeout or a missed deadline rather than a
  race report; the race step's time growing faster than the suite.

### 16. check-pr's whole steps, whatever changed

- **Mechanism:** the integration tier, the uncached lint and the browser tier ran whole on every
  branch, so a change to one handler paid for every package's FFmpeg tests, every file's
  type-aware lint and every browser test.
- **Instance** (check-pr at nas.10, 2026-10-17): 41 s, 132 s and 396 s, for a batch whose Go
  changed in one package.
- **Why it was invisible:** each step was skipped for documentation alone, which read as narrowed.
- **Fix** (the owner: "check-pr only checks what is actually possible to break based on the changes
  made"): `scripts/impact.py` scopes each to what the change reaches: the integration packages
  whose tests build a changed package; the changed web files and what imports them, by
  dependency-cruiser; the browser tests that loaded a chunk holding a changed client source, by the
  last complete browser map, read from the map's own commit, with the tests whose pages drew what
  a changed stylesheet's rules select, and each changed spec whole. What it cannot place runs
  whole, and `make check-everything` runs all three whole each night.
- **Warning sign:** check-pr's steps log "may reach any" or "cannot place" for a change that
  touched one package; a browser map whose `complete` commit falls far behind HEAD (only whole runs
  renew it).

### 17. Every browser test for a change of types alone

- **Mechanism:** a TypeScript file whose change was types alone is in no chunk the browser map
  records, since compiling erases it, so both revert-red's browser run and check-pr's browser step
  could not narrow it and ran every test, though the browser runs the same JavaScript either way.
- **Instance** (2026-10-18): the SDK's generated `gen/api.ts`, given one optional field, took 507 s
  of revert-red's 620 s in the browser tier, and check-pr's browser step ran whole for it.
- **Why it was invisible:** "in no chunk the map can narrow to" read as the map's doubt, and the
  safe answer to a doubt is every test.
- **Fix:** `scripts/emitted_js.py` compiles both versions alone with TypeScript's `transpileModule`
  (the web code is `isolatedModules`, so a file's JavaScript depends on its text alone); a file
  that compiles to the same JavaScript takes no browser run in revert-red and reaches no test in
  `impact.py`. Any doubt (a parse error, no node) answers "different".
- **Warning sign:** a browser-tier row in revert-red's timings for a `gen/` or `types.ts` file; a
  check-pr browser step that "cannot place" a file of types.

### 18. Fixed waits in the browser tier, and one server for every test

- **Mechanism:** each of through-discovery's tests started a discovery and claimed the core again,
  ten claims a run, each waiting on the core's 10 s looks; two tests slept fixed seconds for voice;
  and the whole tier ran on one worker over one shared server, because two timed tests needed the
  machine alone and one of them needed to be the first to play its film.
- **Instance** (the browser test audit, 2026-10-18): through-discovery took 272 s of the tier's
  444 s; the tier had 43 tests, two of them timing-sensitive by nature.
- **Why it was invisible:** a claim's wait is inside a poll, and a test's time reads as its work.
- **Fix:** a newly claimed core dials its signalling connection at once; the plain
  discovery's tests share one claim (their spec's `beforeAll`); voice heard is waited for
  (`expectHeard`). The tests tagged `@timing` (`only-these-tests.ts`) run in a second pass of
  `make browser-tier`, one worker and a fresh server; the rest run first on two workers, each over a
  server of its own (the `server` fixture, `browser-map.ts`). The whole tier went from 444 s to 305 s,
  with the build. More workers did not pay: each server makes the same films' chunks beside the
  others, and four failed three tests by their waits.
- **Warning sign:** a browser test whose time is mostly a poll's; a test that passes alone and fails
  in the first pass (it depends on what ran before it on its server); a new `waitForTimeout`.

### 19. A stylesheet's check in the browser tier, behind a server and a sign-in

- **Mechanism:** a layout only a real engine computes (a logo's box, a tray's corner, a row's
  alignment) was checked by a browser test that started a server, signed in and navigated to draw
  one component; and revert-red could only settle a stylesheet with a browser-tier run, in its
  last, slowest part.
- **Instance** (2026-10-18): the shell's three layout tests took 1.5–5.5 s each per browser; the
  same checks in vitest's browser mode take about 0.1 s, past a fixed 9 s to start both browsers.
- **Why it was invisible:** happy-dom draws no layout, so the browser tier looked like the only
  place such a check could live.
- **Fix:** `*.browser.test.tsx` runs a component alone in Chromium and Firefox with no server
  (`web/vitest.browser.config.ts`, `make web-test-browser`), beside the other web checks in `make
  check` and only when `web/` changed since it passed. Revert-red tries a file's browser-mode tests
  (`vitest related`, which follows stylesheet imports) among its cheap runs, so a stylesheet's red
  is found in seconds and only a sheet they leave green owes a browser-tier run. The browser tier
  keeps what needs the server, the loader or two clients, and Firefox's WebRTC, whose ICE never
  starts inside vitest's page.
- **Warning sign:** a browser-tier test whose assertions are a layout's boxes or computed styles; a
  browser-mode test that imports a page needing the API (it belongs in the tier).

### 20. A probe's red run to its end

- **Mechanism:** a revert-red probe asks whether any test goes red, and one red answers it; but
  its runs (vitest, `go test`) ran every test after the first failure, the whole of a widely
  imported file's importers' tests.
- **Instance** (check-pr, 2026-10-20): reverting `locales/en.ts` turned five of 73 test files
  red in 34 s; stopped at its first, 6 s. Most of the batch's web probes were red, many at 40 to
  60 s, so its code part took 284 s.
- **Why it was invisible:** a red is the probe's success, so a slow red read as a check doing its
  job. The browser tier already stopped at its first failure (`--max-failures=1`); the cheap runs
  never did.
- **Fix:** every test run a probe makes stops at its first red: vitest `--bail=1`, `go test
  -failfast` (`revert_red_attempts.py`'s `FIRST_RED_VITEST`, `FIRST_RED_GO`). A green run still
  runs everything, so nothing is passed on less. Which red is met first may differ, so a file that
  both fails a test and breaks a build may be reported by either; both are covered. The spot check
  still runs to the end, to list every failure.
- **Warning sign:** a red probe whose time is close to its green suite's.

### 21. A probe tree's type check, cold every time

- **Mechanism:** revert-red's kept worktrees keep their ignored files between runs, yet the app's
  `.tsbuild` in them held no build info, so `tsc -b` there built the app from nothing. Why is not
  known: a red type check keeps its build info in the working tree, and the next run takes 5 s.
- **Instance** (check-pr, 2026-10-20): `client-main.tsx`'s type-check probe took 122 s beside
  three other trees' probes, the first tree's `web/apps/web/.tsbuild` empty; cold alone 22 s,
  warm 4 s.
- **Why it was invisible:** the probe trees are out of sight under the repository's own
  directory, and a type check is "only" its type check.
- **Fix:** each probe tree is given the working tree's `.tsbuild` outputs as it is set to the
  head (`revert_red_trees.py`'s `seed_type_builds`). `tsc -b` checks each source against the hash
  its build info records, so a stale copy costs only a rebuild of what differs, and an importer
  of a changed export still fails (measured, an SDK export renamed).
- **Warning sign:** a type-check probe slower than `npm --prefix web run typecheck` in the working
  tree after a small change.

### 22. A server that could not start, waited on for "ready"

- **Mechanism:** `startServing` waited for the server to log "ready", or for `serve` to return;
  a server whose start failed does neither, as it stays up degraded to report why, so the wait ran
  to its limit.
- **Instance** (2026-10-21): with the interest raiser wired without a part, a probe of
  `TestServe_WritesASessionsUseByTheTimeItStops` failed after 300 s, the server's own log saying
  why in its first quarter-second.
- **Why it was invisible:** it shows only when a change breaks the start, and then as one slow red
  among many.
- **Fix:** every captured log also watches for "the server could not start", and a wait for
  anything else fails at it (`cmd/orpheum/serve_test.go`'s `couldNotStart`): 0.25 s.
- **Warning sign:** a serve test red at exactly its wait's limit.

### 23. A batch's reds probed twice, and a query's callers reached last

- **Mechanism:** check-pr's revert-red probed every file of the batch again, though
  check-commit-red had already turned most of them red as each commit was made; and a generated
  query file's probe ran the store's integration tier and then every dependent package's tests,
  where the test that goes red is nearly always the one calling the query that changed.
- **Instance** (check-pr, 2026-10-21): 24 commits, 78 files, 754 s on 4 worktrees, 2921 s of
  probing; six sqlc files 110 to 291 s each, series.sql.go 148 s green in the store's tier before
  142 s of dependents found it red.
- **Why it was invisible:** a red is the probe's success, and a batch's probes are each a few
  minutes; only the summary's slowest files showed where the time went.
- **Fix:** a red revert is recorded with the file's two blobs and the git trees of what its red run
  tested, and a later probe of the same revert over the same trees takes it without running
  (`revert_red_known.py`); the nightly `make check-everything` trusts no record. A generated query
  file's probe runs first the packages that call the queries its change touched, and any other Go
  file's runs, after its own package's, the dependents that use an exported name its change touched,
  before every dependent (`revert_red_queries.py`; check-pr, later on 2026-10-21: a store file 37 s
  green in its package, then 177 s of dependents, its user's test seconds). Only a green run of all
  of them settles a green verdict.
- **Warning sign:** check-pr's revert-red over a batch whose commits each passed check-commit-red
  taking as long as it did before; a sqlc file's probe red only in "its dependents".

### 24. A helper that reconnects on every call

- **Mechanism:** `storetest.QueryRaw` opened a new SQLite pool for each query and closed it after,
  and a new modernc connection parses the whole schema (about 160 rows: tables, triggers, views)
  before its first statement: 3.4 ms of CPU against 40 µs for a query on a warm connection.
- **Instance** (audit 2026-10-25, G1): identify's tests dump every table a pass writes, table by
  table, step by step; `QueryRaw` was 40% of the package's CPU, which had grown from 6.4 s
  (2026-10-08) to 94–99 s under the full suite.
- **Why it was invisible:** the cost is per call and spread over 186 call sites, each one fast
  alone; only a profile shows the schema parse.
- **Fix:** a test's reads of one file share one connection, closed at the test's cleanup
  (`TestQueryRaw_ReadsAFileOnOneConnectionForTheWholeTest`): identify's test binary 43.6 s of CPU
  to 21.1 s.
- **Warning sign:** `_sqlite3InitCallback` or `sqlite3Prepare` high in a test package's profile.

### 25. A fixed deadline on CPU-bound work, under a loaded machine

- **Mechanism:** `TestBackupInto_CopiesWhileAWriteHoldsTheWriter` gives its backup a 5 s context:
  `VACUUM INTO` of the whole migrated schema and the copy's `integrity_check`, both CPU-bound, on
  the maintenance connection. Nothing in it waits on a lock: the write it holds is the one the
  backup must not wait for, and idle readers hold neither a lock nor a snapshot in WAL mode, so
  keeping them open (S1) cannot hold the copy or a checkpoint back.
- **Instance** (2026-10-25): it timed out once ("context deadline exceeded" after 9.5 s) in a
  whole-tree make check at a load average of about 90; it passed alone and in three package runs.
- **Why it was invisible:** the deadline is generous on an idle machine, and the failure looks like
  a lock wait, the thing the test exists to rule out.
- **Fix** (test architecture review, 2026-10-26, finding 3): raised to `testutil.ReceiveLimit`'s
  scale (a minute); a longer bound still catches a regression to a waiting backup, the thing the
  test exists to rule out.
- **Warning sign:** a backup or copy test red only in a loaded whole-tree run.

### 26. A browser walk's fixed waits, under other gates' load

- **Mechanism:** the browser-mode walks (`input-*.browser.test.tsx`) waited by deadlines set for
  an idle machine: every `expect.poll` by its own default of 1 s, a game controller's press by a
  fixed 50 ms hold that the page reads only once a frame, and the player's walk began before the
  media's failure had moved the page under it. A walk that timed out at the 15 s `testTimeout` went
  on pressing keys into the next test's page, which then saw seeks it never made.
- **Instance** (2026-10-25, the owner's queue: browser tests red when several gates run at once):
  four Firefox runs at once of the player, pages and walk files, at a load average of 40 to 75
  from the other agents' gates, failed 22 tests in 8 runs, 7 of the 8 red: polls past 1 s (a video's
  error, a list's order, a page drawn), the controller's presses never read (`focusedTile()` empty,
  BODY for SELECT), and the player's Fire TV walk timed out, its next test then seeing 30 or 50 for
  40. Each file passed alone, the player's 12 of 12 twice at a load of 60. With the fix, four
  such runs of four files passed 160 of 160.
- **Why it was invisible:** a deadline is generous on the idle machine it was set on, and the
  failure reads as the page's: a focus that never moved, a seek that went twice.
- **Fix:** a walk's polls wait up to 10 s (`expect.poll.timeout` in the browser-mode config); a controller's press
  is held until the page has read it held, and let go until it has read it go; the player's walks
  start once the video's load has failed (`theFailedVideo`); and a walk whose page is gone presses
  nothing more (`test-walk.test.tsx`).
- **Left as load:** the first long walks still pass the 15 s `testTimeout` under such load: the
  player's on a Fire TV's page took 15.2 s and 15.5 s (Chromium, Firefox) and a film page's 15.2 s
  in a whole browser-mode run at a load of 25 to 39, against a median of 10.7 s in Firefox's last
  twenty passes (5.5 s in Chromium); every press of a walk is a round trip to the browser. Later
  the same night it failed four of five whole-tree gate runs (23:49 to 23:56, both browsers), its
  Firefox median over twenty passes risen to 13.5 s, while alone it took 3 to 4 s for its 72
  presses. The owner agreed (2026-10-26) to two of the remedies: the browser-mode config's
  `testTimeout` is 30 s, and `make web-test-browser` takes a machine-wide `flock`
  (`BROWSER_MODE_LOCK`), so two gates' browser-mode runs never overlap; the wait for it shows as the
  step's own time (mechanism 7). Splitting the walks per page is left until one fails again.
- **Warning sign:** a browser-mode test red only when another gate runs; `expect.poll` or a fixed
  `setTimeout` in a walk; a walk's next test red after its own timed out.

### 27. A fixed real-time bound on a log wait, under a loaded machine

- **Mechanism:** `testutil.Log.AwaitLine` bounded its wait for a line by `LogLimit`, 5 s, while a
  passing test never waits for it at all (it returns the moment a matching line is written); only
  a run slow enough to miss that window pays the bound, and under contention a server's own slower
  start can miss it though nothing is wrong.
- **Instance** (test architecture review, 2026-10-26, finding 3): 28 `AwaitLine` call sites, the
  probable cause of probes failing with nothing reverted, each costing a rerun.
- **Why it was invisible:** 5 s reads as generous, and the bound never shows in a passing run's
  time: only a failing one pays it, and a failure under load looks like the code's fault.
- **Fix:** `LogLimit` is now `testutil.ReceiveLimit` (a minute), the same scale as every other
  channel wait a test makes; a hang still fails, just not from ordinary contention.
- **Warning sign:** a captured-log test red only in a loaded whole-tree run, with "no log line
  holds" rather than an assertion about the log's content.

### 28. A served test instance that starts as a new installation

- **Mechanism:** every cmd/orpheum test that serves (`startServing`) started from an instance
  default-config had just made: the core migrated persistent.db through every step from empty, and
  hashed each password at argon2id's production costs (64 MiB, three passes, four lanes), which
  the unit tests' `settingstest.Seeded` lowers but nothing lowered for a served instance.
- **Instance** (test architecture review, 2026-10-26, finding 7): 47 serves, 26 sign-ins
  among them; cmd/orpheum 71 s in a probe under load.
- **Why it was invisible:** both costs are production's own, so the tests looked like a faithful
  start; on an idle machine each serve's migration and hashes are a fraction of a second.
- **Fix:** `startServing` gives an instance with no persistent.db a copy of storetest's migrated
  template (`storetest.WriteMigrated`) seeded by `settingstest.Seeded`, whose lowered costs are
  rows a person set, so the server's sweep leaves them. One test still starts cold
  (`TestServe_ComesUpReadyAndStopsCleanly`, which now checks it migrated and wrote the starting
  values) and one checks the hash a new installation writes carries the production costs
  (`TestServe_ANewInstallationHashesPasswordsAtTheProductionCosts`). The package's test binary on
  an idle machine: 36–37 s of CPU to 25–26 s, 9.2–9.7 s to 7.2–7.8 s of wall time.
- **Warning sign:** a serving test's log full of `migrating` lines, or `argon2` high in a package's
  profile.

### 29. A web test helper's wait gave up within 2 s, or after a fixed count of empty ticks

- **Mechanism:** `test-dom.tsx`'s `settleUntil` bounded its wait to about 2 s, and `settleAnswers`
  (used by every `mount`, `click`, `submit`, `fire`, `choose` and `enter`) waited a fixed 5 empty
  timer-turns regardless of whether a request was still in flight or the DOM was still changing.
  A passing test never reaches either bound, so the only thing either number actually did was set
  how much slack a busy machine got before a real hang or a slow promise chain turned into a false
  red (`settleAnswers`) or a wait that gave up silently (`settleUntil`, before the audit of
  2026-10-25, W11, made it fail instead).
- **Instance** (test architecture review, 2026-10-26, finding 3): 102 `settleUntil` call sites and
  every DOM test via `settleAnswers`; proving the fix meant raising `settleUntil`'s own bound,
  which made its revert-red test (`test-dom.test.tsx`) pay the full 2 s, then the full 60 s, in
  real wall time until it was driven with vitest's fake timers instead.
- **Why it was invisible:** 5 empty ticks is enough on an idle machine for the microtask chains an
  ordinary click or mount produces, so `settleAnswers` "worked" without anyone measuring whether it
  was sufficient; 2 s is short enough that nobody minded paying it in a single test.
- **Fix:** `settleAnswers` now waits for `servedBy`'s in-flight request count to reach zero and the
  DOM to stop mutating, for a few consecutive ticks, rather than a fixed guess at how many ticks
  that takes; both it and `settleUntil` fail only at the scale of Go's `testutil.ReceiveLimit` (a
  minute), naming what was still in flight. Testing a minute-scale bound without waiting a minute
  needed vitest's fake timers advanced in small chunks in a loop, not one large
  `advanceTimersByTimeAsync` call, which otherwise left a loop awaiting React's `act()` never
  resolving on the path where the awaited condition resolves partway through (idiosyncrasies.md).
- **Warning sign:** a wait helper whose bound is a guess at "long enough" rather than derived from
  another limit already named elsewhere in the codebase; a test of a timeout that pays the real
  timeout in wall time rather than a faked one.

### 30. A Go file's probe running its whole package to find the one test that calls what changed

- **Mechanism:** revert-red's Go probe ran the file's whole package first, then its dependents', with
  no `-run`, though the test that goes red is nearly always one that calls what the change touched;
  and a red then pays a clean run of the same command (`revert_red_clean.py`), the whole package again.
- **Instance** (test architecture review, 2026-10-26, finding 1): `internal/core/api` 310 tests,
  32 to 81 s a probe; Go's probes 1319 s of one check-pr's 3044 s of probing. Measured the same
  day: api's `together.go`, a 19 s red and 118 s of clean run; identify's `sortkeys.go` 45 s for its
  package where the red test is one of three calling the changed method.
- **Why it was invisible:** a red is the probe's success, and a package's run was the smallest unit
  a Go probe had.
- **Fix:** `scripts/revert_red_naming.py` reads the file's changed declarations from its two
  versions (comments and layout aside), widens them by the package's own code that calls them, level
  by level, and takes the nearest level whose names some test's code names: the test function or the
  test files' helpers it uses, bare in an internal test, `pkg.Name` through an import elsewhere, a
  method by `.Name`. The probe runs its own package's such tests first, `-run` them alone, and its
  dependents' after its own package's whole run, before every dependent's; a package where they are
  more than half its tests is left to its whole run. Only a red is taken from them: a green goes on
  to every run the probe made before, so no verdict rests on the naming (the browser map's
  `tests_loading` orders the browser tier the same way). Warm cache, the same files probed by the
  old and new scripts back to back (2026-10-26): five files of an earlier commit's range 132 s of probing
  to 75 s (`together.go` 42 to 4 s, `sortkeys.go` 20 to 4 s); five of an earlier commit's 113 to 85 s.
- **Not taken:** a map of what each test reaches, transitively (a type reaching its methods): it
  reached 295 of api's 311 tests for any change in it, since every test builds the server through
  one harness and the server registers every handler. Recorded coverage: Go 1.26 keeps it per
  process (`-coverprofile` per package, GOCOVERDIR per process), so a per-test map takes a process
  per test, about 2000 runs each paying its package's set-up, apart from any run the gate makes and
  stale by the time a batch is probed; at a function's grain it narrows no better where the change
  is wiring (`server.go`'s `NewServer`, run by every api test, either way).
- **Warning sign:** revert-red's summary showing "go test, package" reds of a large package with no
  "the tests that name it" run before them, where the file's changed functions are called by name in
  a test; or a "the tests that name it" run that is mostly green before a red in the whole package
  (the naming misses how the tests reach the code: a route by its path, a value through an interface).

### 31. A locale file's probe widening to nearly the whole suite through `i18n.ts`

- **Mechanism:** revert-red scoped a changed locale file to `vitest related` over the file itself;
  103 of 167 app test files import `i18n.ts`, so "related" to a locale file was nearly all of them.
- **Instance** (test-architecture-2026-10-26, finding 4): `en-settings.ts` probes 85 s and 52 s,
  `status-tray.tsx` (the shell, a different fan-out) 81 s; the largest probe kind at 1120 s of one
  check-pr's run.
- **Why it was invisible:** "related" reads as narrow because it is an import graph, not the whole
  suite by name; only a barrel module's fan-in (test-weight.md §11's own warning sign) shows it is not.
- **Fix:** `scripts/revert_red_locales.py` diffs a locale file's two versions by their leaves' dotted
  paths (spreads followed to the files they import, read at the same revision) and matches each
  changed path against a literal `t("a.b")` in the app's source — the same pattern
  `locales/catalogue.test.ts` already uses to find what a file asks for. The source files found are
  run through `vitest related`, narrower than the locale file's own importers; the catalogue's own
  test runs alone first regardless, cheap and already catching a key gone missing. A key only ever
  built from a template literal, or not found at all, is not matched, and the file falls through
  unchanged to the existing whole-file scope — never a GREEN verdict resting on the scan, as the Go
  precedent (§30) never rests one on its naming. Measured against real commits (2026-10-26):
  `en-together.ts`'s D346 change (added keys, caught by the catalogue test alone) 3 s;
  `en-settings.ts`'s (keys asked for only through a built string, so unmatched and left to the old
  scope) 34 s, against the 85 s and 52 s above — the old scope itself measured faster here than in
  the review, which is the noise test-weight.md's own "What makes these hard to see" warns of, not a
  second fix; the gain the scan itself gives shows in `en-together.ts`'s and the unit tests
  (`scripts/tests/test_revert_red_locales.py`) built to prove the matching rather than only measure it.
- **Warning sign:** a probe's "vitest related" row for a `locales/` file's revert taking seconds more
  than its catalogue test alone; a locale commit whose changed keys are all reached only through a
  built string (`` t(`a.${camelKey(b)}`) ``), where the scan can place nothing and every such probe
  pays the old scope in full.

### 32. A worker spawned and torn down for every file, isolated or not

- **Mechanism:** vitest's own isolation starts a fresh worker, module registry and environment per
  file; a file whose module graph holds no mutable state worth isolating pays that startup anyway.
- **Instance** (2026-10-26): 218 files, about 334 ms a worker's own startup each (vitest's own
  hint); a `.test.ts` file with no `@vitest-environment happy-dom` pragma — plain, DOM-free, 97 of
  218 — never mounts a component, so nothing of its module state outlives the test.
- **Why it was invisible:** isolation's cost is each worker's own, which parallelism spreads thin; it
  shows only when many files share few workers, as a large `vitest related` batch does.
- **Fix:** `vitest.config.ts` splits the suite into two projects, extending the root config: "fast",
  the 97 plain `.test.ts` files (found by the absence of the pragma, read from the file itself, not
  kept as a list, so a file that starts using happy-dom moves itself back automatically), with
  `isolate: false`; "default", everything else, isolated as before. Measured: the 97-file subset 3.68 s
  isolated, 1.36 s not, stable across repeated runs, three shuffled orders and a single worker running
  every one of them in sequence; the whole suite's wall time dropped a modest, repeatable amount once
  caches warmed (back-to-back, 2026-10-26: about 29.2 s against 27.5 s). Running `--no-isolate` over
  the *whole* suite, as a check of the risk being traded away, failed a file nothing about it touches:
  a happy-dom file's mounted components kept firing effects into the next file sharing its worker,
  inflating a `console.error` spy's call count from 1 to 6 — exactly what the pragma-based split is
  built to keep out of the fast project. `make check-everything` (the nightly) also runs
  `npm run test:isolated` (`vitest run --isolate`, overriding both projects), the whole suite with
  every file isolated, trusting none of the above.
- **Not taken:** `pool: 'threads'` in place of the default `forks` — measured slower (27.9 s against
  22.9 s for the whole suite) and a higher per-worker startup (427 ms against 334 ms) on this machine.
- **Warning sign:** a `.test.ts` file added to `vitest.config.ts`'s "fast" project's reach that
  mounts a component or opens a module-level cache without resetting it per test — vitest's hint
  line ("Isolate N workers spawned … faster with isolate: false") naming a large N with few of the
  files near vitest's `testTimeout`, which the default project's own isolation still protects.

## What makes these hard to see

- **Parallelism hides cost as contention.** A step that adds 30 s of CPU adds no visible time of
  its own; everything around it gets slower. Look at CPU time, not just wall time.
- **Totals are noisy.** The same run measured ±30% depending on what else ran. Compare runs back
  to back, and measure the step alone and inside the gate.
- **The cost lands where no one looks:** global setup, a gap between log lines, the wait for a
  lock. Test reporters time tests, not the time around them.
- **Each addition is reasonable.** The fix is rarely to undo one. It is to stop paying for the
  same work twice: cache what is fixed, select what is needed, and run only what changed.

## How to look

- **check-pr over budget** (300 s, A24): find the step that grew. The phases are the timestamps
  in `.git/gate-logs/check-pr-verbose.log`.
- **Go:** `go test -count=1 -json ./...`. Sort tests by elapsed time; compare elapsed with CPU.
- **Vitest:** `vitest run --reporter=json` gives per-file durations. The summary's split
  (environment, transform, import, tests) shows how much is test bodies.
- **A suspected wait:** take a goroutine dump during the slow test, or read where it is parked.
  That is how the 10 s tick was found.
- **Every so often, at a milestone audit:** rerun the audit's measurements and compare.

## Caches bring their own risk

The fixes above skip work. A skip whose key misses an input passes a run that should have
failed.

- **`area_stamp.py`** trusts its pathspecs. If a step comes to read something outside them, add
  it there. CI and every fresh clone have no stamps and run everything, which is the backstop.
- **The film cache** keys on FFmpeg's version and every argument, with any file argument keyed by
  its contents. A film made some other way, such as from a file it reads that is not named in its
  arguments, needs its own key.
- **`vitest related`** follows imports. A test that depends on a file without importing it (by
  reading it from disk, say) is not related to it: such tests are found by what they call
  (`readFileSync`, `readdirSync`, `import.meta.glob`, `web_source_readers`) and always run with the
  related ones. A new way of reading files needs adding there.
- **A Go file's dependents** are those whose code or tests import its package. A test that reads
  every package's sources is named in `GO_SOURCE_READERS` (today `internal/archtest`); one that
  builds the binary is found by its `go build`. A new reader of the sources needs adding there.
- **`web_tests.py`** trusts its tree record as `area_stamp.py` does, and runs the whole suite when a
  manifest, the lockfile or a configuration changed, or a file was deleted.
