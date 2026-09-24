// SPDX-License-Identifier: GPL-3.0-or-later

// Package execx is the only place a shipping process starts. It runs the bundled ffmpeg and ffprobe, and
// nothing else, with hardened arguments, an empty environment, the root directory as its
// working directory and stdin closed, in a process group that is killed whole when the run is
// cancelled. A failure carries the exit status and the tail of the tool's stderr. From M3 every
// run re-executes orpheum as a shim that applies confinement (internal/node/confine) first
// (cut-a §7); until then runs are unconfined, and the node says so.
//
// # Invariants
//
//   - It is the only shipping package that imports os/exec or calls exec.Command; the test-only
//     internal/testutil is the other. Enforced by the depguard rule exec-only-in-execx, the
//     forbidigo rule tagged [exec], and TestArch_OnlyExecxImportsOsExec.
//   - Imports nothing from internal/core. Enforced by the depguard rule node-boundary and
//     TestArch_NodeNeverImportsCore.
//   - Errors from the process are wrapped where they enter. Enforced by wrapcheck, which is scoped
//     to this package.
//   - A tool gets the hardening flags first and none of the node's environment (D204).
//     TestRun_PassesHardenedArgumentsAndAnEmptyEnvironment.
//   - A failure says why: its status and the last stderrTailBytes of stderr.
//     TestRun_AFailureCarriesTheStatusAndTheTailOfStderr.
//   - Cancelling a run kills everything it started, so no FFmpeg outlives its job.
//     TestRun_CancellingKillsTheWholeProcessGroup.
//   - A run that writes more than its caller allows is refused, never truncated.
//     TestRun_RefusesMoreOutputThanAllowed.
//
// # Read before changing
//
//   - D26: no FFmpeg in the core; confine it.
//   - D43: confinement is per invocation.
//   - D112: the node probes its confinement and reports it.
//   - D190 §4: the fake ffmpeg fixture asserts the arguments and environment.
//   - docs/contributors/design/plans/cut-a.md §7.
//
// # Where to change
//
//   - The shim entry point and the argument hardening; the stderr tail on failure.
package execx
