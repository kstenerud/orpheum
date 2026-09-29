> **ABOUTME:** How Go is written here: formatting, the lint gate and why each rule is in it,
> naming, packages and their `doc.go`, complexity, errors, context, concurrency, and the one
> sanctioned way to log, tell the time, read the environment, start a process, touch a file,
> reach the database, hold a secret and speak JSON. Each rule names its enforcement.

# Go Standard

The rules serve four aims ([D192](../decisions/working-notes.md) §3): **code a person can read**;
**low complexity, enforced by tooling**; **code understood without chasing definitions**; and
**a gate that catches an LLM's mistakes before a person has to**. Most of this repository is
written by agents, and yoloAI measured what that means: *"Execution catches my errors. Reading
does not."* So each rule sits at the leftmost level that can hold it — type system, then code
generator, then lint, then test, then hook, then prose — and this page mostly explains the
mechanism rather than asking to be remembered
([audit-2026-09-23.md](../design/research/audit-2026-09-23.md) §7).

**How to read "Enforced by".** A linter name means `.golangci.yml`
([the file](../../../.golangci.yml)), run by `make check`. A `[tag]` is a forbidigo rule whose
message starts with that tag; its path exceptions are listed under the same tag in the config.
`TestArch_…` is a test in `internal/archtest`. *(M1)* marks a mechanism that lands with that
milestone; until then the rule is review-only. **Review-only** means no mechanism is planned.

The reasons are in [principles](../principles/README.md); tests have their own standard,
[testing.md](testing.md); SQL has [sql.md](sql.md).

## 1. Toolchain and formatting

- **Go 1.26**, pinned by `go.mod` and checked by `make check-tools`. One module,
  `github.com/kstenerud/orpheum`.
- **gofumpt and gci** format every file; imports are grouped standard library, then third
  party, then this module. `make fmt` applies them. **Enforced by:** `golangci-lint fmt --diff`
  in `make check`.
- **Lines are at most 120 columns.** A line that long is two ideas. **Enforced by:** lll.
- **Every Go file starts with `// SPDX-License-Identifier: GPL-3.0-or-later`**; everything
  outside `web/` is GPL (D80). **Enforced by:** goheader, and `scripts/check_source.py`.

## 2. The lint gate

`.golangci.yml` enables every linter by name (`default: none`), so an upgrade never switches a
new one on silently. It is grouped by what each linter protects, and every architecture rule
carries its reason in its message, so the failure says what to do instead.

**Suppressions.** A finding is fixed in the code. Where the linter is wrong about this line, a
`//nolint:<linter> // <why this finding does not apply here>` names one linter and says why.
"Makes CI pass" is not a reason. **Enforced by:** nolintlint (`require-specific`,
`require-explanation`, and `allow-unused: false`, so a stale suppression fails too).

**Never suppressed, never raised.** cyclop, gocognit, funlen, nestif and depguard are not
silenced by `//nolint` at all: complexity is fixed by extracting named steps, and an
architecture exception is an owner's change to the config, where it is visible. **Enforced
by:** `TestArch_NoForbiddenSuppressions`, and the pre-edit hook that blocks agents from editing
lint configuration.

**The gate is not loosened to land a change** (principles §20). If a rule is wrong, stop and
say so; the owner changes it.

## 3. Naming

