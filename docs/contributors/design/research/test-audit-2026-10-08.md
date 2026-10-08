> **ABOUTME:** An audit of the test suite for redundancy and turnaround: what `make check` and
> `make check-pr` spend their time on, measured per tier, package, test and file; tests that check
> the same behaviour twice; real-clock waits; and ranked recommendations with estimated savings,
> each marked by whether it changes a gate or weakens a test. Gathered 2026-10-08.

# Test audit: redundancy and turnaround

Audited **2026-10-08** at an earlier commit, the commit the last `make check-pr` passed on (278 s). The
owner asked for two things: redundant testing, and ways to make the dev loop faster.

Rules that bound the answers:

- **Rule 4 / D190.** Every behaviour change keeps a test that fails when it is reverted. A merge
  or removal below names the test that still fails.
- **Rule 5.** A removed or weakened test needs the owner's `Test-Weakened:` trailer. Every
  removal here is a proposal.
- **Rule 2.** `Makefile`, `scripts/`, `internal/archtest/` and `web/vitest.config.ts` are gates.
  A change there needs the owner's `Gate-Changed:` trailer.

## How this was measured

- **Machine:** the 8-CPU sandbox. Staging was up, and other agents ran gates at the same time.
  The load average was between 3 and 49 during the runs.
- **Contention inflates absolute times.** Treat single numbers as ±30%. Comparisons were run
  back to back.
- **Go:** `go test -count=1 -json ./...` (unit tier) and the same with `-tags integration`
  (integration tier). Each was run on an extract of `HEAD` in scratch space, so another agent's
  edits in progress could not break the build.
- **Web:** `vitest run --reporter=json`, `tsc -b`, `eslint`, `depcruise` and `prettier`.
- **Python:** `scripts/py_tests.py`, and each module on its own.
- **check-pr:** split into phases by the timestamps in `.git/gate-logs/check-pr-verbose.log` and
  `.git/check-pr/`. These phase times are **derived**, ±5 s.
- **Fixture films:** timed by running the browser tier's own FFmpeg commands.
- **Not run:** `make check` and `make check-pr` (another agent was editing the tree). Their
  totals come from the logs, or are estimates.
- **One test change was measured without editing the file**, using `go test -overlay`.

## Executive summary

**Most of the time goes to waiting and regenerating, not to testing.** Five findings account for
most of it:

1. **A 10 s node tick decides `cmd/orpheum`'s time.**
   - Seven tests each start a core and an in-process node together. The node's first hello
     loses the race to the core's socket, and it waits one status tick, which `testConfig`
     sets to 10 s.
   - With a 1 s tick, measured through an overlay, the package fell from **17 s to 4.7 s**.
     Every test still passed.
   - `cmd/orpheum` depends on nearly everything, so almost every Go change pays this in
     `make check`. It is paid again in the integration tier and in revert-red.
2. **The browser fixtures are re-encoded on every run, and the acceptance run makes four
   films it never plays.**
   - Each 90 s 1080p timing film takes about **20 s** of FFmpeg, and every browser-tier run
     makes two of them.
   - `test-acceptance-home` calls the same `startServer`, so it makes them too, then plays
     only its own film. That is about **35 s of check-pr's serial path**.
3. **The integration tier re-runs the whole unit suite, uncached.**
   - Only 14 of 1,181 tests carry `//go:build integration`.
   - `go test -tags integration -count=1 ./...` re-runs the other 1,167, which `make check` has
     already passed on the same code: no non-test file has the tag.
   - Measured: **37.7 s and 2 m 10 s of CPU** for the whole tier, against **19.6 s** for the 14
     tests alone. That CPU is taken from the timing-sensitive browser tier running beside it.
4. **Revert-red runs the whole web suite once per TypeScript file.** Its Go probes start at the
   package. In the last check-pr, revert-red took about **54 s** for 8 files, 5 of them
   TypeScript.
