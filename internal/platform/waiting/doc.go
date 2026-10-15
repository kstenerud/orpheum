// SPDX-License-Identifier: AGPL-3.0-or-later

// Package waiting marks a context as one a person waits on: a request, an action someone clicked;
// and as one that only reads, a GET or a HEAD. The store hands the one database writer to a
// person's writes before background work's (D303, DL11), and reports a write a reader makes
// (DL10); the api marks every request it serves.
//
// # Invariants
//
//   - A context carries the mark only when ForAPerson put it there, and every context derived from
//     it keeps it; the same for ForAReader. Enforced by
//     TestForAPerson_MarksTheContextAndWhatDerivesFromIt and
//     TestForAReader_MarksTheContextAndWhatDerivesFromIt.
//
// # Read before changing
//
//   - D303: the data-layer principles, DL10 (reading never writes) and DL11 (people's writes first).
//
// # Where to change
//
//   - waiting.go: the mark and its test.
package waiting
