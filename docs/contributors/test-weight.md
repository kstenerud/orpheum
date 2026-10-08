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
  reading it from disk, say) is not related to it, and only the widening to the whole suite
  catches that.
