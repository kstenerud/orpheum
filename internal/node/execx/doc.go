// SPDX-License-Identifier: GPL-3.0-or-later

// Package execx is the only place a shipping process starts. It runs the bundled ffmpeg and
// ffprobe, and orpheum's own binary in another mode as a long-lived child (StartSelf), and nothing
// else, with hardened arguments, an empty environment, the root directory as its
// working directory and stdin closed, in a process group that is killed whole when the run is
// cancelled. A failure carries the exit status and the tail of the tool's stderr. A confined
// runner (NewConfinedRunner) starts every run as orpheum's shim, which confines itself to the
// run's Access (internal/node/confine) and then becomes the tool (cut-a §7); the node's runner is
// confined, and only a test's fake FFmpeg runs unconfined.
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
//   - A run names its input one way: FileInput, a file with only the file protocol allowed, or
//     PipeInput, stdin with only the pipe protocol. TestExtract_FFmpegReadsOnlyTheFileOrThePipeItIsGiven.
//   - A failure says why: its status and the last stderrTailBytes of stderr.
//     TestRun_AFailureCarriesTheStatusAndTheTailOfStderr.
//   - Cancelling a run kills everything it started, so no FFmpeg outlives its job.
//     TestRun_CancellingKillsTheWholeProcessGroup.
//   - A run that writes more than its caller allows is refused, never truncated.
//     TestRun_RefusesMoreOutputThanAllowed.
//   - A logged run returns the end of the tool's log, where FFmpeg's filters write what they
//     measured, and none of its output. TestRunLogged_ReturnsTheEndOfTheLogAndNotTheOutput.
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
//   - execx.go: starting a run, through the shim or not; the argument hardening and a run's
//     input; the stderr tail on failure. child.go: StartSelf, orpheum's own binary as a long-lived
//     child. The shim itself is internal/node/confine's.
package execx
