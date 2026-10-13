// SPDX-License-Identifier: AGPL-3.0-or-later

// Package safe32code writes and reads the codes a person types: safe32 (github.com/kstenerud/safe-
// encoding), in groups of four joined by dashes, "xxxx-xxxx", read in either case with the letters a
// person mistakes for digits taken as them (D218 §3, D223 §2). A core's device and setup codes and a
// discovery's claim code are written this one way, so the closed discovery service writes them the
// same (D275 §1).
//
// # Invariants
//
//   - A code is written most significant first, five bits a character, and what a person typed is
//     read back to it: either case, o as 0, l and i as 1, u as v, dashes and spaces ignored.
//     Enforced by TestDeviceCode_IsSafe32AndReadsWhatAPersonTypes and
//     TestWriteBytes_WritesTheBitsAsWriteDoes.
//   - Imports nothing from internal/. Enforced by TestArch_PublicPackagesImportNothingInternal.
//
// # Read before changing
//
//   - D218 §3: the device code; D223 §2: one way to write a code.
//
// # Where to change
//
//   - safe32code.go: writing and reading.
package safe32code
