> **ABOUTME:** An independent review of the tests' architecture (2026-10-26): what makes them slow,
> load-sensitive, noisy or blind, ranked by cost from the recorded timings, with a fix and a safety
> note each. Read before changing a test harness, revert-red's probes or the web test helpers.

# Test architecture review

- **Date:** 2026-10-26. **Requested by** the owner: "We spend more time on tests than anything else
  in our dev cycles, so it's important that they are well formed."
- **Method:** an independent agent read the tests, the harnesses and `.dev/test-timings/` and
  `.git/revert-red/last-timings`; it ran nothing. The lead checked findings 2 and 5 against the code
  (both hold: `web/apps/web/src/test-dom.tsx:58-87`, `internal/store/storetest/storetest.go:141`).
- **Labels:** **[M]** measured from timings or code; **[I]** inferred. Go per-test times are elapsed
  with the package's tests in parallel, so they include contention.
- **Not repeated here:** [gate-throughput-2026-10-26.md](gate-throughput-2026-10-26.md) (the gates),
  [test-weight.md](../../test-weight.md) (known slowdowns).

## Findings, worst cost first

| # | Finding | Evidence | Cost | Fix | Safety |
| --- | --- | --- | --- | --- | --- |
| 1 | Revert-red's Go probes run whole packages **[M]** | `check_revert_red.py` go_test with no `-run`; `core/api` 310 tests (32–81 s), `cmd/orpheum` 162 (71 s), `core/identify` 117 (28–68 s); probes up to 78 s | ~15 min of check-pr; Go 1319 s, vitest 1120 s of 3044 s probing | a per-test coverage map from check-pr's full Go run (as the browser map); probes run `-run` the tests that reached the reverted function, the whole package when the map is stale or the function new | none lost with that fallback (`impact.py browser_scope` does the same) |
| 2 | A web test's unexpected request does not fail it **[M]** | `servedBy` (`test-dom.tsx:58-87`) throws inside fetch; the app catches it and `reportFailure` logs "Orpheum: a request failed"; the test passes. 65 files use `servedBy` | wrong code passes; the noise hides real failures | `servedBy` records unanswered requests and an `afterEach` (setupFiles) fails the test naming them; `unreachable` stays the declared failure | stronger; some tests will go red and need triage |
| 3 | Fixed real-time bounds fail under load **[M]** | `settleUntil` ≈2 s (`test-dom.tsx:243-254`, 102 call sites); `settleAnswers` a fixed 5 timer turns (`:260`); `testutil.LogLimit` 5 s (`internal/testutil/log.go:16`, 28 uses); test-weight §25's 5 s backup deadline unfixed | probable cause of probes failing with nothing reverted; each costs a rerun | raise failure bounds to `ReceiveLimit` scale (30–60 s); `settleAnswers` waits until `servedBy`'s in-flight count is zero and the DOM is quiet | only a hang reports later |
| 4 | Vitest's per-file cost, and `i18n` imported by 103 of 167 app test files **[M]** | 216 files, 126 s summed: 41 s import, 38 s tests, ~47 s environment; a probe of a locale or shell file runs nearly the suite (`en-settings.ts` 85 s, `status-tray.tsx` 81 s) | the largest probe kind, 1120 s | for `locales/`, run the catalogue test plus the tests rendering the changed keys (a key-usage map); measure `pool: 'threads'` / `isolate: false` for DOM-free `.test.ts` | isolation off can leak module state: only DOM-free, global-free files, with a nightly isolated run |
| 5 | `storetest.ExecRaw` opens a connection per call **[M]** | `storetest.go:141`, 156 call sites in 63 files; a 101-call loop test takes 12.9 s vs 4.4–5.6 s siblings; `QueryRaw` already shares one | seconds per heavy package | a per-test cached connection (as `rawReaderOf`), or `ExecRawBatch` in one transaction | tests that replace or delete the database file keep the uncached form |
| 6 | Fixtures insert raw SQL **[M]** | `INSERT INTO file` in 33 files (46×), `item` 38×, `title` 32×, `version` 30× | a schema rename edits dozens of tests (D344, D338) | builders in `librarytest` with defaults | neutral; keep `ExecRaw` for states the store never makes |
| 7 | `cmd/orpheum` tests start full servers at production cost **[M] times, [I] split** | 47 `serve()` calls each migrate from scratch, argon2id at production cost (64 MiB, t=3, p=4) in 26 sign-ins, the real clock's 2 s failed-sign-in answer; 6–9 s per test, package 71 s; `TestScenario_S25` 21–37 s on the same critical path | the package's wall time, worse under `-race` | seed from the migrated template, lower argon2 cost before `serve` (one test keeps the production parameters); move the `jobsWorld` scenarios to their own package | keep one cold-migration serve test |
| 8 | `archtest` loads the module ~22 times **[M] counts, [I] saving** | `loadModulePackages`/`loadExportFiles` per test; tests of 14 s, package 8–21 s | seconds every gate | load once per process (`sync.Once`) | none |
| 9 | The server is wired by hand in three places **[M]** | `serve`, `api` `newHarness` (`api_test.go:144-230`, ~15 services), `cmd/orpheum` `jobsWorld`; `NewExchanges` in 8 test files | a constructor change edits all; harnesses drift from production | one wiring function used by `serve` and the harnesses, fakes as parameters | stronger: tests run production's wiring |
| 10 | Untested code **[M]** | `internal/core/subtitles` (1204 lines) has no test file (tested through the 310-test api package); `jobqueuetest` 457, `certificatetest` 370, `identifytest` 248, `cloudflaretest` 177 lines of untested test support | slow probes; generator bugs (today's statistics generator) | package-level tests for subtitles with fakes; tests for the generators (`disctest` is checked by the real FFprobe) | stronger |
| 11 | Short sleeps to prove a non-event **[I]** | `fdproxy/channel_test.go:436,466` (50 ms), `execx/film_linux_test.go:196` (500 ms) | under load broken code may not run in time, so the test passes wrongly | wait for a positive signal (writer blocked, run paused), then assert nothing happened | stronger |
| 12 | Tests wait on log sentences **[M]** | `newCapturedLog`/`startServing` wait for "ready" and the `couldNotStart` sentence; 70 `Contains` on log text, 23 in `together/logs_test.go` | rewording a message breaks tests | wait on a structured key (an `event` attribute constant); logs stay asserted, by key | neutral |
| — | Each Go package builds its own migrated template **[I]** | 0.5–2.5 s each under load, ~40 packages | small | one template per `go test` run, keyed by `migrations/SHA256SUMS`, behind a file lock | neutral |

## Already good, keep

`storetest.OpenMigrated`'s per-process template with the invariants checked at the end; `QueryRaw`'s
shared connection; `tparallel` enforced with nearly every Go test parallel; `clocktest` throughout
(18 `time.Sleep` sites in all); `testutil.Receive`'s one-minute bound naming what it waited for;
`startServing` failing fast with the log; `settleUntil` naming its condition; timings kept by every
suite; `impact.py` narrowing the browser tier but erring wide; a small (~15 s) integration tier that
checks the disc generator against the real FFprobe.

## Also found the same day (lead)

- Revert-red does not keep the output of a probe that fails with nothing reverted, so the failing test
  had to be found by reproducing the load (2026-10-26).
- A test can pass on another test's leftovers: `status-tray.test.tsx`'s test of an earlier commit passed with the
  change reverted because an earlier test in the same file had already resolved the module-level
  `React.lazy` of the Watch Together controls; the covering test (054594a8) lives in a file of its own.
  Module-level singletons (lazy components, caches) are state shared across a file's tests: reset
  them per test, or keep the tests that depend on a cold start in their own file.
- Load reproduction: Go unit, Go integration and web unit suites, three at once each, all passed; the
  clean-probe failures came at ~10× oversubscription (load 82 on 8 vCPUs), fixed by an earlier commit.
