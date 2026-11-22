// SPDX-License-Identifier: AGPL-3.0-or-later

// Package probe runs ffprobe on one file, through execx, and turns its output into the probe
// record the core stores verbatim. Hostile bytes stop here: the core receives only the parsed
// record.
//
// # Invariants
//
//   - ffprobe's JSON is another program's (D458): read by strictjson.Foreign, anything JSON allows
//     within D456's limits, and a tag said twice keeps its first value, as ffprobe shows it
//     (ffprobeOutput). TestParse_ATagSaidTwiceKeepsItsFirstValueAsFfprobeShowsIt.
//   - Imports nothing from internal/core and never touches the database. Enforced by the depguard
//     rule node-boundary and, transitively, TestArch_NodeNeverImportsCore.
//   - Starts no process except through internal/node/execx. Enforced by the depguard rule exec-
//     only-in-execx, the forbidigo rules tagged [exec] and [execve], and
//     TestArch_OnlyExecxImportsOsExec.
//   - Removes, renames and truncates nothing. Enforced by the forbidigo rule tagged [delete].
//   - ffprobe reads the input by the file: protocol only, so no filename is read as a URL or a
//     concat list, or, when the node streams the film, by the fdproxy protocol only, given no
//     path (execx.Film, D330). TestProbe_RunsFFprobeOnTheFileProtocolOnly.
//   - A record is bounded: valid UTF-8, bounded strings, and a file with too many streams or
//     chapters is refused rather than cut short. TestParse_BoundsWhatAHostileFileCanMakeTheNodeReport.
//   - A disc image's main title is told what only the disc says: which disc and title it is, its
//     duration, its chapters where the stream has none, and its streams' languages by identifier,
//     a stream's own kept (D350 §4). TestProbe_ADiscsTitleCarriesWhatOnlyTheDiscSays; with the
//     pinned ffprobe reading both discs' titles through fdproxy,
//     TestProbe_ThePinnedFFprobeReadsADiscsMainTitleServedFromItsImage (integration tier).
//   - The same output always makes the same record, whatever order Go iterates a map in
//     (scanner.md: deleting the node's cache changes nothing it reports).
//     TestParse_IsTheSameRecordEveryTime.
//   - An H.264 or HEVC picture that says it reorders frames (has_b_frames), yet whose every packet
//     among the file's first 200, at least 24 of its own, is shown at its decode time, is marked to be
//     re-timed, judged by the first ffprobe call and those packets alone; nothing else is
//     (design/plans/audio-switch-and-retiming-2026-11-01.md §2.4).
//     TestParse_APictureWhoseBFramesCarryNoDisplayTimesNeedsRetiming,
//     TestParse_JudgesRetimingByTheFirstPacketsAlone; with the pinned ffprobe, on clips flattened as
//     file 5667 is, TestProbe_MarksAPictureWhoseBFramesCarryNoDisplayTimes (integration tier).
//   - A sound whose first packet alone lasts more than a second, each of at least its next 8 a frame
//     (at most 250 ms), is marked with how much longer its first lasts than its second, the stretch
//     the core and the sound made take as a wrong time (D534 §1); nothing else is. It is judged by the
//     file's first 200 packets, or, where they hold its first, lasting seconds, and too few after it,
//     by a second ffprobe call listing its first 25 packets alone, at most 16 tracks so listed: file
//     18306's first 200 hold one of its sound's. A file whose sounds' first packets last a frame is
//     probed by one call. TestParse_ASoundWhoseFirstPacketAloneLastsSecondsIsMarkedWithTheStretch,
//     TestProbe_ASoundWhoseLongFirstPacketTheFirstPacketsCannotJudgeIsListedAlone; with the pinned
//     ffprobe, TestProbe_MarksASoundWhoseFirstPacketAloneLastsSeconds (integration tier).
//   - When a file says it was recorded is a QuickTime creationdate, else its container's
//     creation_time, else its first video track's, and a clock's zero or a time before 1950 is none
//     (D349 §4). TestParse_TheTimeAFileSaysItWasRecordedAt.
//
// # Read before changing
//
//   - D126: hostile bytes never reach the core.
//   - D26: FFmpeg is confined.
//   - D112: the node reports its confinement.
//   - docs/contributors/design/plans/scanner.md.
//
// # Where to change
//
//   - The ffprobe arguments and the output parser: probe.go runs ffprobe, bounded in what it may
//     say; soundstart.go judges a sound's first packet (D534 §1), listing a track's first packets
//     alone where it must; parse.go reads its output into the record, refusing a file past the
//     bounds on streams and chapters. disc.go: the record of a disc's main title.
package probe
