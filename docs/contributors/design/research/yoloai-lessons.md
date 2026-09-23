> **ABOUTME:** What the owner's other agent-built Go project, yoloAI, learned about writing Go
> with agents: how it structures the code, the problems it hit and the mechanism that closed
> each, which of its principles transfer here, and the Go rules to reconcile into our standard.

# Lessons from yoloAI

Read 2026-09-23 from a shallow clone at `/yoloai/cache/repos/yoloai` (one commit, so no git
history; the findings files and `agent-failures.md` stand in for it). Specimen IDs are yoloAI's
own: **A** entries from its `agent-failures.md`, **DF** from its findings files, **D** from its
decision log. They are *not* this project's D-numbers. Two earlier notes cover yoloAI's testing
machinery and its agent-facing structure (`/yoloai/cache/research/testing-yoloai.md`,
`audit2-llm.md`); this document does not repeat them except where a Go lesson depends on them.
Nothing in either repository was changed.

## 1. How yoloAI uses Go

**Shape.** One module. The root package is a public library surface (`yoloai.Client`,
`Sandbox`, `Workdir`); `cmd/yoloai` is thin; `internal/` holds the CLI, orchestration, config
and helpers; `runtime/` holds one subpackage per sandbox backend behind one interface, with a
shared conformance suite (`runtime/runtimetest`) that every backend runs through the interface
alone. Tests sit beside code; slow tiers carry `//go:build integration` or `e2e`.

**The recurring pattern is "one sanctioned wrapper per dangerous primitive, and a lint rule that
points at it".** Each wrapper exists because the raw call caused an incident:

| Primitive | The one wrapper | Enforced by |
| --- | --- | --- |
| `exec.Command` | `internal/sysexec` — explicit env required, stderr tail folded into the error | forbidigo, **test files included** (D79) |
| `os.Getenv`, `os.Getwd`, `os.UserHomeDir`, `os.Stdout` … | read once at the CLI edge into a `config.Layout`, threaded down | forbidigo, with the replacement named in each message (D144); `TestArch_LibraryNeverReadsAmbientEnv` |
| `os.WriteFile`, `OpenFile`, `MkdirAll` | `internal/fileutil` — atomic write with fsync, directory fsync | forbidigo (DF37, DF68) |
| `slog.Default`, `fmt.Print*` | a threaded `*slog.Logger`; a declared "discard" sink rather than a nil one | forbidigo plus an AST fence, `TestArch_LibraryTakesNoFeedbackWriter` (D145) |
| `t.Skip` on missing infrastructure | `internal/testutil/uncontrolled.go` — absence fails unless the step names it | one env read, one grep target (D112) |

**What worked.** Every such ban paid for itself the first time an agent reached for the raw
call, because the lint message told it what to use instead. Architecture claims as named tests
(`TestArch_…`) cited from the docs. The conformance suite, which is the only test a fake cannot
satisfy. `make check` run by a Stop hook, so no agent turn ends red. Hand-rolled fakes; no mock
generator. The linter set in its `.golangci.yml` (errcheck, staticcheck, gosec, errorlint,
sloglint, nilerr, forbidigo, depguard, exhaustive, cyclop 15, gocognit 20, nolintlint).

**What it could not retrofit.** `nolintlint` there allows bare `//nolint:errcheck` because
legacy code had hundreds; its standard counts 1,152 directives. Two boundary values
(sandbox name, workdir path) are still validated strings rather than parsed types (DF15). The
suite has no `t.Parallel` and no `-race` run (critique T7, parked). We start with no code, so
none of these costs apply.

## 2. The problems, and what closed each

### Go-specific

