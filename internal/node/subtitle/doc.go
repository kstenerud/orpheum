// SPDX-License-Identifier: AGPL-3.0-or-later

// Package subtitle turns a text subtitle into ASS, which JASSUB draws in the browser, and WebVTT,
// which the cast receiver shows (cut-a §6, D217 §6): a track inside a video, which means reading
// the whole file because its packets are spread through it, a file beside the media, or a
// fetched subtitle's text (D224 §5). One extraction makes both, and the video is read once for
// all the tracks asked of it (D340): FFmpeg writes them as ASS into one Matroska, each is taken out
// of it in memory, and the WebVTT is converted from the ASS.
//
// # Invariants
//
//   - Imports nothing from internal/core and never touches the database. Enforced by the depguard
//     rule node-boundary and, transitively, TestArch_NodeNeverImportsCore.
//
//   - Starts no process except through internal/node/execx. Enforced by the depguard rule exec-
//     only-in-execx and TestArch_OnlyExecxImportsOsExec.
//
//   - A video is read by the file: protocol only, or, when the node streams the film, by the
//     fdproxy protocol only, given no path (execx.Film, D330), and a sidecar's bytes and the ASS
//     reach FFmpeg only on a pipe, with every other protocol refused. Enforced by
//     TestExtract_FFmpegReadsOnlyTheFileOrThePipeItIsGiven.
//
//   - An embedded track and a sidecar each come out as ASS and as the WebVTT of that ASS.
//     Enforced by TestExtract_ATrackAndASidecarBecomeASSAndWebVTT (integration tier).
//
//   - A video's text tracks come from one read of it, however many are asked for, and a track that
//     cannot be made fails alone: when it fails the one read, each track is read on its own. A
//     stall ends the whole extraction (D340). Enforced by
//     TestExtract_AVideosTracksComeFromOneReadOfIt and
//     TestExtract_ATrackThatCannotBeMadeFailsAloneAndAStallFailsThemAll.
//
//   - Every run is watched for a stall; a track's run, whose position moves only when a subtitle
//     is written, stands still for ten minutes before it is stopped. Enforced by
//     TestExtract_EveryRunIsWatchedATracksLongEnoughForItsQuietStretches.
//
// # Read before changing
//
//   - D217 §6: subtitles are extracted when it is cheap to.
//   - D340: one extraction per video, for all its text tracks.
//   - D100: a command names its target by path and payload hash.
//   - docs/contributors/design/plans/cut-a.md §6.
//
// # Where to change
//
//   - extract.go: FFmpeg's arguments; tracks.go: a video's tracks from one read. The agent
//     (internal/node/agent) checks the source and reads a sidecar before calling here.
package subtitle
