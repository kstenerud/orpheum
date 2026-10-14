// SPDX-License-Identifier: AGPL-3.0-or-later

// Package atomics writes a file whole or not at all, for the public discovery packages, which cannot
// import the module's internal/: atomics.go is a twin of internal/platform/atomics/atomics.go, byte
// for byte (the owner, 2026-10-14: duplicate, and keep the copies in line; look-alike-watch-list.md).
// A change is made there and copied here.
//
// # Invariants
//
//   - atomics.go is internal/platform/atomics/atomics.go, byte for byte.
//     TestAtomics_IsInternalPlatformAtomicsByteForByte.
//
// # Read before changing
//
//   - internal/platform/atomics, whose doc.go says what the code does and what tests it.
//
// # Where to change
//
//   - internal/platform/atomics/atomics.go, then copied here whole.
package atomics
