// SPDX-License-Identifier: GPL-3.0-or-later

// Package testutil is where a test declares what it needs from the machine. Require fails the
// test when a capability is absent, and skips it only when the job that runs it has named that
// capability in ORPHEUM_TEST_UNCONTROLLED. It is imported only by tests.
//
// # Invariants
//
//   - A missing capability fails the test unless ORPHEUM_TEST_UNCONTROLLED names it
//     (D190 §2). TestRequire_AnAbsentCapabilityFailsUnlessTheJobListsIt.
//   - This is the only package that may skip a Go test: forbidigo's [skip] rule is excluded
//     here and nowhere else (.golangci.yml).
//   - ORPHEUM_TEST_UNCONTROLLED is read here and nowhere else in Go code, and the Makefile
//     prints it on every Go test run. check-claims fails a test variable nothing sets.
//
// # Read before changing
//
//   - D190 §2: a missing capability fails; it does not skip.
//   - docs/contributors/standards/testing.md §3 and §4.
//
// # Where to change
//
//   - require.go: the policy. A capability that needs finding (a pinned binary, a device)
//     gets its own helper here that finds it and calls Require.
package testutil
