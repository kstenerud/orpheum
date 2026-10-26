// SPDX-License-Identifier: AGPL-3.0-or-later

// Package disctest writes disc images for tests: a UDF volume holding given files, as a DVD's
// (UDF 1.02, one partition) or a Blu-ray's (UDF 2.50, with a metadata partition), and the DVD's
// IFOs and the Blu-ray's playlists that describe their titles. Written from ECMA-167, OSTA's UDF
// 2.60 and how libudfread, libdvdread and libbluray read them; the images are generated, never
// taken from a real disc (D306 §1). Linux's UDF driver mounts them and reads each file as written
// (checked by hand, 2026-10-26). It is imported only by tests.
//
// # Invariants
//
//   - Only tests import it. Enforced by TestArch_TestSupportIsImportedOnlyByTests.
//   - Imports nothing from internal/core. Enforced by the depguard rule node-boundary.
//
// # Read before changing
//
//   - docs/contributors/standards/go.md, Testing: shared fakes live in a <package>test package.
//   - D350: what a disc image's main title is.
//
// # Where to change
//
//   - udf.go: the volume; dvd.go: a DVD's VIDEO_TS; bluray.go: a Blu-ray's BDMV; numbers.go: the
//     writer's narrowing of its numbers.
package disctest
