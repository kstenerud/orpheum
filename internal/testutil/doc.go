// SPDX-License-Identifier: GPL-3.0-or-later

// Package testutil is where a test declares what it needs from the machine. Require fails the
// test when a capability is absent, and skips it only when the job that runs it has named that
// capability in ORPHEUM_TEST_UNCONTROLLED; FFmpegDirectory and RequireMKVToolNix find the
// pinned tools that way. It installs the fake ffmpeg and ffprobe (testing.md §11), and runs the
// MKVToolNix that tests edit Matroska files with, which the node never runs. It generates media
// with the pinned FFmpeg: test images and the M2 acceptance's 200-file fixture tree.
//
// # Invariants
//
//   - A missing capability fails the test unless ORPHEUM_TEST_UNCONTROLLED names it
//     (D190 §2). TestRequire_AnAbsentCapabilityFailsUnlessTheJobListsIt.
//   - This is the only package that may skip a Go test: forbidigo's [skip] rule is excluded
//     here and nowhere else (.golangci.yml).
//   - ORPHEUM_TEST_UNCONTROLLED is read here and nowhere else in Go code, and the Makefile
//     prints it on every Go test run. check-claims fails a test variable nothing sets.
//   - Only tests import this package, so nothing here — skipping, test variables, starting
//     MKVToolNix — reaches a binary. Enforced by TestArch_TestutilIsImportedOnlyByTests; that
//     is why the depguard rule exec-only-in-execx and forbidigo's [exec] exempt it.
//
// # Read before changing
//
//   - D190 §2: a missing capability fails; it does not skip.
//   - docs/contributors/standards/testing.md §3 and §4.
//
// # Where to change
//
//   - require.go: the policy. A capability that needs finding (a pinned binary, a device)
//     gets its own helper here that finds it and calls Require, as ffmpeg.go does.
//   - mkvtoolnix.go: the pinned MKVToolNix (scripts/fetch_mkvtoolnix.py; linux/amd64 only).
//   - image.go and fixturetree.go: media the pinned FFmpeg makes for tests that may not run a
//     process themselves — an image, and cut-a's 200-file fixture tree (testing.md §7, tier 2).
//   - ffmpeg.go and testdata/fake-ffmpeg.sh: the pinned FFmpeg and the fake. The fake is linked,
//     never copied: writing an executable while a parallel test forks makes running it fail
//     with "text file busy".
package testutil
