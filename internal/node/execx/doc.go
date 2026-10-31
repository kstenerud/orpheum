// SPDX-License-Identifier: AGPL-3.0-or-later

// Package execx is the only place a shipping process starts. It runs the bundled ffmpeg and
// ffprobe, and nothing else, with hardened arguments, an empty environment, the root directory as its
// working directory and stdin closed, in a process group, and that on Linux the kernel kills when the
// node dies. Every signal it sends a run goes to the process itself, through its pidfd where the
// kernel has one, then to its group (signalRun): a confined tool can neither start a process nor
// leave its group (internal/node/confine), so the process is what must be reached. A failure carries the exit
// status and the tail of the tool's stderr. A long FFmpeg run is watched (RunWatched): it reports
// -progress on a pipe of its own, and the watchdog stops it when its position does not advance. A confined
// runner (NewConfinedRunner) starts every run as orpheum's shim, which confines itself to the
// run's Access (internal/node/confine) and then becomes the tool (cut-a §7); the node's runner is
// confined, and only a test's fake FFmpeg runs unconfined.
//
// # Invariants
//
//   - A watched run made on a context carrying a Pausing is paused with SIGSTOP to its process and
//     its group and continued with SIGCONT, and its paused time is neither a stall nor spent deadline
//     (D240 §6). TestRunWatched_APausedRunNeitherStallsNorSpendsItsDeadlineAndIsJudgedAgainOnceContinued.
//   - It is the only shipping package that imports os/exec or calls exec.Command; the test-only
//     internal/testutil is another, and the owner's signing tool's cmd/discovery-sign/keygen.go,
//     which runs ssh-keygen on the owner's machine, the last (D394 §5). Enforced by the depguard rule exec-only-in-execx, the
//     forbidigo rule tagged [exec], and TestArch_OnlyExecxImportsOsExec.
//   - Imports nothing from internal/core. Enforced by the depguard rule node-boundary and
//     TestArch_NodeNeverImportsCore.
//   - Errors from the process are wrapped where they enter. Enforced by wrapcheck, which is scoped
//     to this package.
//   - A tool gets the hardening flags first and none of the node's environment (D204), so no
//     LD_LIBRARY_PATH, LD_PRELOAD or LIBVA_DRIVERS_PATH steers what it loads.
//     TestRun_PassesHardenedArgumentsAndAnEmptyEnvironment,
//     TestRun_NoLibraryOrDriverPathInTheNodesEnvironmentReachesTheTool.
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
//   - A watched run can give its stdout to a writer as it comes, keeping none, for an answer too
//     long to hold. TestRunWatched_GivesStdoutToItsWriterAsItComesAndKeepsNone.
//   - A watched run is stalled when its position (out_time, or the frame count) has not advanced:
//     not within its start grace, not for its stall window once it has, or not to its end by its
//     deadline. Progress lines that arrive with the position where it was are not movement.
//     TestWatchdog_OnlyAPositionThatAdvancesIsProgressNotLinesThatArrive,
//     TestWatchdog_ARunThatNeverMovesOrOutrunsItsDeadlineHasStalled.
//   - Bytes its process group has read from or written to storage, or it has written to stdout,
//     are movement too, though not the position's. TestWatchdog_BytesReadOrWrittenAreMovementThoughThePositionStands.
//   - A stall says what the group was doing, by /proc: waiting on the disk when a process is in
//     uninterruptible sleep, spinning when it used half a processor or more, stuck otherwise (D233 §5).
//     TestWatchdog_AStallSaysWhetherItWaitedOnTheDiskSpunOrWasStuck,
//     TestRunWatched_AStalledRunSaysWhatItsGroupWasDoing.
//   - A run whose process outlives SIGKILL by killGrace, as one waiting on a device does, is
//     answered at once, as stalled on the disk or cancelled, and listed by Stuck until it ends
//     (D233 §10). TestRunWatched_ARunThatOutlivesItsKillIsAnsweredAndListedAsStuckUntilItEnds.
//   - A deadline is three times the run's expected length, at least ten minutes, and a day when
//     the length is unknown.
//     TestDeadlineFor_IsThreeTimesTheExpectedLengthAtLeastTenMinutesAndADayWhenUnknown.
//   - A stalled run is stopped with SIGINT, FFmpeg's graceful quit, then SIGTERM, then SIGKILL of
//     its process and its whole group, and returns a StalledError, which is not a RunError: a hang is not the
//     tool's word on its input. TestRunWatched_AStalledRunIsQuitGracefullyAndSaysWhereItStood,
//     TestRunWatched_AStalledRunThatIgnoresItsQuitIsKilledWithItsGroup.
//   - On Linux no tool outlives the node that started it. TestRun_AToolDiesWithTheNode.
//   - A run already reaped is never signalled, nor is its group, whose id may be another's by then
//     (fdproxy's specification, Stopping). TestSignalRun_LeavesTheGroupOfARunAlreadyReapedAlone.
//   - A watched run may read its film through the node (WatchedRun.Film, D330): its channel's pipes
//     follow the progress pipe at descriptors 4 and 5, named by StreamedInput. Such a run passes only
//     having said DONE OK for its own run, a breach of the protocol or a passed budget kills it at once
//     and fails it as the node's verdict, and a film whose disk failed or did not answer is a stall on
//     its disk. judgeFilm is the one judge of a streamed run's end:
//     TestJudgeFilm_PassesOnlyARunThatExited0WithDoneOKForItsOwnRun,
//     TestRunWatched_AStreamedRunReadsItsFilmThroughTheNodeAndPassesOnItsDone,
//     TestRunWatched_AStreamedRunThatBreaksTheProtocolIsKilledAndFails,
//     TestRunWatched_AStreamedRunWhoseFilmCannotBeReadStalledOnItsDisk.
//   - A paused streamed run reads nothing: its READs are held as its process is stopped.
//     TestRunWatched_APausedStreamedRunReadsNothingUntilContinued.
//   - A run names its film one way, Film: by its path, granted to read that path alone, or streamed,
//     granted no path to it. A run that is not watched, as ffprobe's is, reads a streamed film at the
//     same descriptors, after a closed descriptor 3 (RunOnFilm). Bounding a run by its film's size
//     never stats the film: the size is the one the node opened it with.
//     TestFilm_AStreamedFilmGrantsNoPathAndAFilmByPathGrantsOnlyItsOwn,
//     TestRunOnFilm_AnUnwatchedRunReadsItsStreamedFilmThroughTheNode,
//     TestReadingDeadline_IsReadingTheFileAtTheSlowestReadTheNodeIsConfiguredFor.
//   - A film is streamed only by an FFmpeg that lists the fdproxy protocol (fdproxy.ProtocolName)
//     among its input protocols (HasFDProxy). TestHasFDProxy_IsTheProtocolAmongTheInputsFFmpegLists.
//   - FilmOf is the one place a run's film is made: by path, or, once StreamFilms, served from the file
//     the node opened, through its disk's gate, each run numbered and bounded by what it reads, a
//     probe far less than a whole read. TestFilmOf_StreamsTheOpenedFileOnTheTermsOfWhatTheRunReads.
//   - A disc image's film is its main title (internal/node/disc), served from the image's runs that
//     hold it, sized and budgeted as the title and carrying it for the probe, its structure read once
//     through the disk's gate; a runner that cannot stream a film refuses one (ErrDiscNeedsStreaming,
//     D350). TestFilmOf_ADiscImageIsServedAsItsMainTitleAndOnlyStreamed.
//   - That a streamed film's pipe kept its default size, the user's pipe memory spent, is logged once
//     (LoggingTo). TestRunner_SaysOnceThatAStreamedFilmsPipeKeptItsDefaultSize.
//
// # Read before changing
//
//   - D26: no FFmpeg in the core; confine it.
//   - D43: confinement is per invocation.
//   - D112: the node probes its confinement and reports it.
//   - D190 §4: the fake ffmpeg fixture asserts the arguments and environment.
//   - D233 §5, §10: a stall classified, and a process that outlives its kill.
//   - D350: a disc image plays its main title, served through fdproxy.
//   - docs/contributors/design/plans/cut-a.md §7.
//
// # Where to change
//
//   - execx.go: starting a run, through the shim or not; the argument hardening and a run's
//     input; the stderr tail on failure. watch.go: the watchdog, its progress and its deadlines.
//     pause.go: pausing a watched run for play-driven work. film.go: a run's film, by path or
//     served through the node (internal/node/fdproxy), and whether FFmpeg can be served one.
//     disc.go: a disc image's main title as a run's film.
//     proc_linux.go: reading a process group from /proc. stuck.go: the processes left running.
//     process_linux.go and process_other.go: a tool's process group and parent-death signal.
//     The shim itself is internal/node/confine's.
package execx
