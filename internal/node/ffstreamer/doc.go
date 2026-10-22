// SPDX-License-Identifier: AGPL-3.0-or-later

// Package ffstreamer is the node's side of ffstreamer (D330), the protocol over which FFmpeg reads
// its input film through the node: two pipes per input, FFmpeg asking for ranges of the film and the
// node answering each with its bytes, read through the node's turn on the film's disk. FFmpeg, patched
// for it (the orpheum-ffmpeg repository, its PROTOCOL.md, draft 6), never opens the film, so a dead
// share holds a goroutine of the node and never FFmpeg in the kernel; and every read is the node's
// to schedule, hold or refuse.
//
// A Channel is one input of one run: Open makes its pipes, FFmpeg inherits ChildFiles and is named
// its input by InputArguments, Start serves it once FFmpeg has started, and Finish says what the
// channel saw once FFmpeg has exited. The run's verdict is FFmpeg's exit status with the channel's
// Account (Account.Succeeded).
//
// # Invariants
//
//   - Every frame FFmpeg sends is judged before it is used, FFmpeg being possibly taken over by the
//     film it reads: its type, its number in sequence, its length within its type's bounds, and its
//     fields within the protocol's and the terms' rules, checked without arithmetic that can
//     overflow. Any breach kills the run and is its account's failure (ErrViolation).
//     TestChannel_KillsARunThatBreaksTheProtocol, TestChannel_KillsARunWhoseOpeningItCannotWorkWith.
//   - A run read its film whole only when it exited 0 and said DONE OK for its own run, and the node
//     had no reason to fail it. TestChannel_ServesAFilmWholeAndJudgesARunByItsDone.
//   - A READ the film's storage cannot answer is never answered: the run is killed (ErrReadFailed),
//     as one served past its budget is before the read that would pass it (ErrOverBudget).
//     TestChannel_KillsARunWhoseFilmCannotBeRead, TestChannel_KillsARunPastItsBudget.
//   - A held run is served nothing until released. TestChannel_AHeldRunIsAnsweredOnlyOnceReleased.
//   - Finish returns whatever FFmpeg left: a writer blocked on a pipe FFmpeg stopped reading is
//     freed, and a storage read stuck in the kernel is not waited for.
//     TestChannel_FinishesWhateverStateFFmpegLeftItIn.
//   - Frames after DONE change nothing. TestChannel_FramesAfterDoneChangeNothing.
//   - A FileSource reads its film through the disk's gate, each read within its place's window, and
//     fails a run whose film changed under it (ErrFilmChanged).
//     TestFileSource_ServesTheFilmItOpenedAndFailsOneThatChanges.
//   - Text from FFmpeg is made safe to log: every Cc and Cf character escaped.
//     TestDecodeDone_ReadsAFailureAndMakesItsMessageSafeToLog.
//   - The data pipe is sized through SyscallConn, never Fd, which would make it blocking and keep
//     Close from freeing a blocked writer (pipe_linux.go).
//
// # Read before changing
//
//   - D330: why FFmpeg reads through the node.
//   - PROTOCOL.md in orpheum-ffmpeg, draft 6, and its grammar, ffstreamer.dogma: every rule here
//     cites its section.
//   - D329: the disk scheduler a Source reads through.
//
// # Where to change
//
//   - frame.go: the frames, their bounds and their checks. channel.go: serving one input.
//     filesource.go: a film the node opened, read through its disk's gate. ffstreamertest: FFmpeg's
//     side in Go, for tests that stand in for the patched FFmpeg.
//     pipe_linux.go and pipe_other.go: the data pipe's size.
package ffstreamer