| Problem | Specimen | Closed by |
| --- | --- | --- |
| A subprocess inherited the developer's environment, read the real `$HOME`, and `make check` deleted the developer's real VMs | DF19 | Explicit env via `sysexec`; `exec.Command` banned everywhere including tests (D79) |
| A failed subprocess reported `exit status 1`; the cause went to a stream nobody read | DF144, DF145 | `sysexec.TailBuffer` and `EnrichExitError` keep the last lines and wrap with `%w`. *"This convention has been re-broken by every new backend path that did not"* copy it |
| Cancelling a request's context killed a long-lived child it had started | tart idiosyncrasy | Long-lived children are not tied to a request context |
| A detached child kept the controlling terminal | DF40 | `SysProcAttr{Setsid: true}`, one idiom everywhere |
| A network-namespace switch without `runtime.LockOSThread` left a pooled OS thread in the wrong namespace; about 20% of later forks inherited it | DF10 | `LockOSThread` plus restore before unlock, as a convention |
| `filepath.Abs` quietly calls `os.Getwd`, bypassing the ban on it | DF222 | `filepath.Abs` and `os.TempDir` added to forbidigo |
| `os.MkdirTemp("")` wrote into the shared `/tmp` | D94 | Every temp path from one layout accessor |
| A rename alone is not durable; a version stamp written before the work it certifies | DF37, DF68 | `AtomicWriteFile` (temp, fsync, rename, fsync dir); stamp last |
| Two writers of one file with two idioms | DF173 | Filed; the lesson is one writer per file |
| `os.Exit(m.Run())` in `TestMain` skipped deferred cleanup; 110 temp homes leaked | DF61, DF146 | `os.Exit(run(m))`, and a sweep of test-named leftovers |
| About 130 `t.Setenv("HOME")` calls coupled tests to process state and ruled out `t.Parallel` | D23, T8 | Inject the layout; `t.Setenv` only where a real `git` is spawned |
| Integration-tagged files were never linted; 86 issues accumulated behind default output caps | DF146 | A second lint pass with the tags, output caps off; `go vet -tags` in `make check` |
| golangci-lint honours build constraints, so `_darwin.go` files were never linted on Linux, and a banned call slipped through | DF78 | Lint and vet per target `GOOS` |
| `unused` counts a test as a caller, so functions whose only user was their own test lived forever | DF105, D125 | `unused` run with tests excluded |
| Struct fields written and never read | DF221 | AST gate `TestArch_EveryStateFieldHasAReader` |
| `if cfg, err := Load(); err == nil && …` swallowed a validation error | DF212 | Open; `nilerr`/errcheck do not see this shape |
| depguard fenced a façade but not the packages behind it | G2, D57 | Deny by prefix, not by package |
| A shellcheck version skew between CI and a developer's container let seven findings through a green local run | DF187 | Tools pinned, and CI installs the pinned version |

### Agent-specific

| Problem | Specimen | Closed by |
| --- | --- | --- |
| Six passing tests certified a function no backend could reach: the fake had a capability combination no real backend has | A15 | Rule 10 amended: name the implementations that pass a capability guard before the first test; the guarantee moved into a conformance case |
| A fix passed every test and did nothing, because the tests supplied a dependency the real caller lacks | A27 | *"Build the fixture the way the caller builds it"*; the tests now build the object as the CLI does |
| A security guard passed on a shell parse error, because `assert.Error` accepts any error | A22 | Paths passed as argv with no shell, and a positive control that must succeed |
| A lint fix narrowed a test and changed what it tested, on a platform the agent could not run | A19 | Review only; the suppression comment now says the test mirrors production |
| A test tier gated on an env var nothing set skipped itself for months; two security tests had never run | DF94, DF99 | A gate that fails on any `YOLOAI_TEST_*` read that nothing sets; the presence policy (D112) |
| Integration setup output passed to `io.Discard` made a 30 GB download look like a hang | DF97 | `TestRepoHygiene_IntegrationSetupOutput_IsNotDiscarded`; `testutil.LogWriter(t)` |
| A doc said, in bold, "no exceptions"; it was false, and owner and agent both built on it | DF207, DF208 | `TestArch_` citations checked both ways by `check_claim_citations.py` |
| A fix was sized from a diff truncated at 60 lines | A31 | Not gateable in the repo; the file-size limit makes the whole file one read |
| Rules written into the very file being edited were read and not applied | A9, A28, A30 | Making the wrong state unrepresentable (a load gate, a refusal to render a verdict with an empty control) |
| A second change in the same PR went untested; the rule's example became its scope | D129 | `check_revert_red.py`; the rule leads with its scope |
| A bespoke fix duplicated a mechanism a sibling path already had | DF150–DF158, A4 | GEN §18: grep for the defect's shape; converge rather than invent |

**The pattern across both tables.** yoloAI's own log concludes *"Execution catches my errors.
Reading does not"*, and that almost every specimen was caught by the owner or by a gate, never
by an agent re-reading. Its factual-accuracy principle *"fired zero times in six"*. Every fix
above that stuck is a mechanism: a lint rule, a wrapper, a test, a refusal. The fixes that were
conventions only (DF10, DF40, the stderr tail before a helper existed) were re-broken.

## 3. Which principles transfer

yoloAI's principles fill five documents. Most restate things this project already holds, or
answer a question we do not have.

**Transfer, as new principles** (added to `principles/README.md` §17–§21):

| Ours | From | Why it earns a place here |
| --- | --- | --- |
| §17 a rule nothing runs is a hope | GEN §16–17, AGENTS rule 13, the "fired zero times" measurement | Our own §13 was violated twice in three days by an author who had cited D59 (D155, D156). Our tree is large and prose-heavy, which is where this bites hardest |
| §18 a check never seen failing proves nothing | TEST §11, D112's presence policy, A15/A22/A27 | Already decided here as D190 §2–§3, but a decision is not reached at the moment of writing a test; the principle is |
| §19 a defect is a sample | GEN §18 | Our §4 covers a second *dependency*; it is silent on a second *internal* mechanism, which is the commoner agent failure |
| §20 the gate changes on purpose, never to land a change | DEV §6, §10; A19 | Our D110 says it for baselines only; agents loosen whatever is nearest |
| §21 an owner's correction is recorded | `agent-failures.md` | The only input from which yoloAI's later gates were built |

