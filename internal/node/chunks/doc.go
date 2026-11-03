// SPDX-License-Identifier: AGPL-3.0-or-later

// Package chunks makes runs of chunks: one stream of a film, the copied picture, a quality, a
// sound made or a sound copied, cut at the film's grid and written into a store in the node's
// prepared directory, chunk by chunk, each at its time in the film (D247 §10; plans/chunks.md §2).
// A run is one FFmpeg command through execx, whose segment muxer writes raw fragmented MP4 files;
// each, once FFmpeg lists it finished, is split into the store's init and the chunk, its decode
// times moved to the film's.
//
// # Invariants
//
//   - Imports nothing from internal/core and never touches the database. Enforced by the depguard
//     rule node-boundary and, transitively, TestArch_NodeNeverImportsCore.
//   - Starts no process except through internal/node/execx. Enforced by the depguard rule exec-
//     only-in-execx, the forbidigo rules tagged [exec] and [execve], and
//     TestArch_OnlyExecxImportsOsExec.
//   - FFmpeg writes only in the store a run names, inside the node's prepared directory: the one
//     directory its confinement may write, and every path its arguments name. A store named any
//     other way, absolute, climbing or not of a store's shape, is refused before anything is made.
//     TestMake_FFmpegWritesOnlyInItsStoreInsideThePreparedDirectory.
//   - Removes, renames and truncates nothing itself: the store, its files and the raw files are
//     the prepared directory's keeper's (prepared.Shelf). Enforced by the forbidigo rule tagged
//     [delete].
//   - A run reads its chunks and cuts at each bound after its first and at its end, a picture a
//     little early so its keyframe is not missed, a re-timed copy early enough for dts2pts showing
//     its keyframes each its own while after their decode times; an encode keyframes at its chunks'
//     starts and nowhere else; a sound past the film's first chunk starts a chunk early. Enforced by
//     TestArguments_ReadTheRunsChunksAndCutAtEachAndAtTheEnd,
//     TestMake_ARetimedCopyIsCutAtAKeyframeShownSoonerThanItsFirst,
//     TestArguments_EncodeAQualityWithKeyframesAtItsChunksStarts and
//     TestArguments_StartASoundAChunkEarlyAndCutItAtTheBounds.
//   - A raw file holds its run's one stream, never a timecode track the MP4 muxer would add for a
//     picture carrying a timecode, as an MXF's or a camera's MOV does (D360).
//     TestArguments_WriteNoTimecodeTrackBesideTheStream.
//   - Each chunk is put in its store whole, as FFmpeg finishes it, at its time in the film plus
//     TimelineOffsetMs: a picture's shown at its bound, a sound's after the run's samples before
//     it less the encoder's priming; a lead-in and what FFmpeg wrote past the run are dropped.
//     Enforced by TestMake_PutsEachChunkAtItsTimeAsFFmpegFinishesIt and
//     TestMake_TimesASoundBySamplesAfterItsLeadIn; with the pinned FFmpeg, two runs' chunks join
//     into one stream at the film's times, by TestMake_TwoRunsChunksJoinIntoOneStreamAtTheFilmsTimes
//     (integration tier).
//   - A copied picture's run seeks past its first keyframe, before the next, as the core says:
//     FFmpeg moves a seek into Matroska 130 ms earlier, so a seek at the keyframe lands on the one
//     before it and every chunk of the run would hold the one before it. A decoded stream seeks to
//     its start. Enforced by TestArguments_ReadTheRunsChunksAndCutAtEachAndAtTheEnd and, with the
//     pinned FFmpeg, TestMake_TwoRunsChunksJoinIntoOneStreamAtTheFilmsTimes, whose Matroska
//     picture's packets must be the film's. An MPEG-TS run, which FFmpeg seeks to a frame and not a
//     keyframe, seeks at or before its first keyframe instead, as the core says, and every copied
//     picture's times start at its first keyframe (-avoid_negative_ts make_zero), so an MPEG-TS's
//     chunks are the film's too, its one keyframe its first included. Enforced by
//     TestArguments_ACopiedPictureIsTimedFromItsFirstKeyframe,
//     TestMake_AnMPEGTSPicturesRunSeeksAtOrBeforeItsFirstKeyframe,
//     TestMake_RefusesARunNotOfARunsShape and, with the pinned FFmpeg,
//     TestMake_TwoRunsChunksJoinIntoOneStreamAtTheFilmsTimes and
//     TestMake_AnMPEGTSWhoseOnlyKeyframeIsItsFirstIsOneChunkMadeWhole.
//   - A copied picture's run past the film's first chunk is led in from the chunk before, as the
//     core says: it seeks past that chunk's keyframe, cuts at the run's first chunk too and drops
//     the lead-in's raw file uncounted, so FFmpeg times an open GOP's pictures shown before the run's
//     first keyframe and decoded after it as the film does; begun at that keyframe, it crushes their
//     times together and leaves a hole at the join (idiosyncrasies.md; The Pumbletons S33E8). Enforced
//     by TestArguments_ACopiedPictureIsLedInFromTheChunkBefore, TestMake_DropsACopiedPicturesLeadIn,
//     TestMake_RefusesARunNotOfARunsShape and, with the pinned FFmpeg and an open GOP's HEVC,
//     TestMake_TwoRunsChunksJoinIntoOneStreamAtTheFilmsTimes.
//   - A sound copied is the film's own track, every channel, packet for packet: run from a lead-in,
//     at the film's own times, its first packet put a tick before the film so the muxer, which counts
//     its cuts from a run's first packet, cuts every run at the same packets; each chunk is placed by
//     its listed end less its samples, and the init waits for the first packet, as AC-3 needs
//     (D368 §1, idiosyncrasies.md). A run of the film's chunk 0 starting more than the list's
//     tolerance into the film, its first keyframe before its start discarded, is led in from the
//     film's start and cut at chunk 0's start too, the lead-in named 00000 and each raw file a chunk
//     past its own. Enforced by
//     TestArguments_CopyASoundAtItsTimesInTheFilmAndCutItAtTheBounds,
//     TestArguments_ACopiedSoundWhoseFilmsFirstChunkStartsLateIsLedInFromTheFilmsStart,
//     TestMake_TimesACopiedSoundByItsListedEndInTheFilm,
//     TestMake_ACopiedSoundWhoseFilmsFirstChunkStartsLateDropsWhatIsBeforeIt and
//     TestDuration_IsEachSamplesOwnOrTheFragmentsOrTheTracksDefault; with the pinned FFmpeg, two
//     runs' chunks of a 5.1 AAC, a stereo AAC and a 5.1 AC-3 film are the film's packets bit for bit,
//     each once, at their times, by TestMake_ACopiedSoundIsTheFilmsOwnPacketsBitForBitAtTheirTimes,
//     and of a film whose grid starts past its start, from chunk 0's, by
//     TestMake_ACopiedSoundOfAFilmWhoseGridStartsPastItsStartIsCutAtItsFirstChunk (integration tier).
//   - A sound interleaved in super-blocks, as the core says RealMedia's cook is, decodes from ten
//     seconds before its run, not two, and from the film's start without a seek: FFmpeg loses the
//     super-block a seek lands in (idiosyncrasies.md). Enforced by
//     TestArguments_AnInterleavedSoundSeeksTenSecondsEarlyAndNeverToTheStart and, with the pinned
//     FFmpeg, TestMake_AnInterleavedRealMediaSoundIsWholeAndAtItsTimesInEveryRun.
//   - No run seeks to the film's start; one reading from it reads unsought: FFmpeg moves a seek into a
//     film with B-frames 3/23 s earlier, before the start, and an AVI seeking there loses its first
//     keyframe and the sound interleaved ahead of its picture, which put a sound half a second early
//     (idiosyncrasies.md). Enforced by TestArguments_NoRunSeeksToTheFilmsStart and, with the pinned
//     FFmpeg, TestMake_AnAVIsSoundInterleavedAheadOfItsPictureIsAtItsTimesFromTheFilmsStart.
//   - A run of a picture marked to be re-timed, its B-frames carrying no display times, rebuilds them
//     with FFmpeg's dts2pts, a copy on its output and an encode on its picture's input, moved back so
//     its first keyframe is shown at its bound, and reads a second past its end, an encode keyframed
//     there too, so the frames before its end are timed from those after it and what is past it is cut
//     off; no other run is (design/plans/audio-switch-and-retiming-2026-11-01.md, Part 2). Enforced by
//     TestArguments_ReTimeAPictureWhoseBFramesCarryNoDisplayTimes; with the pinned FFmpeg, two runs'
//     chunks of a flattened H.264 and HEVC clip have the clip's own composition offsets, but its last
//     few pictures', and an encode of it every picture once, by
//     TestMake_APictureWhoseBFramesCarryNoDisplayTimesIsReTimedWhenMarked (integration tier).
//   - A raw file is made a chunk only when the span the muxer lists for it is its chunk's, and the
//     run's last only once FFmpeg has ended: FFmpeg stopped in a file closes it and lists it as
//     finished; the refusal says where the run read from and first cut. Enforced by
//     TestMake_NeverMakesAChunkOfAFileNotCutAtItsBounds and
//     TestMake_ACopiedSoundWhoseFilmsFirstChunkStartsLateDropsWhatIsBeforeIt.
//   - A run not of a run's shape is refused before anything is made. Enforced by
//     TestMake_RefusesARunNotOfARunsShape.
//   - What is not a raw file of the muxer's is refused, never read past its end. Enforced by
//     TestFragments_RefuseWhatIsNotARawFile.
//
// # Read before changing
//
//   - D247 §10: chunks made on request, cut at the source's keyframes.
//   - docs/contributors/design/research/slow-links.md §10: the recipe, measured.
//   - docs/contributors/design/plans/chunks.md: the build plan.
//   - D217 §5: the node owns its prepared directory and its budget.
//
// # Where to change
//
//   - arguments.go: the FFmpeg command line. maker.go: one run, its raw files and their times.
//     fmp4.go: reading and retiming fragmented MP4. listed.go: the muxer's list and the spans it
//     says.
package chunks
