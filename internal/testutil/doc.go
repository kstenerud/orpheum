// SPDX-License-Identifier: AGPL-3.0-or-later

// Package testutil is where a test declares what it needs from the machine. Require fails the
// test when a capability is absent, and skips it only when the job that runs it has named that
// capability in ORPHEUM_TEST_UNCONTROLLED; FFmpegDirectory and RequireMKVToolNix find the
// pinned tools that way. It installs the fake ffmpeg and ffprobe (testing.md §11), and runs the
// MKVToolNix that tests edit Matroska files with, which the node never runs. It generates media
// with the pinned FFmpeg: test images, films and the M2 acceptance's 200-file fixture tree, each made
// once and kept in the generated-media cache (media.go), so a later run gets it without FFmpeg. Receive is a
// test's bounded wait on a channel, and Log a logger's output a test waits on for a line.
// A test finds a log line by its event (slogx.EventKey) with LineWith, LinesWith and
// Log.AwaitEvent, not by its message's English, so rewording a message breaks no test.
// SocketDirectory is where a test's Unix sockets go.
//
// # Invariants
//
//   - A missing capability fails the test unless ORPHEUM_TEST_UNCONTROLLED names it
//     (D190 §2). TestRequire_AnAbsentCapabilityFailsUnlessTheJobListsIt.
//   - This is the only package that may skip a Go test: forbidigo's [skip] rule is excluded
//     here and nowhere else (.golangci.yml).
//   - ORPHEUM_TEST_UNCONTROLLED is read here and nowhere else in Go code, and the Makefile
//     prints it on every Go test run. check-claims fails a test variable nothing sets.
//   - A test waits on a channel only through Receive, a select's case, or <-ctx.Done(), so no wait
//     can hang a run. TestArch_EveryTestWaitOnAChannelIsBounded.
//   - Generated media are kept under a key of everything that decides them: the FFmpeg build (its
//     -version line, its executable's size and modification time), the output's form and every
//     argument, a file argument by its contents. FFmpeg runs in an empty environment from "/", so
//     nothing else reaches it. TestMediaKey_ChangesWithEveryArgumentAndTheOutput,
//     TestMediaKey_KeysAFileArgumentByItsContents, TestMediaKey_ChangesWithTheFFmpegBuild.
//   - A kept file is never changed by a test: it is read-only, and callers get a copy or its bytes.
//     It appears whole or not at all, renamed into place, and a failed run keeps nothing; the cache
//     trims the files used least recently past its cap. TestGenerateMedia_IsMadeOnceAndEachCallGetsACopy,
//     TestGenerateMedia_KeepsNothingOfAFailedRun, TestGenerateMedia_ParallelMakingsOfOneFileEachGetItWhole,
//     TestMediaCache_TrimsTheLeastRecentlyUsedPastItsCap.
//   - Every generator here makes its media through the cache, in the user's cache directory:
//     TestGenerateMedia_KeepsThePinnedFFmpegsMediaInTheUsersCacheDirectory,
//     TestGenerateImage_IsMadeOnceOnItsStdoutAndKept, TestBuildFixtureTree_ClipsAreKeptAsGeneratedMedia.
//   - A test's Unix sockets go in SocketDirectory's directory, directly in the temporary directory
//     and removed when the test ends, never under t.TempDir, whose path leaves a socket a few bytes
//     or none of its 108. TestSocketDirectory_IsShortWhateverTheTestIsCalledAndGoneWhenItEnds.
//   - Only tests import this package, so nothing here — skipping, test variables, starting
//     MKVToolNix — reaches a binary. Enforced by TestArch_TestSupportIsImportedOnlyByTests; that
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
//   - media.go, image.go and fixturetree.go: media the pinned FFmpeg makes for tests — GenerateMedia's
//     file from any arguments, an image, and cut-a's 200-file fixture tree (testing.md §7, tier 2) —
//     and the generated-media cache they are kept in, in the user's cache directory
//     (orpheum/test-media; $XDG_CACHE_HOME moves it on Linux). A test whose FFmpeg run is what it
//     tests, or whose input differs every run (mkvmerge's output), runs FFmpeg itself instead.
//   - ffmpeg.go and testdata/fake-ffmpeg.sh: the pinned FFmpeg and the fake. The fake is linked,
//     never copied: writing an executable while a parallel test forks makes running it fail
//     with "text file busy".
//     EBUR128Summary is the loudness log the fake writes where a test needs FFmpeg to have
//     measured something.
//   - self.go: RunSelf, which runs the test binary again as a process of its own.
//   - receive.go: Receive, the one way a test waits on a channel: bounded by ReceiveLimit, and
//     failing with what it waited for and every goroutine's stack, so a wait cannot hang a run.
//   - socket.go: SocketDirectory, the one place a test listens on a Unix socket in.
package testutil
