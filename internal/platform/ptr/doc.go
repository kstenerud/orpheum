// SPDX-License-Identifier: GPL-3.0-or-later

// Package ptr reads an optional value, such as a nullable column sqlc gives as a pointer: the one
// helper for it, where four copies had grown in four packages (AGENTS.md rule 11).
//
// # Invariants
//
//   - A nil pointer reads as its type's zero value, any other as what it points to.
//     TestValueOrZero_ReadsNilAsZero.
//   - This package imports nothing from the module. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - AGENTS.md rule 11: one way to do each job.
//
// # Where to change
//
//   - ptr.go.
package ptr
