// SPDX-License-Identifier: AGPL-3.0-or-later

// Package archtest holds the architecture tests: the design's structural promises, checked against
// the real package graph from go list. Each checker is a pure function and is unit-tested against
// a synthetic graph that violates it, so every test is seen to fail (D190). The package has no
// production code; its tests run a process (go list), as internal/testutil does for the tests'
// tools, and neither ever ships.
//
// # Invariants
//
//   - TestArch_CoreNeverImportsNode: no internal/core package depends, even transitively, on
//     internal/node.
//   - TestArch_CoreNeverExecs: no internal/core package depends, even transitively, on os/exec,
//     except through the one exempt edge of D199 (modernc.org/libc, the SQLite driver's C runtime).
//   - TestArch_NodeNeverImportsCore: no internal/node package depends, even transitively, on
//     internal/core.
//   - TestArch_OnlyStoreImportsDatabaseSQL: only internal/store imports database/sql or a SQLite
//     driver directly.
//   - TestArch_OnlyExecxImportsOsExec: of the code that ships, only internal/node/execx imports
//     os/exec directly; internal/testutil, this package's tests and the signing tool's keygen.go (D394 §5) may too.
//   - TestArch_TestSupportIsImportedOnlyByTests: no package that ships imports internal/testutil
//     or a test-support package (storetest, clocktest, settingstest, authtest, jobstest…).
//   - TestArch_ReleaseBinaryReadsNoEnvironment: the release build of cmd/orpheum reads no
//     environment variable (D204).
//   - TestArch_HomeIsNeverACredential: every function of internal/core/api that asks whether a
//     request came from home has a signed-in person in hand: it takes an auth.Principal or calls a
//     require… method (research/security-week-2026-10-30.md §5, Plex2Shell's second step).
//   - TestArch_APIHandlersAreNamedForTheirOperations: each handler internal/core/api registers is
//     the method its OperationID names.
//   - TestArch_NoHandBuiltJSON: no code that ships spells out a JSON object's keys in a string; a
//     document is encoded from a struct, so a renamed field is renamed everywhere it is written
//     (agent failures A13).
//   - TestArch_IDsAreNeverGivenTwice: every table of persistent.db whose rowid is an id has it
//     AUTOINCREMENT, or borrowed from the row it extends, so an id the API gave out never names
//     another row (D243 §5).
//   - TestArch_PublicPackagesImportNothingInternal: no package under discovery/, the public building
//     blocks a closed service imports (D275 §1), depends on one under internal/, which Go would
//     refuse it.
//   - TestArch_NodeprotoHasNoLogic: internal/nodeproto imports nothing from the module except
//     internal/platform, and declares no functions outside methods.
//   - TestArch_EveryPackageHasDocGo: every package under internal/, cmd/ and discovery/ has a doc.go whose
//     comment has the headings Invariants, Read before changing and Where to change. Generated
//     packages (internal/store/sqlcgen, internal/store/cachegen) are exempt; their owner's doc.go
//     covers them.
//   - TestArch_NoGenericPackageNames: no package is named util, common, helpers or the like.
//   - TestArch_NoForbiddenSuppressions: no //nolint names a complexity linter or depguard.
//   - TestArch_QueriesNameEveryColumn: no query in internal/store/queries or
//     internal/store/cache/queries selects or returns *.
//   - TestArch_EachTableHasOneWritingPackage: each persistent.db and cache.db table is written, by
//     calls to the generated queries that insert into, update or delete from it, by one package:
//     the one schema.md's "Who writes each table" names. Calls are found by type-checking, so a
//     service method named like a query is not taken for one.
//
// # Read before changing
//
//   - D190 §3 and §4: architecture claims as named tests, each first seen to fail.
//   - D33: the core never touches media.
//   - D26: no FFmpeg in the core process.
//   - D6 and D31: one store.
//   - docs/contributors/design/research/audit-2026-09-23.md §7.
//   - docs/contributors/standards/go.md.
//
// # Where to change
//
//   - graph_test.go: loading the package graph.
//   - rules_test.go: the checkers and their TestArch_ tests.
//   - environment_test.go: the release binary's environment rule.
//   - json_test.go: the hand-built JSON rule; json_decoding_test.go: the strict decoding rule (D456).
//   - handlers_test.go: the API's handler-name rule.
//   - writers_test.go: the one-writer-per-table rule; writers_selftest_test.go: its parts against
//     input that breaks them.
//   - rules_selftest_test.go: each checker against a violating synthetic graph.
//   - Add a claim as a pure checker, a TestArch_ test, a self-test that makes it fail, and a
//     citation in the owning package's doc.go.
package archtest
