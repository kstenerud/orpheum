> **ABOUTME:** The code-debt backlog (principle §26): improvements seen and not yet made, each with the
> tax it levies and what would fix it, for the owner to schedule as code-fixing batches.

# Code debt

An improvement spotted while doing something else lands here unless it is small and in code already
being changed ([§26](principles/26-poor-code-is-a-recurring-tax.md)). Each entry says what it costs,
who pays, and what would fix it. A fixed entry leaves the list, its commit named in the commit that
removes it. Test-run slowdowns have their own record, [test-weight.md](test-weight.md); process
holes, [process-review-2026-10-26.md](design/research/process-review-2026-10-26.md).

| Debt | The tax | The fix | Found |
| --- | --- | --- | --- |
| `internal/store`'s own tests insert raw SQL (8 files), since they cannot import `librarytest`'s builders | a schema change still edits them by hand | a store-internal builder set beside `storetest` | 2026-10-26, fixture builders |
| ~47 log assertions match English sentences outside `together` and the start/ready lines | rewording a message breaks tests | match by the `logEvent` key, as an earlier commit did for the rest | 2026-10-26 |
| `internal/core/subtitles`' fetch-on-request flow (`fetch.go`, `record.go`, `textwait.go`, `fetchonce.go`, `forget.go`) has no package tests | its changes are probed through the 311-test API package | package tests with a fake Subtitler | 2026-10-26 |
| `NewExchanges` is built by hand in 8 test files | a constructor change edits all of them | take it from `assembly.Build` or one helper | 2026-10-26, D351 |
| Revert-red cannot narrow a wiring change (`NewServer`, `assembly.Build`): every API test runs | each such probe costs the whole package | a recorded per-test coverage run, nightly or on demand (owner's call) | 2026-10-26 |
| The locale probe trusts only literal keys; `en-settings.ts`' keys are built at run time | its probes run the whole related set (~34 s) | trust a template literal's fixed prefix | 2026-10-26 |
| `scripts/check_revert_red.py` is at 796 of its 800 lines | the next change must split it first | move a part (the outcome reporting) to its own module | 2026-10-27 |
| `internal/core/api/errors.go` is 427 lines (the 400 warning) | harder to read and change | split by error family | 2026-10-27 |
| `cmd/orpheum/session_uses_test.go:77` sleeps 5 ms to separate two real-clock stamps | real time in a test (testing.md) | a clocktest clock through the session's stamps | 2026-10-26 |
