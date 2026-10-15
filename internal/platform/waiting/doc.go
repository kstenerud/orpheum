// SPDX-License-Identifier: AGPL-3.0-or-later

// Package waiting marks a context as one a person waits on: a request, an action someone clicked.
// The store hands the one database writer to such a context's writes before background work's
// (D303, DL11); the api marks every request it serves.
//
// # Invariants
//
//   - A context carries the mark only when ForAPerson put it there, and every context derived from
//     it keeps it. Enforced by TestForAPerson_MarksTheContextAndWhatDerivesFromIt.
//
// # Read before changing
//
//   - D303: the data-layer principles, DL11 (people's writes first).
//
// # Where to change
//
//   - waiting.go: the mark and its test.
package waiting
