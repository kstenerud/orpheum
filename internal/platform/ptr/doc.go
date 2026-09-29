// SPDX-License-Identifier: GPL-3.0-or-later

// Package ptr reads and writes an optional value, such as a nullable column sqlc gives as a
// pointer: the one helper for each direction, where copies had grown in several packages
// (AGENTS.md rule 11).
//
// # Invariants
//
//   - A nil pointer reads as its type's zero value, any other as what it points to.
//     TestValueOrZero_ReadsNilAsZero.
//   - A zero value writes as nil, any other as a pointer to it. TestNilIfZero_WritesZeroAsNil.
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