**Transfer to the Go standard, not to principles:** parse-don't-validate (DEV §4), no ambient
configuration (DEV §12), justify every discard (DEV §7), name for the reader's distance
(DEV §15), readers do not mutate and delete-before-create (its `go.md`). Each is a rule a linter
or a reviewer applies to a line of code; a principle would add prose where a lint message
already speaks.

**Already held here:** boring technology (our §1), one way to do it (§4), safe defaults (§15),
factual accuracy (§8).

**Do not transfer:**
- **Library-first and the public-surface contract** (ARCH §1–§4). yoloAI is a library with
  embedders; our public contract is the HTTP API, and D85's OpenAPI diff guards it.
- **Security principles on containers** (SEC §3, §6). Different threat model; ours is D26, D112
  and D194.
- **Default to public** (GEN §11). We build privately until the first beta (D192).
- **Cross-platform per platform** (GEN §10), beyond cross-compiling. The server is Linux-first.
- **The sudo-ownership machinery** behind its `fileutil` bans. We run as a service user.
- **A "docs last swept" date in `AGENTS.md`.** Cheap, but its value came from `architecture/`
  having no gate; ours will be gated from the first package. Worth revisiting if prose
  drift appears.

## 4. For the Go standard

Recommendations for the lead to reconcile into `standards/go.md`; none is written there by
this round.

1. **One exec package, `internal/node/execx`, is the only caller of `os/exec`,** tests
   included (D79, DF19). It builds the child's environment explicitly, folds stderr's tail into
   the error while wrapping with `%w` (DF144/145), starts the child in its own session and
   kills the process group on cancel (DF40), and never runs `sh -c` (A22). A prepare job's
   child is bound to the *job's* context, never to the HTTP request that queued it.
2. **Confinement is applied in the re-exec shim, before `exec`, on a locked OS thread.**
   Landlock and seccomp are per-thread kernel state; DF10 is what happens when Go's thread pool
   meets per-thread state.
3. **Ambient reads live only in `cmd/`:** `os.Getenv`/`LookupEnv`/`Environ`, `os.Getwd`,
   `filepath.Abs` (DF222), `os.TempDir` (D94), `os.UserHomeDir`, `os.Stdout`/`Stderr`,
   `fmt.Print*`, `slog.Default` (D145), `time.Now` and `time.Local` (through
   `platform/clock`). Every forbidigo message names the replacement.
4. **One atomic-write helper** (temp, fsync, rename, fsync directory) and one owner per file
   (DF37, DF68, DF173). Delete-before-create for named persistent things, starting with the
   node's Unix socket file.
5. **`TestMain` calls `m.Run()` and returns; it never calls `os.Exit`** (DF61). No
   `t.Setenv` for anything the code could be handed instead (T8); `usetesting` enforces the
   `t.TempDir`/`t.Context` half.
6. **Lint every build tag and every shipped `GOOS`.** A second golangci-lint pass with
   `--build-tags integration,lab`, output caps off (`max-issues-per-linter: 0`,
   `max-same-issues: 0`) (DF146); `go vet` per target `GOOS` for the Linux-only confinement
   files and their stubs (DF78).
7. **`unused` with tests excluded** to find API whose only caller is its test (D125).
8. **`nolintlint` with `require-explanation`, `require-specific` and `allow-unused: false`**
   from the first commit, and no suppression of `cyclop`/`gocognit` at all.
9. **Parse at the boundary into types.** Typed IDs, closed enums with
   `default-signifies-exhaustive: false`, `secrets.Value` (D194) as the model; an empty string
   is never a default.
10. **Slices in API responses are never `nil`** (its critique G4b): `nil` marshals as JSON
    `null`, which the generated TypeScript client then types as nullable.
11. **Names for the reader's distance.** Struct fields and parameters spelled out; a comment
    that says what a field is means the name is wrong (its `go.md`).
12. **`-race` in `make test-all`.** yoloAI never ran it (T7); Watch Together, the job queue
    and the node's report stream are concurrent from day one.
13. **No `init()` registration.** Wire implementations explicitly in `main`, so the wiring is
    visible to the reader and to `unused`.

## Not tried

- yoloAI's git history, which the shallow clone does not have; fix commits were not read.
- Running yoloAI's gates or its linter against its tree to confirm the counts it states.
- Checking that each recommended linter option exists under the golangci-lint version this
  project pins; they were taken from yoloAI's config and the audit, not re-verified.
- Any measurement of how often these failures occur; the specimens show that they occurred.
