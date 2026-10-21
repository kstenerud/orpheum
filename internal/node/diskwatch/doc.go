// SPDX-License-Identifier: AGPL-3.0-or-later

// Package diskwatch keeps the node from piling up on a disk that does not answer (D233 §10): a
// network share whose host is gone, mounted to wait for it, blocks every open and read of it in
// the kernel for good, where no timeout or cancel ends the thread that made it (staging,
// 2026-10-21: 103 such threads, and a node that could not even exit). Each operation on a root goes
// through its gate: one that has not returned within its window is answered as stalled, and is
// left to end on its own, listed as stuck; while it is, every other operation on that root is
// answered as stalled at once, touching nothing. A dead share costs one stuck thread, not one per
// request, retry and scan. When the stuck operation returns, the root is open again.
//
// # Invariants
//
//   - An operation answers when it returns within its window, and as stalled, ErrStalled naming the
//     root and path it waited on, when it does not; what it returns later is given to its release.
//     Enforced by TestGate_AnOperationPastItsWindowIsStalledAndReleasedWhenItReturns.
//   - While an operation on a root is stuck, every operation on that root is stalled at once,
//     running nothing, and those on other roots run; the root opens when it returns.
//     Enforced by TestGate_ARootWithAStuckOperationRunsNothingMoreUntilItReturns.
//   - Imports nothing from internal/core. Enforced by the depguard rule node-boundary.
//
// # Read before changing
//
//   - D233 §10: a stall on a disk makes its root unavailable at the core.
//   - docs/contributors/idiosyncrasies.md: a hard-mounted network share.
//
// # Where to change
//
//   - gate.go: the gate, its stuck operations, and the stall it answers.
package diskwatch
