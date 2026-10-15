// SPDX-License-Identifier: AGPL-3.0-or-later

// Package querytally counts the database statements a unit of work runs, by query name, and its
// write transactions: the api counts each request's, logs them with a slow request, and tests
// check a list's count does not grow with the library (D303, DL6, DL17). The store counts into
// whatever Tally a statement's context carries.
//
// # Invariants
//
//   - A Tally counts each statement under the name its sqlc header gives, and reads back the totals
//     and a summary, the most run first. Enforced by TestTally_CountsByQueryNameAndSummarises.
//
// # Read before changing
//
//   - D303: the data-layer principles, DL6 (nothing a request does grows with the library) and
//     DL17 (slow work names itself).
//
// # Where to change
//
//   - querytally.go: the Tally and how a context carries it.
package querytally