5. **`make check` re-runs work whose inputs did not change.**
   - The Go side is cached (test cache, vet, golangci-lint, tsc and the eslint cache).
   - Not cached: the whole vitest suite (**35 s** in the last gate log), the gate scripts' tests
     (11 s), depcruise (13 s), check_docs (7 s) and check_claims (8 s). They run on every
     Go-only turn too.
   - After a Go change, `lint_speculative_api.py` took **36 s**, running its two platforms one
     after the other.

**Redundancy is modest.** The integration tier's re-run is the largest. Beyond it:
- four of the seven slow `cmd/orpheum` tests can share server starts;
- one `internal/core/jobs` test is a strict subset of a `jobqueue` test;
- two browser-tier tests run in Firefox for no browser-specific reason;
- two vitest tests each sleep 1.05 s of real time to reach a stall.

**Estimated effect, if the no-gate-change items alone are done:**
- `check-pr`: about **278 s → 200 s**;
- the Go side of `make check` after a Go change: about **15 s** shorter.

**With the gate changes as well (the owner's call):**
- `check-pr`: about **150–170 s**;
- `make check` on a Go-only turn: about **35 s** less CPU contention.

All of these are estimates.

## 1. Where the time goes

### 1.1 `make check`

`check-verbose` runs `check-tools`, `web-deps` and `go-warm` first, then every check in parallel
(`Makefile:54-62`, `-j$(nproc)`). The last gate log (`.git/gate-logs/check-verbose.log`) shows the
web tests finishing last. The 90 s budget (`standards/testing.md` §10) is review-only.

| Step | Measured | Cached when inputs are unchanged? |
| --- | --- | --- |
| `go-warm` (`go list -export -deps -test`) | 0.5–0.9 s warm | yes (build cache) |
| `go-test` (`go test ./...` + the dev-tag pass) | 24.7 s uncached; **29 s after a one-file change** in `internal/core/together` (cmd/orpheum ~23 s of it) | yes, per package |
| `go-lint` (two golangci-lint runs) | 2–4.5 s warm; 16 s after that change | yes |
| `go-vet` (five passes) | 29 s cold; 2.2 s after that change | yes |
| `go-speculative` (`lint_speculative_api.py`) | 2–3 s warm; **36 s after that change** (55 s in the live tree under load) | yes, but two platforms run **serially** (`scripts/lint_speculative_api.py:155-157`) |
| `web-test` (vitest, 95 files, 481 tests) | 16–18 s alone; **35 s** in the last gate log | **no**: every file, every time |
| `web-typecheck` (`tsc -b`) | 12 s cold, 2 s warm | yes (incremental) |
| `web-lint` (eslint `--cache`) | not measured warm; 105 s uncached under load | yes |
| `web-deps-check` (depcruise) | 13 s | no |
| `web-format-check` (prettier `--cache`) | 8.3 s uncached | yes |
| `check-scripts` (`py_tests.py`, 16 modules in parallel) | 11 s | no |
| `check-docs` / `check-claims` | 7.2 s / 7.8 s | no |
| `check-source`, `check-deps`, `check-architecture` | 1.8 s, 0.6 s, 0.1 s | no |

Vitest's own breakdown (last gate log): **environment 30%, transform 25%, import 24%, tests 20%**.
The tests' own bodies take 16.8 s, summed over files. The rest is starting environments and
transforming modules again, and vitest reports the transform as "re-done on every run".

The pre-commit hook does not re-run the gate on a tree it just passed (`scripts/check_stamp.py`).
That is already right.

### 1.2 `make check-pr` (278 s on 2026-10-08)

Derived from the log timestamps. `test-integration`, `test-browser`, `web-lint-full` and the
cheap branch checks run in parallel (`Makefile:194`). The home acceptance then runs alone, then
revert-red.

| Phase | Approx. | What dominates |
| --- | --- | --- |
| Browser tier: `npm run build` + `go build -tags dev` | 0–31 s | contended by the integration tier's compile and the uncached eslint |
| Browser tier: global setup (`web/e2e/server.ts`) | 31–82 s | **making four films with FFmpeg**, two of them 90 s at 1080p |
| Browser tier: 13 tests, one worker | 82–139 s | test bodies sum to 49 s; voice test 17.6 s |
| (integration tier, web-lint-full) | inside the above | integration: `cmd/orpheum` 53 s, `store` 34 s, `api` 24 s in the log |
| Home acceptance: build + global setup | ~139–179 s | **the same four films again, unused**, plus its own (7 s) |
| Home acceptance: the test | ~179–224 s | 37 s |
| Revert-red | ~224–278 s | 5 TypeScript files × the whole vitest suite, 4 trees at once |

Fixture costs, measured one at a time: warm-up film 0.35 s; prepared film 0.2 s; **each timing
film 20.3 s** (38 s under heavier load); acceptance film for 0.5 min 7.1 s.

### 1.3 The slowest tests

**Go, unit tier** (elapsed under `-parallel 8`, so inflated by parallelism):

| s | Test | Why |
| --- | --- | --- |
| 11.2 | `cmd/orpheum` `TestPlayback_ACutFilmStartsOnItsLadderAndItsStallsStartNothing` | 10 s tick (§2.1) |
| 10.9 | `TestPlayback_ARenditionOnTheNodesDiskIsAdoptedAndAnUnfinishedOneEvicted` | 10 s tick |
| 10.9 | `TestPlayback_AFilmsSubtitleIsExtractedOnItsNodeAndServedByTheCore` | 10 s tick |
| 10.8 | `TestPlayback_AFilePlaysFromTheNodesSocketThroughTheCore` | 10 s tick |
| 10.8 | `TestPlayback_AFilmIsMeasuredOnItsNodeAndPlaysLevelled` | 10 s tick |
| 10.7 | `TestRunNode_TheNodeScansItsRootAndTheCoreHearsEveryRecord` | 10 s tick; **0.87 s of CPU over 10.15 s alone** |
| 10.6 | `TestRunNode_AFilmInATMDBLibraryIsQueuedForItsMatch` | 10 s tick |
| 4.0–6.9 | `TestScenario_S25_ATMDBOutageOvernightParksAndMatchesAllByMorning` | contention |
| 3.8–4.9 | `internal/archtest` `TestArch_EachTableHasOneWritingPackage` | `go list -export` + type-checking (gate code) |
| 2.9–4.1 | `internal/core/api` `TestSubtitleChoice_TheLastLanguageChosenFollowsTheSeriesUnwatchedEpisodes` | contention; **0.35 s alone** |
| 3.2–3.5 | `internal/core/chunks` `TestHandleMakeChunks_EachEndIsTakenAsAPreparesIs` | — |
| 2.4–2.8 | `cmd/orpheum` `TestCanary_…`, `TestServe_TheSystemPageShows…`, `TestRunNode_AListingStalled…` | whole-server starts |
| 2.4 | `internal/core/backup` `TestRestore_…` (two) | — |
| 2.1–2.4 | `internal/node/agent` `TestAgent_AResultIsSentUntilTheCoreAcknowledgesIt` | — |

Packages by wall time (unit tier): `cmd/orpheum` 17–23 s, `internal/core/api` 15–18 s,
`internal/core/identify` 6.4 s, `internal/archtest` 4.7–11 s, everything else under 5 s.

- `internal/core/api`: 189 tests; serially each is about 0.1 s.
- Its cost is mostly SQLite statement parsing under `newHarness`: `sqlite3Prepare` is 30% of
  the package's CPU profile. The suite already copies one migrated template database
  (`internal/store/storetest`), so there is no cheap fix there.

**Go, integration-only tests:**
- `TestM2_EveryFileHasAFilingAndAReasonAndTheCoreNeverOpensAMediaPath` measured 5.5 s, 7.1 s
  and 17.7 s in three runs. The spread is the hello race of §2.1.
- `TestReferenceScale` takes 11–12 s.
- `TestMake_TwoRunsChunksJoinIntoOneStreamAtTheFilmsTimes` takes 4 s.

**Vitest files:**

| s | File | Tests | Why |
| --- | --- | --- | --- |
| 2.03 | `apps/web/src/together-viewer.test.tsx` | 16 | a **1.05 s real sleep** at :264 |
| 1.85 | `apps/web/src/watch-together.test.tsx` | 11 | the same sleep at :347 |
| 1.10 | `apps/web/src/request-wait.test.tsx` | 4 | `shouldAdvanceTime` fake timers, which advance in real 20 ms steps (:18) |
| 0.81 | `apps/web/src/app.test.tsx` | 7 | — |
| 0.69 | `apps/web/src/screen-refusals.test.tsx` | 11 | — |
| ≤0.62 | the other 90 files | | |

**Playwright** (last check-pr):
- slowest is the voice test, 17.6 s, chromium only;
- the play-timing MKV test takes 4.6–4.8 s per browser;
- the invitation test takes 4.2–4.5 s per browser;
- everything else is under 3.5 s.

**Python** (each module alone, under load):
- `test_branch_gates.py` 19.8 s (50 tests);
- `test_gate_reruns.py` 16.6 s;
- `test_quiet_gate.py` 10.7 s;
- `test_hooks.py` 8.3 s;
- the rest under 2.2 s.

In parallel the whole set takes 11 s.

## 2. Findings

### 2.1 The node's first hello waits a 10 s tick in seven Go tests

- **The cause.** `startServingOn` (`cmd/orpheum/serve_test.go:201`) starts the node in the same
  instant as the core. The node's first hello reaches no socket.
  - `Agent.Run` (`internal/node/agent/agent.go:162-172`) logs "lost the core" and waits for the
    next tick.
  - The tick is `time.NewTicker(StatusIntervalSeconds)` (`cmd/orpheum/node.go:173`), and
    `testConfig` sets it to 10 (`cmd/orpheum/serve_test.go:151`).
- **The evidence.**
  - A goroutine dump taken 6 s into `TestRunNode_TheNodeScansItsRootAndTheCoreHearsEveryRecord`
    shows the agent parked at `agent.go:169` (`case <-tick`).
  - `TestRunNode_AListingStalledOnItsDiskMakesItsRootUnavailable` sets the tick to 1
    (`node_test.go:437`) and takes 2.3 s.
- **The browser tier already avoids the race.** `web/e2e/server.ts:346` starts the node only
  once the core answers, "so its first hello does not wait for a status tick". The Go helpers
  never got the same treatment.
- **The measurement.** `testConfig` with `StatusIntervalSeconds: 1`, through `go test -overlay`:
  - the package took **4.7 s against 17 s**, and all tests passed;
  - the seven tests fell to 1.3–1.7 s each.
- **Why it matters beyond speed.**
  - It is a real-clock dependency of the kind rule 16 forbids. Whether a test pays 10 s depends
    on scheduling, so `TestM2_…` measured anywhere from 5.5 s to 17.7 s.
  - It also breaks `standards/testing.md` §10: "a unit test that takes more than about a second
    belongs in a tag or needs a smaller input".
- **A product question for the owner, not a test change.** A real node that starts before its
  core also waits a full tick before its first hello. That is up to 10 s at every start, staging
  included. The fix would be to retry the first hello sooner. It is a behaviour change that
  needs its own test, so it is outside this audit.

### 2.2 The integration tier re-runs the unit tier

- **The duplication.**
  - `make test-integration` is `go test -tags integration -count=1 ./...` (`Makefile:274-277`).
  - The tag adds 14 tests in 12 files, and no non-test file carries it. The remaining 1,167
    tests are the same code `make check` ran, and the pre-commit stamp guarantees that pass.
- **The cost.** 37.7 s and 2 m 10 s of CPU in the full tier, against 19.6 s for only the 14
  tagged tests (by `-run`, the way `DEV_ONLY_TESTS` already selects the dev-tag tests,
  `Makefile:136`).
- **What the re-run catches.** Only flakes, by running everything a second time uncached. That
  is a property of the test, not of the integration tier. `make test-all` (`-race -count=1`) is
  the target meant for that.

### 2.3 The browser fixtures

- **Re-encoded every run.**
  - `makeFilms` (`web/e2e/server.ts:287`) runs FFmpeg synchronously (`execFileSync`) for the
    warm-up film, the prepared film and **two 90 s 1080p H.264 + TrueHD + ASS timing films**
    (`timingFilmArguments`, :101).
  - Every argument is fixed, so the outputs are the same every run.
  - Each run starts a fresh server instance, so caching the source files cannot leak a prepared
    copy from one run into the next. "One per browser" is about the per-run node and is kept.
- **Unused in the acceptance run.**
  - `acceptanceSetup` (`web/e2e/acceptance-server.ts:144`) calls `startServer`, which always
    makes those four films.
  - `acceptance.spec.ts` plays only `acceptanceFilm(profile)` (:186, :230).
- **Serial.** The two timing films are made one after the other. FFmpeg used about 2 cores
  here, so making them in parallel would roughly halve the time.

### 2.4 Revert-red's TypeScript scope

- **The scope is the whole suite.** `scope_for` (`scripts/check_revert_red.py:351`) returns
  `npm --prefix web run test`, all 95 files, for every TypeScript file.
- **Go starts narrower.** A Go file starts at its package and widens to `./...` only if it
  survives (`wide_scope`, :356).
- **The cost.** Revert-red runs 4 trees at once (`:724`), each on a full vitest run that is
  itself parallel. The last run took about 54 s for 5 TypeScript files.
- **The fix.** `vitest related <file> --run` (vitest's import graph) is the TypeScript
  equivalent of the package scope.
  - Kept with the whole suite as the widening step, it reports the same outcome for every file.
  - A red is found sooner, and only a survivor pays for the full run.
  - Go probes into `cmd/orpheum` also pay the 10 s tick of §2.1.

### 2.5 `make check` re-runs unchanged web and script checks

- **Cached already.** Go's test cache, vet, golangci-lint, tsc's incremental build, and the
  eslint and prettier caches.
- **Not cached.** These run on every turn that touched any file, whatever it was:
  - vitest (35 s in the gate);
  - depcruise (13 s);
  - `py_tests` (11 s);
  - check_docs and check_claims (7–8 s each).
- **Why it matters.** On an 8-CPU machine they compete with the Go steps that actually changed.
- **The remedy.** A per-area stamp, like `check_stamp.py`'s for the whole tree, would skip
  `web-test` and `web-deps-check` when nothing under `web/` changed since the last pass. It
  would skip `check-scripts` likewise for `scripts/`.

### 2.6 `lint_speculative_api.py` after a Go change

- It runs golangci-lint's `unused` twice, for linux/amd64 then darwin/arm64, in a `for` loop
  (`scripts/lint_speculative_api.py:155-157`).
- Warm, that is 2–3 s.
- After a one-file change in `internal/core/together`, it was **36 s**, the longest Go step in
  that run. The two platforms are independent, so running them at once should save close to
  half.

### 2.7 Real-clock waits (rule 16, `standards/testing.md` §12)

| Where | Wait | Verdict |
| --- | --- | --- |
| `cmd/orpheum/serve_test.go:151` with `node.go:173` | a real 10 s ticker | **flag**: §2.1 |
| `web/apps/web/src/together-viewer.test.tsx:264` | `setTimeout(stallAfterMs + 50)`, 1.05 s real | **flag**: the stall detector runs on `setTimeout`, and `packages/player/src/stalls.test.ts` already advances fake time |
| `web/apps/web/src/watch-together.test.tsx:347` | the same, 1.05 s | **flag** |
| `web/apps/web/src/request-wait.test.tsx:18` | `shouldAdvanceTime: true` with `settleAnswers` (`test-dom.tsx:191`): five real 20 ms ticks per call, ~0.3 s a test | minor; `advanceTimeDelta: 1` would cut it |
| Eight `time.Sleep(1–10 ms)` poll loops: `internal/node/agent/answer_test.go:139`, `devices_test.go:125`, `internal/node/execx/watch_linux_test.go:122,143,161,272,296`, `internal/core/nodes/connection_test.go:60` | bounded polls on a real goroutine, process or request, each with a deadline and a `nolint` naming why | acceptable |
| `cmd/orpheum/playback_test.go:118` `pollUntil` | 50 ms polls, 30 s bound | acceptable once §2.1 is fixed |

### 2.8 Parallelism and runner settings

- **Go.** Every package with more than a handful of tests marks nearly all of them
  `t.Parallel()` (counted from the JSON run's `run` and `pause` events). Nothing slow is left
  serial.
  - `go test` runs packages 8 at a time at `-parallel 8`.
  - `internal/core/api` ran in 4.6 s at `-parallel 32` against 8.5 s at the default. But that
    was under heavy outside load, where more goroutines simply win more CPU. It needs an idle
    trial before it is worth anything.
- **Vitest.** `web/vitest.config.ts` keeps the defaults: the `forks` pool, isolation on, every
  file run.
  - 39 of 96 files use happy-dom, and each needs the DOM, so none can move to the cheaper node
    environment.
  - `--pool=threads` was no faster (17.5 s against 16.1 s).
  - `--experimental.fsModuleCache` ran warm in 12–14 s against 16–18 s.
    - But in this audit's two-tree setup it failed with stale-module errors.
    - It is experimental, and needs a clean trial before anyone relies on it.
- **Playwright.** `web/e2e/playwright.config.ts` sets `workers: 1`, `fullyParallel: false` and
  `retries: 0`.
  - One worker is deliberate: the timing tests are the point, and two prepares would share a
    node.
  - No retries is right: a retry would hide a flake.
  - So the browser tier saves time through less work, not more workers.
- **Test cache.**
  - `-count=1` appears only where a fresh run is the point: `test-all`, `test-one`, `cover` and
    `test-integration`. `make check` uses the cache.
  - Only `test-integration`'s `-count=1` costs the gate, and only because of §2.2.

### 2.9 Redundant tests

None is pure dead weight. Each proposal below names what still fails on a revert.

**Go, `cmd/orpheum` (each test starts a whole core and node):**

- `TestPlayback_ACutFilmStartsOnItsLadderAndItsStallsStartNothing` (`stall_test.go:21`):
  - Its stall assertion repeats `internal/core/api` `TestReport_AStallOnTheLadderIsOfferedNothing`
    (`stall_report_test.go:150`).
  - Its ladder URL repeats `TestPlayback_ACutFilmStartsOnItsLadderAndPreparesNoCopy`
    (`ladder_test.go:17`).
  - Its node-made timeline is asserted in `TestPlayback_AFilmIsMeasuredOnItsNodeAndPlaysLevelled`.
  - It adds one thing: a timeline the node really made gives the 2400 kb/s quality on the
    ladder, a units check across node and core.
  - **Proposal:** fold that assertion into `AFilmIsMeasured…` and drop the test. That saves one
    server start. The api tests still fail on a revert of the stall or ladder logic.
  - **Needs Test-Weakened**, because a test is removed.
- **Merge three starts into one, deleting no assertion:**
  - `TestRunNode_TheNodeScansItsRootAndTheCoreHearsEveryRecord` (`node_test.go:33`),
    `TestPlayback_AFilePlaysFromTheNodesSocketThroughTheCore` (`playback_test.go:25`) and
    `TestPlayback_ARenditionOnTheNodesDiskIsAdoptedAndAnUnfinishedOneEvicted`
    (`playback_test.go:191`) build the same `filmOnNode(t, "nfo")` fixture and wait for the
    same log line. Their assertions do not overlap.
  - Merged, every assertion still runs. It saves two starts, about 20 s of CPU, but little wall
    time, as they run in parallel.
  - The merge is a test restructure: `check-test-weakening` may count the removed functions, so
    the commit should say so.
- **Not redundant:**
  - `TestPlayback_AFilmsSubtitleIsExtractedOnItsNodeAndServedByTheCore` (`/jobs`, the doctor's
    confinement check, the real extraction path);
  - `TestRunNode_AFilmInATMDBLibraryIsQueuedForItsMatch` (it needs the TMDB source);
  - `TestM2_EveryFileHasAFilingAndAReasonAndTheCoreNeverOpensAMediaPath`, the only runtime proof
    that the core never opens a media path, and cut-a's M2 acceptance.

**Go, `internal/core/jobs` against `internal/platform/jobqueue`:**

- `TestRequeueRunning_ReturnsWhatACrashLeftRunning` (`internal/core/jobs/jobs_test.go:173`) is a
  strict subset of `TestRequeueRunning_ReturnsWhatAStopLeftRunningWithItsAttemptCounted`
  (`internal/platform/jobqueue/queue_test.go:418`). Removable, though it saves only
  milliseconds. **Needs Test-Weakened.**
- `jobqueue` `TestDemoteUnwanted_GivesTheJobsNoOneWaitsForAnyMoreTheirNewPriority`
  (`wanted_test.go:15`) nearly copies `internal/store` `TestJobStorage_KeepsAJobWantedAndDemotesItOncePast`
  (`jobs_test.go:321`).
  - They use the same targets and the same times.
  - `TestReleaseWanted_ANoLongerWantedJobFallsAtTheNextDemotion` and
    `TestJobStorage_AReleasedJobIsDemotedAtOnce` are the same pair.
  - The `jobqueue` copies add only the queue supplying its clock, and a second look that demotes
    nothing.
  - Low value either way; leave them unless the owner wants fewer tests to maintain.
- `TestScenario_S71_ARepeatRequestRaisesWakesAndMerges` (`internal/core/jobs/scenario_test.go:206`)
  repeats four `jobqueue` `TestEnqueue_…` cases. It is a named scenario, so keep it.

**Web:**

- The two 1 s stall tests partly overlap:
  - `together-viewer.test.tsx:227` is the only app-level check that a stall reaches the report
    body. Keep it, on fake time.
  - `watch-together.test.tsx:305` really checks that no command is sent and the socket opens
    once. It can reach "a lower quality" without a stall, the way `play-state.test.ts:109` does.
  - "moves to a lower quality" is asserted three times. `play-state.test.ts:142` is the one that
    matters.
- **Firefox runs of server-side behaviour** (`web/e2e/playwright.config.ts` runs every spec in
  both projects):
  - `play-timing.spec.ts:42`, "the System page shows the machine's hardware encoding and what a
    prepared film's prepare used", checks the node's probe and the prepare listing. Both are
    server-side, and the test has a `toPass` of up to 90 s. Chromium alone would do.
  - `together.spec.ts:64`, "an invitation works once…": "works once" is the hub's
    (`internal/core/together/hub_test.go:340`). The parts no vitest covers (the HAR sweep, the
    reload before joining, the button's geometry) do not depend on the browser.
  - Restricting either test to chromium is a narrowing. It **needs Test-Weakened**, and it is
    the owner's call: this is the browser tier D217 §4 defines.
- **Not redundant:** the sdk tests (`together-follow`, `together-drift`, `together-sound`,
  `together-connection`) against the app tests. Each pair is a unit test and its wiring, and
  reverting the wiring fails only the app test.
- **Not redundant:** the table tests. The large tables (`identify/filename_test.go`, `langtag`,
  `decide`, `api/reach_test.go`) have at most one or two rows in the same equivalence class,
  each costing microseconds.

## 3. Recommendations, ranked

Savings are estimates, except where marked "measured".

| # | Recommendation | Saves | Gate? | Test-Weakened? |
| --- | --- | --- | --- | --- |
| 1 | Fix the hello race in `cmd/orpheum`'s helpers: start the node once the core logs "ready", as `web/e2e/server.ts:346` does, or set `StatusIntervalSeconds: 1` in `testConfig` (`serve_test.go:151`) | **measured: 12 s off the package (17 → 4.7 s)**, and 7 tests stop depending on the real clock. Pays in `make check`, the integration tier, and revert-red's Go probes | no gate change | no |
| 2 | Let `test-acceptance-home` make only its own film: give `startServer` a way to skip the shared films (`web/e2e/server.ts:287`, `acceptance-server.ts:144`) | ~35 s of check-pr's **serial** path | no gate change | no |
| 3 | Cache the browser fixture films across runs, keyed by FFmpeg's version and the arguments (for example under `.dev/`), or at least make the two timing films at once | ~40 s cached, ~20 s in parallel, off check-pr's critical path; also every revert-red browser-tier attempt. Costs ~95 MB of disk if cached | no gate change | no |
| 4 | Fake timers for the two stall tests (`together-viewer.test.tsx:264`, `watch-together.test.tsx:347`) and `advanceTimeDelta: 1` in `request-wait.test.tsx:18` | ~3 s of real sleeping; removes two rule-16 breaches | no gate change | no; the assertions stay |
| 5 | Merge the three `filmOnNode("nfo")` starts in `cmd/orpheum` into one test | ~20 s CPU, little wall time | no gate change | say so in the commit; no assertion is lost |
| 6 | Run only the 14 integration-tagged tests in `test-integration`, selected by name as `DEV_ONLY_TESTS` is (`Makefile:136`, `:277`) | **measured: 37.7 s → 19.6 s** and ~100 s of CPU taken off the browser tier's timing | **needs the owner's Gate-Changed** | no |
| 7 | Narrow revert-red's TypeScript scope to `vitest related <file> --run`, widening to the full suite only for a survivor, as Go does (`scripts/check_revert_red.py:351`) | ~30–40 s of check-pr when a branch touches 5 TS files; more on bigger branches | **needs the owner's Gate-Changed** | no; the verdict is the same |
| 8 | Skip `web-test` and `web-deps-check` in `make check` when `web/` is unchanged since the last pass, and `check-scripts` likewise for `scripts/` (a per-area `check_stamp.py`) | ~35–50 s of CPU per Go-only turn, and less contention for the Go steps | **needs the owner's Gate-Changed** | no |
| 9 | Run `lint_speculative_api.py`'s two platforms at once (`:155-157`) | ~15 s after a Go change | **needs the owner's Gate-Changed** | no |
| 10 | Fold the ladder test into `TestPlayback_AFilmIsMeasuredOnItsNodeAndPlaysLevelled` | one server start | no gate change | **needs Test-Weakened** |
| 11 | Run the System-page and invitation browser tests in chromium only | ~10–25 s of Firefox in the browser tier | no gate change (the Playwright config is not a gate) | **needs Test-Weakened**; the owner's call |
| 12 | Drop `internal/core/jobs` `TestRequeueRunning_ReturnsWhatACrashLeftRunning` | milliseconds | no gate change | **needs Test-Weakened** |
| 13 | Trial vitest's `fsModuleCache` on a clean tree | maybe 3–5 s a run, if it is sound | **needs the owner's Gate-Changed** (`web/vitest.config.ts`) | no |

Items 1–5 are no-gate-change work an agent can do. They should take check-pr from 278 s to
about 200 s. Items 6–9 are the owner's gate changes, and should take it to about 150–170 s while
also cutting `make check`. Items 10–12 trade a test for time; each saves little, and none should
go in without the owner's trailer.

## 4. For the dev loop, today, with no change

- **Go:** `go test -run '<Test>' ./<pkg>/` uses the test cache. Leave out `-count=1` unless a
  fresh run is the point. Until item 1 lands, anything under `cmd/orpheum` costs about 10 s
  more than it should.
- **Web:** `npx vitest related <changed files> --run`, run in `web/`, runs only the tests that
  import what changed. `make test-web-watch` keeps a watcher running.
- **Revert-red:** `make check-revert-red ONLY="<path>"` probes chosen files, and
  `make check-pr-failed` re-runs only the failed steps (`standards/makefile.md`).
- **Browser tier:** `make test-browser` re-encodes about 40 s of films first (item 3).
  `E2E_ARGS='--project chromium --grep "<title>"'` with `make browser-tier` narrows the run.

## 5. Side effects of this audit

- `scripts/fetch_mkvtoolnix.py` installed MKVToolNix 102.0 (55 MB) into `.tools/mkvtoolnix/`,
  because the integration run needed it and it was missing outside the dev image.
- Scratch files were removed. About 500 entries this audit added to
  `web/node_modules/.vitest-cache` were deleted again.
