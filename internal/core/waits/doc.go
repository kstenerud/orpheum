// SPDX-License-Identifier: AGPL-3.0-or-later

// Package waits says how long the work a person waits on will take (D324 §6): a file's
// read-through, for its loudness and its picture's timeline. It reads the work from
// cache.db, the files' sizes and lengths from persistent.db, and how long running work has run from
// the node exchanges; it measures each node's read speed from the read-throughs that finish there,
// in memory, starting from the node's slowest read.
//
// # Invariants
//
//   - A read-through running says what its bytes not yet read take at its node's read speed; one
//     waiting adds the bytes ahead of it on its node, over the node's read-throughs at once: running
//     work it would not bump, and waiting work above it, or level and older. Enforced by
//     TestEstimator_AReadThroughWaitsForTheBytesAheadOfItOnItsNode and
//     TestEstimator_AReadThroughsNodeSpeedIsWhatItsLastReadsMeasured. One running that says how far
//     into its film it is goes at its own pace instead.
//     TestEstimator_ARunningReadThatSaysHowFarItIsGoesAtItsOwnPace.
//   - Imports nothing from internal/node, never os/exec. Enforced by the depguard rule
//     core-boundary, and transitively by TestArch_CoreNeverImportsNode and TestArch_CoreNeverExecs.
//
// # Read before changing
//
//   - D324 §6; docs/contributors/design/plans/interest-priority.md §6.
//   - D255 §2: which running work a person's bumps, which is not ahead of theirs.
//
// # Where to change
//
//   - waits.go: the estimator, its read speeds, and each kind of work's estimate. The job rows it
//     reads are internal/store's OpenReadThroughs and OpenJobsOfFile; the API's words are
//     internal/core/api's getFileWaits, and the viewer's, web/apps/web/src/file-waits.ts.
package waits
