# AGENTS.md

**Orpheum** (provisional name, D195) is a self-hosted video media server: a Go server with a
media node, and a TypeScript/React web client and cast receiver. Two people use it first; that
release is **Cut A**. This file is the contract for every agent changing the code. `CLAUDE.md`
imports it. Rules, not reasons: each rule names what enforces it, and the reasons are behind the
pointers.

## Start here

1. **[`docs/contributors/design/plans/cut-a.md`](docs/contributors/design/plans/cut-a.md) is the
   build contract.** It lists what to build, where it runs, the layout, every endpoint and
   message, and milestones M0–M5 with acceptance tests. Build the milestone you are given, in
   order.
2. **Read a package's `doc.go` (Go) or `README.md` (web) before changing it.** Its
   **Invariants** say what must stay true and which test or lint enforces each one. Its **Read
   before changing** names the few decisions that govern it. Read those; do not read the whole
   design tree.
3. **Standards:** [`docs/contributors/standards/`](docs/contributors/standards/README.md).
   Principles: [`docs/contributors/principles/`](docs/contributors/principles/README.md).
   Decisions: [`docs/contributors/decisions/index.md`](docs/contributors/decisions/index.md).

## The gate

**`make check` must pass before you finish.** The Stop hook runs it at the end of every turn
that changed a file and blocks the turn with the failures. The pre-commit hook and CI run it too.
A branch also passes `make check-pr` (the integration tier, revert-red, test weakening, commit
lint) before it merges.
`make help` lists everything.

**Fixing a failure, re-run only what failed; then run the whole gate once.** Loop on the narrowest
run that shows the failure: `go test -run <Test> ./<pkg>/`, `npm --prefix web test -- <file>`,
`make check-revert-red BASE=<base> ONLY="<path>..."`, or `make check-pr-failed`, which re-runs
only the check-pr steps that failed last time (revert-red on only its failed files). A targeted
green is not the gate's: when it passes, run the full `make check-pr` once before the branch is
done. *Standards: makefile.md §4.*

## Rules

1. **Cut A's design is frozen** (D193 §8). If the plan is silent, ambiguous or wrong, stop and
   ask the owner; do not invent design, and do not quietly build around it. *Review, and the
   owner.*
2. **The gates are the owner's.** Edits to lint and compiler configuration, `Makefile`,
   `scripts/`, `.claude/`, `.githooks/`, `.github/`, `deps/*.allow`, `internal/archtest/` and
   test baselines are refused. Never loosen a gate to make code pass; if one is wrong, tell the
   owner. A branch that changes a gate needs the owner's `Gate-Changed: <why>` trailer.
   *Pre-edit and pre-bash hooks; `make check-pr`.*
3. **A new dependency is the owner's decision.** Propose the name, the job it does and why the
   standard library or an allowed package cannot. *`deps/*.allow` + `make check-deps` + pre-bash
   hook.*
4. **Every behaviour change carries a test that fails when the change is reverted** (D190). A
   change only a device can confirm adds a `VQ<n>` item to
   [`verification-queue.md`](docs/contributors/verification-queue.md) and a
   `Verified-By: VQ<n>` trailer. *`make check-revert-red`.*
5. **Never weaken a test to get green**: no deleting tests, adding skips, dropping assertions,
   widening tolerances or re-recording goldens. If a test is genuinely wrong, say so to the owner
   first; the commit then carries `Test-Weakened: <why>`. *`make check-test-weakening`.*
6. **A missing capability fails; it does not skip** (D190). Only a job's named
   `ORPHEUM_TEST_UNCONTROLLED` list may be absent. *Standards: testing.md; `check-claims`.*
7. **Generated code is never edited by hand.** Edit the source (`internal/store/queries/*.sql`,
   the handlers) and run `make generate`. *Pre-edit hook; `make check-generated`.*
8. **Migrations are append-only.** Never edit one that is committed; add the next. Until the
   owner declares migrations live (D228), a schema change instead folds the last step into the
   baseline and adds the change as the one step after it. *Pre-edit hook;
   `TestSteps_AreABaselineAndOneStepUntilMigrationsAreLive`; standards: sql.md.*
9. **Cite decisions by ID (`D<n>`) where you write rationale; never invent one.** Archived
   decisions and `design/research/` are history, not specifications. *`make check-claims`,
   `make check-docs`.*
10. **A load-bearing claim about the code gets a `TestArch_<Claim>` test**, cited by name in the
    owning package's `doc.go` or README. *`make check-claims`.*
11. **One way to do each job.** Before writing a helper, search for the existing mechanism; if
    there are already two, stop and say so. *`dupl`, depguard deny-lists, review.*
12. **Secrets never print.** Use `secrets.Value`; only provider clients call `Reveal()` (D194).
    *forbidigo; the M1 canary test.*
13. **Commits:** subject `type(scope): summary` (`feat`, `fix`, `docs`, `test`, `refactor`,
    `build`, `ci`, `chore`, `perf`; `!` marks a break); `feat`/`fix`/`refactor`/`perf` carry a
    prose body saying what was wrong and why this fixes it; AI work ends with
    `Co-Authored-By: <exact model> <noreply@anthropic.com>`. *commit-msg hook;
    `make lint-commits`.*
14. **Multi-commit work goes on a `work/<topic>` branch**, merged to `main` when done and then
    deleted. A migration's branch is `release-vX.Y.Z` (standards: sql.md). In a yoloAI sandbox,
    commit on the checked-out branch instead: the owner takes the commits out with
    `yoloai apply`, and branches do not travel. *Review.*
15. **When the owner corrects a claim you made, record it** in
    [`docs/contributors/agent-failures.md`](docs/contributors/agent-failures.md) before moving
    on. It is the input from which the next gate is built. *Review.*
16. **Tests never touch the network, the real media library or the real clock.** Use generated
    fixtures, the fake FFmpeg and `clocktest`. *Standards: testing.md; forbidigo.*
17. **Audit your own work at every commit, and the whole codebase at every milestone.** Before
    each commit, work through the "At every commit" section of
    [`audit-checklist.md`](docs/contributors/audit-checklist.md) against the diff: above all,
    read each name it adds or changes as someone at the use site who has not seen the
    declaration. When a milestone is done, work through the whole checklist before starting the
    next. A prose rule is followed unevenly, and these checks catch what slipped past it before
    it spreads. *The checklist; no gate can check meaning (agent-failures A8).*

## Where things are

| Path | What |
| --- | --- |
| `cmd/orpheum/` | the binary: flags → mode |
| `internal/core/` | the server. Never runs FFmpeg, never opens media (D193 §1) |
| `internal/node/` | the media node: every file read and every FFmpeg run |
| `internal/nodeproto/` | wire types between core and node |
| `internal/store/` | the only package that touches SQLite |
| `internal/platform/` | the sanctioned clock and logger |
| `internal/archtest/` | architecture tests (`TestArch_…`) |
| `web/` | MPL-2.0: `packages/sdk`, `packages/player`, `apps/web`, `apps/cast-receiver` |
| `scripts/` | the gates, in Python, with tests in `scripts/tests/` |
| `deps/` | the dependency allowlists |
| `docs/contributors/` | design, decisions, standards, principles |
