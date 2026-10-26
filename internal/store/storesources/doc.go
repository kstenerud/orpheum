// SPDX-License-Identifier: AGPL-3.0-or-later

// Package storesources holds the store's tests that read the whole module's Go sources: that every
// generated query has a caller, and that nothing but the store's statistics loader runs ANALYZE. They
// live apart from internal/store because Go's test cache keys a package's result on every file its
// tests open: in internal/store, any edit anywhere in the module ran all of the store's other
// tests again (audit 2026-10-25, G2). It has no code; it is imported by nothing.
//
// # Invariants
//
//   - Every query the generated code (internal/store/sqlcgen) holds has a caller in the module's
//     sources. Enforced by TestGeneratedQueries_EachHasACaller.
//   - Nothing in the program but internal/store/statistics.go runs ANALYZE or PRAGMA optimize, which
//     would replace the committed planner statistics (D303 §3, DL15). Enforced by
//     TestStatistics_NothingElseAnalyzes, which fails a walk that read no source.
//
// # Read before changing
//
//   - D303 §3, DL15: the committed statistics.
//   - docs/contributors/test-weight.md, "Caches bring their own risk": a test that reads the
//     module's sources is uncached on every edit, so it lives where it costs only itself.
//
// # Where to change
//
//   - callers_test.go: the generated queries' callers. analyze_test.go: who runs ANALYZE.
package storesources
