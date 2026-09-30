> **ABOUTME:** How tests are written in every language here: what a change must carry, how a
> check proves it can fail, how absence and skips are handled, fakes, fixtures, goldens, the
> `TestArch_` convention, naming, and which make target runs what. Each rule names its enforcement.

# Testing Standard

[testing-strategy.md](../design/plans/testing-strategy.md) says **what** gets tested: the corpus
tiers, the product assertions, the device lab. This standard says **how** a test is written,
in Go, TypeScript and the Python gate scripts alike. The reasons are
[principles §17–§21](../principles/README.md#17--a-rule-that-nothing-runs-is-a-hope) and
[D190](../decisions/working-notes.md).

**Every rule names its enforcement.** *(planned)* marks a mechanism that does not exist yet; the
rule is review-only until it lands. **Review-only** means no mechanism is planned, and the
reviewer carries it.

## 1. A behaviour change carries a test that fails when it is reverted

**Every** behaviour change: count the changes in the commit, count the tests that would go red
if each were undone, and make the numbers match. Not "the package has tests", and not "the
headline fix has one" — the second change in a commit is the one that escapes.

- Write the test first and watch it fail against the unfixed code. A bug fix starts from a
  test that reproduces the bug.
- Red-on-revert is **necessary, not sufficient**. It shows a test is wired to the code, never
  that the code is reachable from `cmd/`. See §5 for the other half.
- **The exception is a trailer, not a silence.** A change that cannot go red in the suite —
  a fix only a television shows — carries `Verified-By: VQ<n>` in the commit message, naming an
  item in [verification-queue.md](../verification-queue.md) that exists and is ticked.
- **A refactor goes in a commit of its own.** When a file's part of a `fix` or `feat` commit
  already made is only a refactor, a later commit that touches that file says so:
  `Refactor-Only: <path> — <why>`. Like `Verified-By:`, it is listed for review on the author's
  word, not passed silently.

**Enforced by:** `scripts/check_revert_red.py`, run by `make check-pr` against a base revision
(the commit the owner last took, marked with `make mark-applied`). For each source file a
behaviour commit changes (Go, Python, web TypeScript and CSS, a migration step with its checksum
line and golden), it restores the old file in a throwaway worktree and requires a test to fail,
and it accepts a `Verified-By:` trailer only for a ticked item. Every other file a behaviour
commit changed is listed as UNCHECKED.

## 2. Every check is first seen to fail

Before a pass is trusted, the same check has been seen failing on a case where failure is
certain ([principles §18](../principles/README.md#18--a-check-you-have-never-seen-fail-proves-nothing)).

- **A test of a negative has a positive sibling.** If the pass condition is "this was refused",
  another assertion in the same test shows the same machinery succeeding where it should, and
  the refusal is attributed to our layer by error identity (`errors.Is`, an error code, an
  errno), never by "some error occurred". A shell parse error and a permission denial are both
  errors (yoloAI A22).
- **Empty is invalid.** A comparison whose inputs can be empty guards against emptiness first:
  two empty lists are equal, and "0 dropped of 0 frames" means nothing played.
- **Assert the content of an error, not only its presence.** `err != nil` tests that something
  went wrong, not that the thing under test did.
- **Every gate has tests that make it fail and tests that make it pass**, as
  `scripts/tests/test_check_docs.py` does, and a gate's matcher is tested on the text it must
  ignore (comments, code spans, string literals) as well as on the text it must catch. The
  architecture rules have their own self-test (`internal/archtest`).

**Enforced by:** revert-red (§1) for product tests; `make check-scripts` runs the gates' tests.
That each gate has a failing case, positive siblings and error identity are **review-only**.

## 3. A missing capability fails; only a named list may be absent

A test whose prerequisite **could exist on this host and does not, fails** — Landlock, Quick
Sync, `/dev/dri`, a pinned FFmpeg, a browser ([D190](../decisions/working-notes.md) §2).

- The one way to downgrade that is `ORPHEUM_TEST_UNCONTROLLED`, a comma-separated list set **per
  CI step or lab job**, never in a developer's shell profile. The name should feel wrong to set
  on a machine you control. Nothing auto-detects `CI`.
- The list is read in **one** helper, in `internal/testutil`, which every test calls to declare
  its prerequisite. It fails the test when the capability is absent and unlisted, and skips
  with the capability's name when it is listed. Every run prints the list it was given.
- A test that **cannot** run on a platform (tvOS on a Linux runner) is not scheduled there — by
  build tag, `GOOS` file suffix or job matrix — and the run lists it as **uncovered** with the
  reason. Uncovered is printed, never silent.
- Every `ORPHEUM_TEST_*` variable the code reads is **set** by something: the Makefile, a CI
  workflow or the lab runner. A variable that nothing sets gates a tier that has never run
  (yoloAI DF94).

**Enforced by:** `scripts/check_claims.py` (`make check-claims`) fails on an `ORPHEUM_TEST_*`
read that nothing sets; the skip ban of §4 keeps the helper the only door. The helper is
`testutil.Require`, tested by `TestRequire_AnAbsentCapabilityFailsUnlessTheJobListsIt`; the
Makefile's Go test targets print the list.

## 4. No skip outside the one helper

Tests do not skip themselves. The helper of §3 and the conformance suites' declared
"cannot support, because…" are the only places a skip is produced.

| Language | Banned outside the helper | By |
| --- | --- | --- |
| Go | `t.Skip`, `t.Skipf`, `t.SkipNow` | forbidigo's `[skip]` rule in `.golangci.yml`, excluded only for `internal/testutil` |
| TypeScript (Vitest, Playwright) | `.only`, `.skip`, `.skipIf`, `.runIf`, `.todo`, `.fixme` on `describe`, `it` and `test` | ESLint `no-restricted-properties` in `web/eslint.config.js` |
| Python | `unittest.skip*`, `self.skipTest` | **review-only** — the gate scripts have no prerequisites to be absent |

A known-broken combination is a row in the platform skip list
([testing-strategy.md](../design/plans/testing-strategy.md), the hls.js shape), not a commented-out
or skipped test.

## 5. Fakes, not mocks; and the fixture the real caller would build

- **Hand-written fakes** that implement the interface the code consumes. No mock generators,
  no expectation-recording mocks, no assertions about call order. A fake is small, readable,
  and survives a refactor that keeps behaviour.
- **Build the fixture the way the caller builds it.** If the code under test receives a
  dependency, check that its real caller has one to give (yoloAI A27). A fake may not claim a
  combination of capabilities that no real implementation has: before the first test of a
  capability-guarded branch, **name the real implementations that pass the guard** and confirm
  there is at least one (yoloAI A15).
- **One conformance suite per contract, run through the interface against every
  implementation** ([D190](../decisions/working-notes.md) §4) — the client's six methods, the
  encoders. It is the only test no fake can satisfy.

**Enforced by:** depguard in `.golangci.yml` denies the mock frameworks (`github.com/golang/mock`,
`go.uber.org/mock`, `github.com/vektra/mockery`) and testify; ESLint `no-restricted-syntax` in
`web/eslint.config.js` refuses `vi.mock` or `vi.doMock` of our own modules, so the SDK takes its
transport as a parameter instead. The fixture rule is **review-only**; the conformance suites
carry it structurally.

## 6. Table tests, and names that describe behaviour

- **Several cases of one behaviour are one table**: a slice of named cases and a loop calling
  `t.Run(tc.name, …)` in Go; `it.each` or `test.each` in TypeScript; `self.subTest` in Python.
  The case name says what differs: `"ger and deu disagree within one file"`, not `"case 3"`.
- **A test's name states the behaviour it pins**, readable as a sentence:
  `TestScan_SkipsAppleDoubleFiles`, `TestPrepare_ErrorCarriesStderrTail`,
  `it("refuses a credential in a query parameter")`. The Go shape is
  `Test<Unit>_<Behaviour>`. Never `TestScan2`, `TestHappyPath` or `TestFix`.
- Test helpers call `t.Helper()` so a failure points at the caller.

**Enforced by:** `thelper` in `.golangci.yml`. Names and tables are **review-only**; a name-shape
check is cheap to add if they drift.

## 7. Fixtures are generated, recorded, or named after a bug

[D109](../decisions/working-notes.md)'s tiers decide where a fixture lives:

| Tier | Form | Rule |
| --- | --- | --- |
| 0 | none | pure functions, table tests |
| 1 | recorded node reports and probe JSON, kilobytes | named for **what was recorded and from where**, and never hand-edited into a shape no node produced (§5) |
| 2 | clips **generated by the test run** | the generator is committed and is the specification; the clips are not committed |
| 3 | a few real files, in-tree | each is **named after the failure it reproduces** (`appledouble-sidecar.mkv`, `truncated-container.mkv`) and listed in its directory's README with the bug or decision it pins; a file never earns a place by covering a codec |
| 4 | external, opt-in | its absence is announced, per §3 |

No fixture contains commercial content ([D187](../decisions/working-notes.md)): media is
generated, public-domain or Creative Commons, and filename-parser cases use real naming
*patterns* with synthetic titles.

**Enforced by:** **review-only**. A check that every tier-3 file has a README row is cheap to
add once the directory exists.

## 8. Goldens and baselines are edited by the owner

A golden file (an expected schema, an expected playlist, a subtitle screenshot) and a frozen
baseline (a latency, a quality score) change **only as the subject of their own commit**, never
alongside the code that made them differ
([D110](../decisions/working-notes.md);
[principles §20](../principles/README.md#20--a-gate-changes-on-purpose-never-to-land-a-change)).

- A threshold compares against the frozen baseline, never the previous run, with a per-test
  tolerance written as a literal in the test.
- A golden is regenerated by an explicit `-update` flag that prints what changed, never by
  default and never in CI.
- Loosening a tolerance, deleting a test, adding a skip or reducing assertions is a
  **test-weakening** change, and says so in a `Test-Weakened:` trailer with the reason.

**Enforced by:** the pre-edit hook (`.claude/hooks/pre_edit.py`) refuses an agent's edit to any
path `scripts/gate_paths.py` lists, which includes `testdata/golden/` and `testdata/baselines/`;
`scripts/check_test_weakening.py`, run by `make check-pr`, lists deleted tests, new skips, fewer
assertions and changed goldens, and fails without the trailer. Until migrations are live (D228),
library.db's schema goldens follow each fold of the last step into the baseline and are not
listed; as gate paths they still need `Gate-Changed:`.

## 9. `TestArch_` is reserved for claims the docs make

A sentence in `architecture/`, `principles/`, a plan or the decision log that states a
checkable fact about the code — *the core never executes FFmpeg* — is backed by a test named
`TestArch_<Claim>` and cites it by that bare name in backticks. The prefix is used for nothing
else, so the gate is a search, not a judgement.

- The test fails when the claim stops holding, and was **seen failing** (§2): the M0
  acceptance list ([cut-a.md](../design/plans/cut-a.md) §8) turns CI red with a SQLite import
  outside `store`, an `exec.Command` in core and an import of `internal/node` from core.
- A transitive claim ("the core never reaches `os/exec`") needs the import graph, which
  `internal/archtest` walks; depguard sees only direct imports. Use both: depguard gives fast
  editor feedback, the test the proof.
- The test's comment names the decision it defends.

**Enforced by:** `scripts/check_claims.py` (`make check-claims`): every cited `TestArch_` name
exists, and every `TestArch_` test is cited.

## 10. What runs where

| Target | What it runs | Who runs it |
| --- | --- | --- |
| `make check` | every gate that needs only the pinned toolchain: docs, the gate scripts' tests, source headers, dependencies, claims, Go lint, vet and unit tests including `TestArch_`, the generated-code diff, and the web workspace's checks. The `check:` line in the Makefile is the list | the Stop hook, the pre-commit hook, CI on every push |
| `make check-pr` | the integration tier (`make test-integration`, the pinned FFmpeg and MKVToolNix and the reference scale), the browser tier, an uncached web lint and the gates that compare against a base revision: revert-red, test weakening, commit messages | CI on a branch; by hand before merging |
| `make test-all` | `make check`, then the race detector with the test cache off; the tagged integration tier joins it when it exists | before merging; CI |
| lab runner | the device tiers ([device-lab.md](../design/plans/device-lab.md)) | nightly and on demand |

- **A unit test needs no FFmpeg, no media, no network and no daemon.** `go test ./...` passes on a
  clean checkout with nothing installed but Go ([D109](../decisions/working-notes.md)).
- A test that needs more carries `//go:build integration` (or `lab`), and is still **compiled**
  and linted by `make check` (`go vet -tags integration`, `golangci-lint --build-tags
  integration`), so a deleted helper cannot hide in a file the default build skips.
  `make test-integration` runs the tier against the pinned FFmpeg, and CI runs it after
  `make check`.
- `make check` aims to finish in about **90 seconds**, because every agent turn that touched code
  waits on it. A unit test that takes more than about a second belongs in a tag or needs a
  smaller input.
- CI runs the same make targets as a developer; no gate is pull-request-only.

**Enforced by:** the Makefile ([makefile.md](makefile.md)); the time budget is **review-only**.

## 11. The fake `ffmpeg`

The node runs FFmpeg only through `internal/node/execx`, which takes the binary's path as a
parameter. Unit tests point it at a **fake `ffmpeg`/`ffprobe`**: a POSIX shell script kept as a
file under `internal/testutil/testdata/` (so shellcheck covers it), linked into `t.TempDir()` by
`testutil.InstallFakeFFmpeg` ([D190](../decisions/working-notes.md) §4). Linked, never copied:
writing an executable while a parallel test forks lets the child inherit the open file, and
running it then fails with "text file busy". The fake:

- **records the argument list** the node built, which the test asserts through
  `FakeFFmpeg.Calls` — `-force_idr 1` reaching NVENC, absolute input and output paths, the
  segment pattern — with Go's failure messages rather than a shell script's;
- **records the environment** it received, which the test asserts is the explicit, minimal one
  execx builds, not the test process's own;
- **writes a known line to stderr and exits with a chosen status**, so the test can assert that
  the error the node returns carries the tail of stderr;
- **can hold**: start a grandchild that keeps stdout open and wait, so a test can cancel a run
  and see the whole process group die.

Real FFmpeg appears only in the `integration` tier and in `assumptions/`
([D111](../decisions/working-notes.md)), at the pinned version, and its absence fails (§3).

**Enforced by:** forbidigo's `[exec]` rule bans `exec.Command` and `exec.CommandContext`
outside `execx`, and `TestArch_OnlyExecxImportsOsExec` checks the imports. execx's own tests
run against the fake (`internal/node/execx/execx_test.go`).

## 12. Tests touch no network and no real media

- **Filesystem:** everything a test writes is under `t.TempDir()` (Go), a `tempfile` directory
  (Python) or the runner's temporary directory (TypeScript). A media root is a fixture tree built
  there. No test reads or writes a real library, `$HOME`, `/var/lib/orpheum` or the repository's
  working tree.
- **Network:** servers are `httptest.Server` or its equivalents on loopback. No test resolves a
  name or opens a connection off the machine; TMDB and OpenSubtitles are fakes behind the
  provider interface.
- **Configuration is injected, not ambient.** A test hands the code its paths, clock and
  environment; it does not steer it with `t.Setenv` or by changing directory, which also rules
  out `t.Parallel`. Time comes from the fake clock, so no test sleeps to synchronise.
- **Setup output is logged, never discarded.** Long setup writes to the test's log (a
  writer over the test's own output — `t.Output()`, since Go 1.25), not to `io.Discard`, so a
  stall reads as a stall (yoloAI DF97).
- **The environment is hermetic**: every test target exports `TZ=UTC`, `LC_ALL=C.UTF-8`,
  `GIT_CONFIG_GLOBAL=/dev/null` and `GIT_CONFIG_NOSYSTEM=1`, so the local run is the stricter one.
- **A flaky test is fixed or deleted.** It is not retried in CI. Device runs are the exception,
  and every retry is recorded ([D190](../decisions/working-notes.md) §6).

**Enforced by:** forbidigo's `[clock]` rule bans `time.Now`, `time.Since` and `time.Until`
outside `internal/platform/clock`, and its `[ambient]` rule bans environment, working-directory
and stdio reads outside `cmd/orpheum`; `usetesting` flags `os.MkdirTemp`, `os.Setenv` and
`context.Background` in tests. The hermetic exports are the Makefile's `HERMETIC`, set on every
test target. The network rule, sleeps and `io.Discard` are **review-only**; running the unit tier
in an empty network namespace is the mechanical form if the network rule is ever broken.

## 13. Go specifics that touch tests

- `TestMain` calls `m.Run()` and **returns**; it never calls `os.Exit`, which skips deferred
  cleanup (yoloAI DF61). Prefer `t.Cleanup` to a `TestMain` at all.
- Tests use the standard `testing` package with plain comparisons, or `go-cmp` (once the owner
  allowlists it) for structural
  diffs. No assertion framework.
- A test file sits beside the code it tests, in the same package unless it deliberately tests
  the exported surface (`package foo_test`).

**Enforced by:** depguard denies testify; forbidigo's `[ambient]` rule bans `os.Exit` outside
`cmd/orpheum`, which covers `TestMain`. The rest is **review-only**.
