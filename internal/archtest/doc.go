// SPDX-License-Identifier: GPL-3.0-or-later

// Package archtest holds the architecture tests: the design's structural promises, checked against
// the real package graph from go list. Each checker is a pure function and is unit-tested against
// a synthetic graph that violates it, so every test is seen to fail (D190). The package has no
// production code; its tests are the one place outside internal/node/execx allowed to run a
// process (go list).
//
// # Invariants
//
//   - TestArch_CoreNeverImportsNode: no internal/core package depends, even transitively, on
//     internal/node.
//   - TestArch_CoreNeverExecs: no internal/core package depends, even transitively, on os/exec.
//   - TestArch_NodeNeverImportsCore: no internal/node package depends, even transitively, on
//     internal/core.
//   - TestArch_OnlyStoreImportsDatabaseSQL: only internal/store imports database/sql or a SQLite
//     driver directly.
//   - TestArch_OnlyExecxImportsOsExec: only internal/node/execx imports os/exec directly, and only
//     this package's tests besides.
//   - TestArch_NodeprotoHasNoLogic: internal/nodeproto imports nothing from the module except
//     internal/platform, and declares no functions outside methods.
//   - TestArch_EveryPackageHasDocGo: every package under internal/ and cmd/ has a doc.go whose
//     comment has the headings Invariants, Read before changing and Where to change.
//   - TestArch_NoGenericPackageNames: no package is named util, common, helpers or the like.
//   - TestArch_NoForbiddenSuppressions: no //nolint names a complexity linter or depguard.
//   - TestArch_QueriesNameEveryColumn: no query in internal/store/queries selects or returns *.
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
//   - rules_selftest_test.go: each checker against a violating synthetic graph.
//   - Add a claim as a pure checker, a TestArch_ test, a self-test that makes it fail, and a
//     citation in the owning package's doc.go.
package archtest
