// SPDX-License-Identifier: GPL-3.0-or-later

// Package readthrough reads a video through once, with one FFmpeg, and learns what only a read of
// the whole file tells. How loud each audio track is, by EBU R128 with FFmpeg's ebur128 filter, so
// the core can set a gain that brings films to one level and the player can apply it (D226): a
// track with more than two channels is measured as it is, which a receiver plays, and as the
// standard stereo mix a prepare makes of it, which a stereo device hears, since that mix
// renormalises its gains and plays 7 to 8 dB quieter. And the picture's timeline, from its packets
// copied undecoded into FFmpeg's framecrc muxer: its keyframes, where a copy of it can be cut, and
// its bytes in each second of the film, the peaks a link must carry (D247 §10). A read asks for
// either or both; both come from the one read of the file.
//
// # Invariants
//
//   - Imports nothing from internal/core and never touches the database. Enforced by the depguard
//     rule node-boundary and, transitively, TestArch_NodeNeverImportsCore.
//
//   - Starts no process except through internal/node/execx. Enforced by the depguard rule exec-
//     only-in-execx and TestArch_OnlyExecxImportsOsExec.
//
//   - The video is read only as a local file, with every other protocol refused, and each sound
//     is measured through its own named filter, the stereo mix being prepare's own. Enforced by
//     TestRead_ReadsOnlyTheFileAndMeasuresEachSoundByName.
//
//   - The timeline is read from the one read that measures the sound, by the picture's stream
//     index among all the file's streams, copied and never decoded, and as FFmpeg writes it, so no
//     film's packets are held: its keyframes in time order, each once, in milliseconds of the
//     film's time base, and every packet's bytes in the second it is shown in. Enforced by
//     TestRead_TheTimelineComesFromTheSameReadAsTheLoudness and
//     TestPacketLines_KeepTheKeyframesInOrderOnceAndTheBytesOfEachSecond.
//
//   - A measurement matches what FFmpeg measures of a sound of known level, as it is and as the
//     stereo mix, and a timeline the keyframes and seconds of a picture made with known ones.
//     Enforced by TestRead_AToneOfKnownLevelMeasuresAsItIsAndItsPictureHasItsKeyframes
//     (integration tier).
//
//   - A read is watched for a stall, with the deadline of reading the whole file, or, when it
//     measures loudness, of decoding its sound at real time when the core says how long it is and
//     that takes longer. Enforced by TestRead_IsWatchedForAPositionThatStandsStillAndSaysWhenItStalled
//     and TestRead_ALongFilmIsAllowedAsLongAsDecodingItsSoundTakesOnlyWhenItsSoundIsMeasured.
//
// # Read before changing
//
//   - D226: what is measured and why the stereo mix is measured too.
//   - D83: loudness is measured once and applied as gain.
//   - D247 §10: the keyframes a film's chunks are cut at, and the measured recipe
//     (docs/contributors/design/research/slow-links.md §10).
//   - D100: a command names its target by path and payload hash.
//
// # Where to change
//
//   - read.go: the one FFmpeg run, its outputs and its watchdog. loudness.go: the loudness output's
//     filters and the reading of their summaries. timeline.go: the timeline's output and the
//     reading of framecrc's lines. The agent (internal/node/agent) checks the source before
//     calling here.
package readthrough
