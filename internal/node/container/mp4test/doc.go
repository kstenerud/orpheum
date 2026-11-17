// SPDX-License-Identifier: AGPL-3.0-or-later

// Package mp4test builds small MP4 files for tests elsewhere in the node: tracks of many samples,
// their movie box after their media, whose index is read and cut down to a stretch (D512). Its
// samples are bytes, not media. It imports nothing of internal/node/container, so container's own
// tests can import it. It is imported only by tests.
//
// # Invariants
//
//   - Only tests import it. Enforced by TestArch_TestSupportIsImportedOnlyByTests.
//   - Imports nothing from internal/core. Enforced by the depguard rule node-boundary.
//
// # Read before changing
//
//   - docs/contributors/standards/go.md, Testing: shared fakes live in a <package>test package.
//   - D512: an MP4's index cut down to a stretch.
//
// # Where to change
//
//   - mp4.go.
package mp4test
