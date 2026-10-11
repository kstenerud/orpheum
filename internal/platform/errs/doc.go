// SPDX-License-Identifier: AGPL-3.0-or-later

// Package errs says what was being done when an error happened, and passes success through
// untouched: the one helper for the deferred Close and the trailing rows.Err() both the store and
// the core's data directory had grown a copy of (AGENTS.md rule 11).
//
// # Invariants
//
//   - A nil error stays nil; any other is wrapped, so errors.Is still finds it.
//     TestWrapIf_WrapsOnlyAFailure.
//   - This package imports nothing from the module. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - AGENTS.md rule 11: one way to do each job.
//
// # Where to change
//
//   - errs.go.
package errs
