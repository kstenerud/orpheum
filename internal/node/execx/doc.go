// SPDX-License-Identifier: GPL-3.0-or-later

// Package execx is the only place a shipping process starts. It runs the bundled ffmpeg and
// ffprobe, and nothing else, with hardened arguments, an empty environment, the root directory as its
// working directory and stdin closed, in a process group that is killed whole when the run is
// cancelled, and that on Linux the kernel kills when the node dies. A failure carries the exit
// status and the tail of the tool's stderr. A long FFmpeg run is watched (RunWatched): it reports
// -progress on a pipe of its own, and the watchdog stops it when its position does not advance. A confined
// runner (NewConfinedRunner) starts every run as orpheum's shim, which confines itself to the
// run's Access (internal/node/confine) and then becomes the tool (cut-a §7); the node's runner is
// confined, and only a test's fake FFmpeg runs unconfined.
//
// # Invariants
//
//   - A watched run made on a context carrying a Pausing is paused with SIGSTOP to its process
//     group and continued with SIGCONT, and its paused time is neither a stall nor spent deadline
//     (D240 §6). TestRunWatched_APausedRunNeitherStallsNorSpendsItsDeadlineAndIsJudgedAgainOnceContinued.
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
//   - A watched run can return the end of the tool's log, where FFmpeg's filters write what they
//     measured, and none of its output. TestRunWatched_ReturnsTheEndOfTheLogAndNotTheOutput.
//   - A watched run is stalled when its position (out_time, or the frame count) has not advanced:
//     not within its start grace, not for its stall window once it has, or not to its end by its
//     deadline. Progress lines that arrive with the position where it was are not movement.
//     TestWatchdog_OnlyAPositionThatAdvancesIsProgressNotLinesThatArrive,
//     TestWatchdog_ARunThatNeverMovesOrOutrunsItsDeadlineHasStalled.
//   - A deadline is three times the run's expected length, at least ten minutes, and a day when
//     the length is unknown.
//     TestDeadlineFor_IsThreeTimesTheExpectedLengthAtLeastTenMinutesAndADayWhenUnknown.
//   - A stalled run is stopped with SIGINT, FFmpeg's graceful quit, then SIGTERM, then SIGKILL of
//     its whole group, and returns a StalledError, which is not a RunError: a hang is not the
//     tool's word on its input. TestRunWatched_AStalledRunIsQuitGracefullyAndSaysWhereItStood,
//     TestRunWatched_AStalledRunThatIgnoresItsQuitIsKilledWithItsGroup.
//   - On Linux no tool outlives the node that started it. TestRun_AToolDiesWithTheNode.
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
//     input; the stderr tail on failure. watch.go: the watchdog, its progress and its deadlines.
//     pause.go: pausing a watched run for play-driven work.
//     process_linux.go and process_other.go: a tool's process group and parent-death signal.
//     The shim itself is internal/node/confine's.
package execx