A name says what the thing **is**, in the domain's words, so a reader at a use site needs no
trip to the declaration (yoloAI's "name for the reader's distance").

- **Words, not abbreviations.** `libraryID`, `payloadHash`, `preparedRendition`, not `libID`,
  `ph`, `pr`. Accepted short forms, because every Go reader knows them: `ctx`, `err`, `id`,
  `ok`, `i` (an index), `t`/`b`/`tb`/`f` (testing), `w`/`r` (an HTTP handler's), `tc` (a table
  case). **Enforced by:** varnamelen — a name shorter than three letters is allowed only within
  six lines of its declaration, receivers and type parameters included.
- **Types are named for the domain concept** the design uses: `Title`, `Edition`, `Version`,
  `File`, `Grant`, `PreparedRendition`, `JobKind` — the words of
  [schema.md](../design/plans/schema.md) and [cut-a.md](../design/plans/cut-a.md). A type
  named for its mechanism (`Manager`, `Handler`, `Data`, `Info`) needs a reason. **Review-only.**
- **Booleans read as predicates:** `isRemote`, `hasSubtitles`, `canSetRate`, `needsReentry`.
  **Review-only.**
- **Units are in the type or the name.** A duration is a `time.Duration`, a moment a
  `time.Time`; anything else says its unit: `positionMs`, `sizeBytes`, `bitrateKbps`,
  `checkEveryHours`, matching the schema's `*_ms` columns. **Enforced by:** revive `time-naming`
  forbids a unit suffix on a `time.Duration` (the type already says it); the suffix on a plain
  integer is **review-only**.
- **A field name carries its own meaning.** If a field needs a comment to say what it is,
  rename it; keep comments for what a name cannot say (an invariant, a zero value's meaning, a
  side effect). **Review-only.**
- **Receivers** are a short word for the type (`fake`, `scanner`), the same in every method.
  **Enforced by:** revive `receiver-naming`, and varnamelen for length.
- **No shadowing** of a builtin or an import (`len`, `url`, `path`). **Enforced by:** predeclared,
  revive `redefines-builtin-id` and `import-shadowing`, govet `shadow`.
- **Errors:** a sentinel is `ErrNotFound`, a type is `NotFoundError`. **Enforced by:** errname.
- **Exported identifiers have doc comments** that start with the name and end with a full
  stop. **Enforced by:** revive `exported`, godot.
- **Spelling is British**, in comments and strings, as in the docs. **Enforced by:** misspell
  (`locale: UK`).

## 4. Packages and `doc.go`

- **The layout is [cut-a.md](../design/plans/cut-a.md) §3**, and each directory boundary is a
  trust or licence boundary the linters can see. A new package fits that tree.
- **A package is named for what it does**, one lowercase word, the same as its directory. No
  `util`, `common`, `helpers`, `misc`, `shared`, `types`. **Enforced by:**
  `TestArch_NoGenericPackageNames`, revive `var-naming`.
- **No `init()` and no package-level mutable state**; dependencies are constructed in
  `cmd/orpheum` and passed to constructors. **Enforced by:** gochecknoinits, gochecknoglobals.
- **Accept small interfaces, declared where they are used; return concrete types.** An
  interface appears when a second implementation or a test fake needs it. **Enforced by:**
  interfacebloat (at most six methods), inamedparam (parameters named in interface methods);
  the rest is **review-only**.

**Every package has a `doc.go`**, and it is the first thing to read before changing the
package. Its package comment has a purpose paragraph and exactly these headings, in order:

- **`# Invariants`** — what must stay true, each naming the lint rule or `TestArch_` test that
  enforces it. An invariant nothing checks says so ("review-only until M1's test").
- **`# Read before changing`** — three to seven decision IDs and plan sections, each with a
  few words saying what it decided. Archived decisions are never cited.
- **`# Where to change`** — which file holds what, and what a typical change touches.

**Enforced by:** `TestArch_EveryPackageHasDocGo` (the file and headings); `scripts/check_claims.py`
(every `TestArch_` name cited exists and every one is cited; every `D<n>` in code is a live
decision). The content is **review-only**. When code implements a plan section, that section
gains an `IMPLEMENTED — see <package>` banner.

**The architecture, as depguard rules** (direct imports; the `TestArch_` tests check the
transitive closure, which depguard cannot see):

| Rule | What it forbids | Transitive check |
| --- | --- | --- |
| `core-boundary` | `internal/core/**` importing `os/exec`, `internal/node/...`, image or media-parsing packages (D26, D33, D99, D126) | `TestArch_CoreNeverImportsNode`, `TestArch_CoreNeverExecs` |
| `node-boundary` | `internal/node/**` importing `internal/core/...` or `internal/store` (D33, D31) | `TestArch_NodeNeverImportsCore` |
| `nodeproto-is-types` | `internal/nodeproto` importing anything internal but `internal/platform` | `TestArch_NodeprotoHasNoLogic` |
| `store-is-below-core` | `internal/store` importing core or node | — |
| `platform-is-a-leaf` | `internal/platform/**` importing anything above it | — |
| `decide-sees-capabilities` | `internal/core/decide` importing `net/http` or `api` (D2, D103) | — |
| `sql-only-in-store` | `database/sql` or a SQLite driver outside `internal/store` (D6, D31) | `TestArch_OnlyStoreImportsDatabaseSQL` |
| `exec-only-in-execx` | `os/exec` outside `internal/node/execx` and archtest's tests | `TestArch_OnlyExecxImportsOsExec` |
| `one-way-to-do-it` | the alternatives below (principles §4) | — |

**One way to do each thing.** depguard refuses the alternatives, and each message names the
replacement:

| Instead of | Use |
| --- | --- |
| gorm, ent, bun, sqlx; squirrel, goqu | sqlc, and the one search filter builder in `internal/store` ([sql.md](sql.md)) |
| log, logrus, zap, zerolog | `log/slog` via `internal/platform/slogx` |
| testify, gomock, mockery | `testing` with plain comparisons; a hand-written fake |
| viper, envconfig | flags and environment read once in `cmd/orpheum`; runtime settings in the database |
| pkg/errors, cockroachdb/errors | `fmt.Errorf` with `%w`, `errors.Is`, `errors.As` |
| gorilla/mux, chi, gin, echo, fiber, httprouter; swaggo | huma v2 on `net/http` (D193 §6); OpenAPI from the handlers (D85) |
| fx, dig, wire | constructors, wired by hand in `cmd/orpheum` |
| cobra, urfave/cli | the standard `flag` package |
| goose, golang-migrate | our runner in `internal/store` (D181) |
| benbjohnson/clock, clockwork | `internal/platform/clock` |

## 5. Complexity and size

These numbers are the gate; they are not raised to fit a function, and not suppressed (§2).

| Limit | Value | Linter |
| --- | --- | --- |
| Cyclomatic complexity per function | 15 | cyclop |
| Cognitive complexity per function | 20 | gocognit |
| Function length | 70 lines, 45 statements | funlen |
| Nested `if` complexity | 5 | nestif |
| Control nesting depth | 4 | revive `max-control-nesting` |
| Parameters / results | 6 / 3 | revive `argument-limit`, `function-result-limit` |
| Duplicated code | 100 tokens | dupl |
| Naked returns | never | nakedret |
| File length | warn at 400 lines, fail at 800 | `scripts/check_source.py`; revive `file-length-limit` |

Tests are exempt from funlen and dupl only, because a table of cases is clearer long than
factored. **The fix for a complexity finding is a named step**: extract the part a reader would
describe in a phrase, and give it that phrase as its name.

## 6. Types that say what they hold

- **A closed set is a named type with constants**, never a bare string: `type JobState string`
  with `JobStateQueued`, …; the schema's `CHECK` vocabularies map one to one. **Every switch
  over one names every value; `default` does not count.** A new codec or job kind then breaks
  every switch that forgot it. **Enforced by:** exhaustive (`default-signifies-exhaustive:
  false`, switches and map literals).
- **A closed set of variants with data** (the node's report records, the Watch Together
  messages) is a sealed interface marked `//sumtype:decl`, and every type switch over it is
  exhaustive. **Enforced by:** gochecksumtype.
- **A string field holds text**, not a rendered date, a byte count, an error or an enum
  (yoloAI's "name the data, not its rendering"). **Review-only.**
- **Parse at the boundary, then trust the type** (yoloAI's "parse, don't validate"): a value
  that has passed validation has a type that says so. **Review-only.**

## 7. Errors

- **Wrap with what was being done**, lower case, no final punctuation:
  `fmt.Errorf("open library %d: %w", libraryID, err)`. **Enforced by:** revive `error-strings`,
  `errorf`; errorlint (`%w`, not `%v`, for an error).
- **Inspect with `errors.Is` and `errors.As`**, never `==` or a type assertion on an error.
  **Enforced by:** errorlint.
- **The `err` idiom both linters accept.** Inside a function that already has an `err` (a named
  result, or an earlier `x, err :=`), write `err = f()` then `if err != nil`, or give the new
  error its own name (`if healErr := f(); healErr != nil`). `if err := f()` fails govet's
  `shadow`; `if err = f(); err != nil` fails gocritic's `sloppyReassign`. **Enforced by:** both
  linters.
- **Every error is handled or returned.** No bare call that drops one, no `_ =` or `_, _ =` on
  one, and no `return nil` after checking `err != nil`. **Enforced by:** errcheck
  (`check-blank`, `check-type-assertions`), nilerr, nilnesserr, revive `unhandled-error`.
- **No `nil, nil`.** A lookup that finds nothing returns a sentinel (`ErrNotFound`). **Enforced
  by:** nilnil.
- **An error from outside the module is wrapped where it enters**: in `store` (the driver),
  `node/execx` (a process), `core/datadir` and the node's scan, probe and bytes (the
  filesystem), `core/api`, `core/identify` and `core/turn` (the network). Inside the module,
  wrapping is for adding context, not ceremony. **Enforced by:** wrapcheck, scoped to those
  packages by an exclusion.
- **A failed subprocess's error carries the tail of its stderr** and the exit status; `exit
  status 1` alone is not a diagnosis. **Enforced by:** the fake-`ffmpeg` fixture
  ([testing.md](testing.md) §11) *(M2)*.
- **No `panic`** outside `cmd/orpheum` and tests; return an error. **Enforced by:** `[panic]`.
- **A type assertion uses the two-value form.** **Enforced by:** forcetypeassert, errcheck.

## 8. Context

- **Every function that does I/O, waits, or may be cancelled takes `ctx context.Context` first.**
  **Enforced by:** revive `context-as-argument`; noctx (HTTP requests, `exec`, `net.Listen` and
  SQL calls without a context); contextcheck (a function that drops the context it was given).
- **A context is never stored in a struct.** **Enforced by:** containedctx.
- **Only `cmd/orpheum` creates a root context** (`signal.NotifyContext`); tests use
  `t.Context()`. **Enforced by:** `[context]`; usetesting in tests.
- Context values carry request-scoped data only (a request ID, the authenticated principal),
  under unexported key types. **Enforced by:** revive `context-keys-type`; the rest
  **review-only**.

## 9. Concurrency

- **Every goroutine has an owner that waits for it**, and it ends when its context ends. The
  owner is the function or type that started it; its doc comment says so. No fire-and-forget.
  Use `sync.WaitGroup.Go` and the context; `errgroup` is not a dependency yet.
  **Review-only**, with the race detector in `make test-all`.
- **A channel is closed by its sender, only.** A `select` that waits also waits on
  `ctx.Done()`. **Review-only.**
- **A mutex guards named fields, declared next to them**, and is never copied. **Enforced by:**
  govet `copylocks`; the placement is **review-only**.
- **Loop variables, atomics, WaitGroups** are used the modern way. **Enforced by:** copyloopvar,
  revive `atomic`, `waitgroup-by-value`, `datarace`.
- **SQLite writes go through the store's one writer** ([sql.md](sql.md) §Connections); no other
  code serialises database access.

## 10. Logging

- **The logger is a `*slog.Logger` built once by `slogx.New` in `cmd/orpheum`** and passed to
  constructors; a component adds `With("component", "scan")`. There is no global logger.
  **Enforced by:** depguard `one-way-to-do-it` (the `log` package and third-party loggers);
  `[ambient]` (`slog.Default`, `slog.SetDefault`); sloglint `no-global: all`.
- **Messages are constant; data goes in attributes with camelCase keys**; use `err` as the key
  for an error, not `error`; with a context in scope, use the `…Context` form so request IDs
  follow. **Enforced by:** sloglint (`static-msg`, `key-naming-case: camel`, `forbidden-keys`,
  `context: scope`, `no-mixed-args`).
- **Library code never prints.** **Enforced by:** `[print]` (`fmt.Print*`, `print`, `println`).
- **A secret logs as `[redacted]`** because `secrets.Value` implements `slog.LogValuer` (§16).

## 11. Time

- **Code that needs "now" takes a `clock.Clock`**; `clock.System` in production,
  `clocktest.NewFake` in tests. No test sleeps to make time pass. **Enforced by:** `[clock]`
  (`time.Now`, `time.Since`, `time.Until` outside `internal/platform/clock`).
- **Stored times are `int64` milliseconds since the epoch** (`Now().UnixMilli()`), named
  `…At` or `…Ms`, as the schema's are. A client's monotonic time is `…MonoMs` and never mixed
  with server time (D66, D165). **Review-only.**
- **Durations are `time.Duration`** in Go and converted at the boundary. **Enforced by:**
  durationcheck (no `Duration * Duration`), revive `time-naming`.

## 12. Environment, argv and output

- **Only `cmd/orpheum` reads the environment, `os.Args`, the standard streams or the working
  directory, and only it calls `os.Exit`.** It turns them into typed values and passes them
  down (yoloAI's "no ambient configuration"). Variables take the `ORPHEUM_` prefix (D195).
  **Enforced by:** `[ambient]`; revive `deep-exit`. Test variables are read only by
  `internal/testutil` ([testing.md](testing.md) §3).
- **Nothing configures the shipped binary but its config file, its flags and its database**
  ([D204](../decisions/working-notes.md)). A release build hands its subcommands an empty
  environment; only a dev build reads `ORPHEUM_DEV_*`. The standard library's own environment
  reads are closed: outbound HTTP goes through `internal/platform/outbound` (no proxy from
  `HTTP_PROXY`), `time.Local` is UTC, SQLite keeps temporary data in memory, and `os.TempDir`
  and `filepath.Abs` are not used outside `cmd/orpheum`. **Enforced by:**
  `TestEnvironmentLookup_IsEmptyInARelease`, `TestUseUTC_TimesAreUTCWhateverTZSays`,
  `TestNewClient_TakesNoProxyFromTheEnvironment`, `TestVerifyPragmas_RefusesAConnectionWithoutThem`;
  `TestArch_ReleaseBinaryReadsNoEnvironment`; forbidigo `[outbound]` and `[ambient]`. A bare
  `&http.Client{}` falls back to the default transport, which no lint sees: **review-only**.
- **Configuration has one way in, and nothing defaults in code.** An opinionated value is a
  setting in the registry, seeded as a row with its starting value, or a config-file key with one
  (D200–D202, [plans/settings.md](../design/plans/settings.md)); a file or flag holds only what
  must exist before the database opens. A new constant that is really an opinion is a finding.
  **Enforced by:** review; a missing setting row heals and says so
  (`TestRead_HealsARemovedSettingAndSaysSo`).

## 13. Processes

- **Every process starts in `internal/node/execx`**, which re-executes `orpheum` as the
  confinement shim, passes an explicit environment (never the inherited one) and hardened
  arguments (cut-a §7). The core never starts a process. **Enforced by:** depguard
  `exec-only-in-execx` and `core-boundary`; `[exec]` and `[execve]`;
  `TestArch_OnlyExecxImportsOsExec`, `TestArch_CoreNeverExecs`.
- The other exceptions start no process that ships: archtest's tests, which run `go list`, and
  `internal/testutil`, which runs the MKVToolNix the integration tier edits Matroska files with
  (the node never edits media). **Enforced by:** `TestArch_TestutilIsImportedOnlyByTests`.
- **`execx` is the only place a child's lifetime is decided**, and it decides it the same way
  every time ([yoloai-lessons.md](../design/research/yoloai-lessons.md)):
  - never `sh -c`: the argv is a slice;
  - the child's stderr tail is folded into the returned error, so a failure names its cause;
  - the child runs in its own process group, which is killed when the context ends, so an
    FFmpeg never outlives the job;
  - a job's child follows the **job's** context, not the HTTP request that queued it.

  **Enforced by:** the package boundary above; the fake-FFmpeg fixture tests the argv, the
  environment and the stderr-in-error ([testing.md](testing.md)).
- **Confinement is applied in the re-exec shim, on a locked OS thread, before `execve`**
  (cut-a §7). Landlock and seccomp are per-thread kernel state; applied on an arbitrary
  goroutine they bind whichever thread it happens to run on (yoloAI DF10). **Enforced by:** the
  escape test ([D112](../decisions/working-notes.md), D190), which runs the real shim.

## 14. Filesystem

- **The core touches files only in `internal/core/datadir`**: the state and cache directories.
  It never opens a media path; media bytes come from the node's socket (D193 §1). **Enforced
  by:** `[core-fs]` (`os.Open`, `ReadFile`, `WriteFile`, `Stat`, `MkdirAll`, `filepath.WalkDir`
  and their kin, anywhere in `internal/core` but `datadir` and tests); M2's test runs the core
  with the media root unreadable to it.
- **A media root is opened once, read-only, as `confine.MediaRoot`**, a wrapper over `os.Root`
  that exposes reads only. Everything the node reads goes through it, so `..` and symlinks
  cannot leave the root. **Enforced by:** `[media-root]` (`os.OpenRoot` and `os.OpenInRoot`
  outside `confine` and `datadir`).
- **Nothing removes, renames or truncates a file** except the trash writer (`internal/node/trash`,
  when deletion arrives) and `datadir`'s own directories (principles §2, D144, D145). **Enforced
  by:** `[delete]` (`os.Remove`, `RemoveAll`, `Rename`, `Truncate`, the same methods on
  `os.Root` and `os.File`, and their `syscall`/`unix` forms); a test hashes every fixture before
  and after a full scan *(M2)*. Prepared-rendition eviction will need its package added to the
  exception, by the owner.
- **A file we write is written atomically, by one helper** (temporary file, sync, rename, in
  `datadir`), and the one it replaces is kept where principles §2 says so. Each file has one
  owning package. A socket file is removed before it is created, and only by its owner.
  **Enforced by:** `[delete]` (only `datadir` may rename); `dupl`; otherwise **review-only**.

## 15. Database

Everything is in [sql.md](sql.md). In Go: **only `internal/store` imports `database/sql` or the
driver**, and everyone else calls its generated queries. **Enforced by:** depguard
`sql-only-in-store`, `TestArch_OnlyStoreImportsDatabaseSQL`. Rows are closed and their errors
checked; **enforced by:** sqlclosecheck, rowserrcheck. SQL is never assembled from strings;
**enforced by:** gosec G201/G202; a `SELECT *` in a Go string fails unqueryvet.

## 16. Secrets

D194 is the design. In Go:

- **A third-party secret is a `secrets.Value`** from the moment it is read. Its `String`,
  `GoString`, `Format`, `MarshalJSON`, `MarshalText` and `LogValue` emit `[redacted]`, so
  printing, logging or serialising one cannot leak it. **Enforced by:** the type *(M1)*, and the
  canary test that drives the API, logs, `doctor`, backups and the database file
  (`TestCanary_ASecretAppearsNowhereButAsCiphertext`), and bug reports once they exist.
- **`Value.Reveal()` is called only by the provider client that sends the secret to its owner's
  service** (TMDB under `identify`, TURN credential derivation in `turn`; the OpenSubtitles and
  DNS-01 clients join the list when they land). Pass the `Value`, never the string. **Enforced
  by:** `[reveal]`, whose path exceptions are the list.
- **Only `internal/core/secrets` queries the `secret` table.** **Enforced by:** `[sealed-secret]`.
- **A password is never stored or compared as text**; it is an argon2id hash (D194 §2).
  **Review-only** until M1's tests.

## 17. JSON and the wire

- **JSON keys are camelCase** — the TypeScript client's idiom and the web agent's — and every
  field of a marshalled struct has an explicit `json` tag. **Enforced by:** tagliatelle
  (`json: camel`), musttag. The one exception is reading another program's format (`go list`
  in archtest), declared in the config.
- **Wire types are their own structs**, never the store's rows: generated rows carry no JSON
  tags, so a column added by a migration cannot leak into an API response. **Enforced by:**
  `sqlc.yaml` (`emit_json_tags: false`) plus musttag, which refuses to marshal an untagged
  struct.
- **Errors leave in cut-a §5.2's one shape**, with `code` from its closed enumeration.
  **Enforced by:** exhaustive over the code type; the shape by the generated OpenAPI document
  and `oasdiff` *(with the API)*.
- **JSON errors are checked**, including `Marshal`'s. **Enforced by:** errchkjson.
- **A list in a response is never `nil`**, because `nil` marshals as `null` and the client's
  type says array. Construct `[]T{}` for an empty result; sqlc emits empty slices
  (`emit_empty_slices`). **Enforced by:** the handler tests; otherwise **review-only**.
- **Node messages are versioned** and readers are tolerant of unknown fields (D85, migrations.md).
  **Review-only.**

## 18. Generated code

- **Generated code is committed, starts with `// Code generated … DO NOT EDIT.`, and is never
  edited by hand**: sqlc into `internal/store/sqlcgen`; later the OpenAPI document and the
  TypeScript types. To change it, change its source and run `make generate`. **Enforced by:**
  `make check-generated` (regenerates and diffs); the pre-edit hook; golangci-lint excludes it
  (`generated: strict` and the `sqlcgen` path).

## 19. Testing

[testing.md](testing.md) is the standard; the Go-specific mechanisms are:

- The standard `testing` package, table tests with `t.Run(tc.name, …)`, names shaped
  `Test<Unit>_<Behaviour>`, hand-written fakes. **Enforced by:** depguard (no testify or mock
  frameworks), thelper, tparallel, usetesting (`t.TempDir`, `t.Context`, `t.Setenv`).
- **A fake that other packages' tests share lives in a `<package>test` package** beside the
  real one (`clock/clocktest`), not in the package itself, so production code never compiles
  it. **Enforced by:** `scripts/lint_speculative_api.py`, which fails on unexported code whose
  only callers are tests.
- **`TestArch_` is reserved for claims the docs make**; each is cited in a `doc.go` or a
  standard. **Enforced by:** `scripts/check_claims.py`.
- **No `t.Skip`** outside the `internal/testutil` helper that applies D190's presence policy.
  **Enforced by:** `[skip]`.
- **`TestMain` calls `m.Run()` and returns; it never calls `os.Exit`**, which skips deferred
  cleanup (yoloAI DF61). **Review-only** (revive `deep-exit` exempts `TestMain`, so a test for it
  belongs in archtest if this ever recurs).
- **Inject what a test varies; don't set the environment.** `t.Setenv` is for the one thing
  that genuinely reads it (`cmd/orpheum`, `internal/testutil`). **Review-only.**
- **The race detector runs in `make test-all`**, and code must compile for macOS too (developers
  may work on one), which `make check` verifies with `GOOS=darwin go vet`. **Enforced by:** the
  Makefile.

## 20. Dependencies

- **Every module in `go.mod`, direct or indirect, is listed in
  [deps/go.allow](../../../deps/go.allow) with a reason and the decision that chose it.** Adding
  one is an owner action. The standard library and `golang.org/x` come first. **Enforced by:**
  `scripts/check_deps.py`; the pre-edit hook on the allowlist.
- **No `replace` or `retract` directives.** **Enforced by:** gomoddirectives.
- **`go.mod` and `go.sum` are tidy.** **Enforced by:** `go mod tidy -diff` in `make check`.
- `github.com/google/go-cmp`, which [testing.md](testing.md) §13 allows for structural diffs,
  is used only once the owner has added it to the allowlist.

## 21. Calls to other hosts

Every request to another host — a metadata provider, an image server, later the update check
and the subtitle provider — goes through `internal/platform/outbound` (§12) and has a stated
policy for each way it can fail, **decided before it is built** (audit-2026-09-25 §2.2;
agent-failures A5). [D213](../decisions/working-notes.md) is the pattern.

| Answer | What it usually means | What the policy must say |
| --- | --- | --- |
| **404 / gone** | our stored reference is stale, or the thing was removed | whether our record is fetched again, how often, and when we give up; never "missing for good" on the first answer |
| **429 / `Retry-After`** | we asked too fast | wait the time asked, in both its forms, capped; the wait costs no attempt |
| **401 / 403** | our key is missing or refused | hold every job of the kind until something outside changes (`jobs.MarkBlocked`), and say so where an admin looks |
| **5xx, a timeout, a refused connection** | the host is having trouble | a back-off schedule with a cap and an end, and a pause for the host after several failures in a row, probed once per step |
| **a 200 that is not what was asked for** | a proxy, a captive portal, a changed API | refuse it as a malformed answer; never store it |

- **Every request says who we are** (`User-Agent: Orpheum/<version>`). **Enforced by:**
  `TestNewClient_TakesNoProxyFromTheEnvironment`.
- **A request built from someone else's text is checked before it is sent**: a provider's image
  path is its expected shape, so it cannot name another path, query or host. **Enforced by:** the
  fetcher's own tests (`TestImageFetcher_FetchesOnlyTMDBsImagePathsFromItsImageServer`); otherwise
  **review-only**.
- **Nothing refetches on a timer the provider has not asked for, and nothing fetches in bulk
  that one request could answer**; a periodic refresh is a per-provider setting, spread out
  (D213 §3). **Review-only.**
