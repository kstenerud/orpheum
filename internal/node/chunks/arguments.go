// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/internal/node/encode"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// cutEarlyMs is how much before a chunk's start the muxer is told to cut: a keyframe at
// 10.009999 s is otherwise not taken for one at 10.010 s (Kyoo's hls_time_delta; slow-links.md
// §10). No two keyframes of the grid are this close.
const cutEarlyMs = 50

// retimedCutEarlyMs is how much before a chunk's start the muxer is told to cut a re-timed copy
// (isRetimed). The muxer times its cuts from its first packet's display time, and dts2pts shows each
// keyframe after its decode time by as many pictures as are reordered before it, which differs from
// keyframe to keyframe: an HEVC keyframe of The Gold Rush is shown 83 to 250 ms after it is decoded, so
// a run whose first keyframe is shown 250 ms late takes a later one shown 125 ms late for 125 ms
// early, and a cut 50 ms early missed it: chunk 349 could never be made (staging, 2026-11-03;
// toolkit.md item 12). Cut this early, a keyframe is missed only if its film's delays differ by about
// ten pictures at 24 fps, and the keyframe before it is cut at instead only if the two are less than
// about two thirds of a second apart, closer than encoders place them.
const retimedCutEarlyMs = 400

// rawListName is the segment muxer's list of the raw files it has finished, one line each, in the
// run's raw directory.
const rawListName = "list.csv"

// runStartMs is where a run starts reading: its first chunk's start, or, for a sound or a copied
// picture past the film's first chunk, the chunk before it, whose chunk is dropped (its LeadInMs);
// or, for a sound copied whose film's first chunk starts well after the film does
// (isLedInFromFilmStart), the film's start.
func runStartMs(command nodeproto.MakeChunks) int64 {
	if command.LeadInMs > 0 {
		return command.LeadInMs
	}
	if isLedInFromFilmStart(command) {
		return 0
	}
	return command.BoundsMs[0]
}

// isLedInFromFilmStart is whether a run of a sound copied makes the film's first chunk, which starts
// more than spanToleranceMs after the film does: its picture's first keyframe a read-through counts
// is that late, as a film whose first keyframe an edit list puts before its start, and drops, starts
// at its next, 10.2 s in (a film, staging, 2026-10-28). The copy is cut where chunk 0
// starts, as the picture is, and what is before it is a lead-in from the film's start, dropped: the
// muxer, which cuts only at the times it is given, put all the film read before chunk 0's end in it,
// and the seek to chunk 0's start, which FFmpeg lands on the picture's keyframe at or before it,
// the dropped one (10.176752 s is after 10.176), read from the film's start. A first chunk within
// spanToleranceMs of the film's start keeps the sound from the film's start, the priming with it.
func isLedInFromFilmStart(command nodeproto.MakeChunks) bool {
	return command.Stream.Kind == nodeproto.StreamSoundCopied && command.FirstChunk == 0 &&
		command.BoundsMs[0] > spanToleranceMs
}

// soundSeekEarlyMs is how long before its run a sound's decode starts, trimmed after decoding: a
// run starts on a keyframe, and FFmpeg seeking an Opus track (and any with a seek pre-roll) to one
// decodes it without the pre-roll before it, which put the sound 60 to 90 ms later than its
// timestamps and made a seam at every run's start (idiosyncrasies.md). Two seconds is many
// pre-rolls; the decode trims to the run's start by its decoded timestamps.
const soundSeekEarlyMs = 2_000

// interleavedSoundSeekEarlyMs is soundSeekEarlyMs for a sound interleaved in super-blocks
// (nodeproto.MakeChunks.IsSoundInterleaved): FFmpeg's RealMedia demuxer, its seek landing on a
// picture's keyframe partway through a super-block, drops or garbles the sound until the next
// super-block, and at times the one after, which a cook super-block of about two seconds put past a
// run's start (idiosyncrasies.md). Ten seconds is past two of the longest.
const interleavedSoundSeekEarlyMs = 10_000

// seekMs is where FFmpeg is told to seek: a copied picture's seek, past its first keyframe, so
// FFmpeg lands on it (nodeproto.MakeChunks.PictureSeekMs); a sound's soundSeekEarlyMs before its
// run's start, or an interleaved one's interleavedSoundSeekEarlyMs, which its filter trims to
// (soundTrimMs); otherwise the run's start, which a decode trims to exactly, and from which a
// sound copied copies what FFmpeg lands on, a little before it.
func seekMs(command nodeproto.MakeChunks) int64 {
	switch command.Stream.Kind {
	case nodeproto.StreamPicture:
		return command.PictureSeekMs
	case nodeproto.StreamSound:
		if command.IsSoundInterleaved {
			return max(0, runStartMs(command)-interleavedSoundSeekEarlyMs)
		}
		return max(0, runStartMs(command)-soundSeekEarlyMs)
	case nodeproto.StreamQuality, nodeproto.StreamSoundCopied:
		return runStartMs(command)
	}
	return runStartMs(command)
}

// isSeeking is whether FFmpeg is told to seek at all: not to the film's start, which a run reads from
// unsought. FFmpeg moves a seek into a film with B-frames 3/23 s earlier, before the start, and an
// AVI seeking there lands past its first keyframe, losing the film's first packets: the
// first 21 frames of a sound interleaved ahead of its picture, which put a sound, timed by its
// samples, half a second early for all its run (Quillwick, staging, 2026-11-01), and the picture's
// first keyframe; and RealMedia loses its first super-block, as any seek loses the one it lands in
// (idiosyncrasies.md).
func isSeeking(command nodeproto.MakeChunks) bool {
	return seekMs(command) > 0
}

// soundTrimMs is how much a sound's decode drops before its run's start: what it was decoded early.
func soundTrimMs(command nodeproto.MakeChunks) int64 {
	return runStartMs(command) - seekMs(command)
}

// firstRawNumber is the chunk the run's first raw file is: its first chunk, or the one before it
// for a lead-in, -1 for one led in from the film's start (isLedInFromFilmStart).
func firstRawNumber(command nodeproto.MakeChunks) int {
	if runStartMs(command) != command.BoundsMs[0] {
		return command.FirstChunk - 1
	}
	return command.FirstChunk
}

// rawNumberShift is how far past its chunk's number a raw file's is: one for a run led in from the
// film's start, whose lead-in, chunk -1, the muxer, numbering from zero at the least, names 00000;
// none otherwise.
func rawNumberShift(command nodeproto.MakeChunks) int { return max(0, -firstRawNumber(command)) }

// seconds is milliseconds as FFmpeg's seconds.
func seconds(ms int64) string { return strconv.FormatFloat(float64(ms)/1000, 'f', 3, 64) }

// cutTimes are where the muxer cuts: every chunk boundary after the run's start, the end of the
// last chunk among them, so what FFmpeg writes past it lands in a raw file that is dropped. The
// muxer times its cuts from the run's first packet: relative to the run's start, its first packet's
// time, but for a sound copied, whose first packet is put a tick before the film's start
// (copiedSoundTimes), so it is cut at the film's own times, a tick early alike in every run. A
// picture is cut a little early, so its keyframe is not missed;
// a re-timed copy more so (retimedCutEarlyMs); a sound is cut at the first frame at or after the time.
func cutTimes(command nodeproto.MakeChunks) string {
	origin := runStartMs(command)
	if command.Stream.Kind == nodeproto.StreamSoundCopied {
		origin = 0
	}
	early := int64(cutEarlyMs)
	switch {
	case command.Stream.Kind.IsSound():
		early = 0
	case isRetimed(command) && command.Stream.Kind == nodeproto.StreamPicture:
		early = retimedCutEarlyMs
	}
	var cuts []string
	for _, bound := range command.BoundsMs {
		if bound > runStartMs(command) {
			cuts = append(cuts, seconds(bound-origin-early))
		}
	}
	return strings.Join(cuts, ",")
}

// copiedSoundTimes keeps a copied sound's packets at their times in the film, the file's start at
// zero (-copyts -start_at_zero), as no seek moves them: FFmpeg copies from a little before its seek,
// by as much as the demuxer lands before it (85 ms into an MP4's AAC), so a run's first packet is not
// known before it runs, and the segment muxer times its cuts from that packet (idiosyncrasies.md).
// The first packet alone is put one tick before the film's start, so every cut is at the film's own
// time, a tick early, in every run alike: each chunk holds the packets from the first at or after its
// start, whichever run made it, and two runs' chunks join with every packet once. The first packet is
// in a lead-in the run drops, or is the film's first, its priming, which then lasts only to the next
// (a tick, before an MP4's AAC; or as long as the film's sound starts after it); not at zero, where an
// MP4's AAC has its second packet, which the muxer would move.
func copiedSoundTimes() []string {
	return []string{"-bsf:a", `setts=ts=if(eq(N\,0)\,-1\,TS)`, "-avoid_negative_ts", "disabled"}
}

// keyframeTimes are where an encode puts its keyframes, relative to the run's start: at each of
// its chunks' starts, so its chunks cut where the copied picture's do, and, for a run that reads past
// its end (readEndMs), at its end too, so what it reads past is cut off into a raw file of its own.
func keyframeTimes(command nodeproto.MakeChunks) string {
	start := runStartMs(command)
	bounds := command.BoundsMs[:len(command.BoundsMs)-1]
	if isRetimed(command) {
		bounds = command.BoundsMs
	}
	times := make([]string, 0, len(bounds))
	for _, bound := range bounds {
		times = append(times, seconds(bound-start))
	}
	return strings.Join(times, ",")
}

// retimingReadPastMs is how far past its last chunk's end a re-timed run reads (isRetimed): FFmpeg's
// dts2pts filter gives each frame the time of the one at its place in the picture order, which for
// the frames before the run's end are among the frames after it; read to the end alone, it guesses
// their times (design/plans/audio-switch-and-retiming-2026-11-01.md §2.2). What is read past the end
// is cut off and dropped. A second, not the half the design chose: the run's end is a keyframe, and
// the pinned dts2pts times an H.264 IDR right only once it has read about 0.65 s past it (sixteen
// pictures at 24 a second); at half a second it guessed the keyframe three pictures early, the muxer
// did not cut there, and the run's last chunk held the next one's first half second as well
// (TestMake_APictureWhoseBFramesCarryNoDisplayTimesIsReTimedWhenMarked). HEVC needed less.
const retimingReadPastMs = 1000

// isRetimed is whether a run rebuilds its picture's display times from the bitstream's picture
// order (nodeproto.MakeChunks.NeedsPictureRetiming): the picture, copied or encoded, of a film whose
// probe said its B-frames carry none.
func isRetimed(command nodeproto.MakeChunks) bool {
	return command.NeedsPictureRetiming &&
		(command.Stream.Kind == nodeproto.StreamPicture || command.Stream.Kind == nodeproto.StreamQuality)
}

// readEndMs is where FFmpeg stops reading the film: the run's last chunk's end, or retimingReadPastMs
// past it for a re-timed run.
func readEndMs(command nodeproto.MakeChunks) int64 {
	end := command.BoundsMs[len(command.BoundsMs)-1]
	if isRetimed(command) {
		end += retimingReadPastMs
	}
	return end
}

// retimingFilter is FFmpeg's bitstream filter that rebuilds an H.264 or HEVC picture's display times
// from its slice headers' picture order count, without decoding it.
const retimingFilter = "dts2pts"

// encodeRetimingFilters are an encode's bitstream filters on its input, before it is decoded: the
// decoder reorders by the picture order itself, but times its frames by their packets'. The dts2pts
// filter shows the run's first keyframe as long after its decode time as the B-frames' reordering,
// which would time every picture of the encode that much late, its forced keyframes a picture or two
// after its chunks' bounds and the muxer's cuts missed; setts moves them all back, so the keyframe is
// shown at its decode time, the time the film's grid lists it at, as a copied chunk's keyframe is
// shown at its bound.
const encodeRetimingFilters = retimingFilter + ",setts=pts=PTS-STARTPTS+STARTDTS"

// Arguments is FFmpeg's command line for one run, after execx's hardening flags: the source film, the
// run's raw directory, where the segment muxer writes, and the node's encoder.
func Arguments(source execx.Film, rawPath string, command nodeproto.MakeChunks, encoder encode.Encoder) []string {
	arguments := []string{"-v", "error", "-nostats"}
	if command.Stream.Kind == nodeproto.StreamSoundCopied {
		arguments = append(arguments, "-copyts", "-start_at_zero")
	}
	if isSeeking(command) {
		arguments = append(arguments, "-ss", seconds(seekMs(command)))
	}
	arguments = append(arguments, "-to", seconds(readEndMs(command)))
	if isRetimed(command) && command.Stream.Kind == nodeproto.StreamQuality {
		// The picture's stream alone, by its index: a cover picture beside it is no H.264 or HEVC.
		arguments = append(arguments, "-bsf:"+strconv.Itoa(command.VideoStreamIndex), encodeRetimingFilters)
	}
	arguments = append(arguments, source.Input()...)
	switch command.Stream.Kind {
	case nodeproto.StreamPicture:
		// make_zero: a copy's times start at its first keyframe whichever side of it the seek is. One
		// seeking past it, as into MP4 or Matroska, starts below zero and is moved up to it anyway; one
		// seeking before it, as into MPEG-TS, would start as far after zero as the seek was before the
		// keyframe, and the list would time every raw file that much late (idiosyncrasies.md).
		arguments = append(arguments, "-map", "0:"+strconv.Itoa(command.VideoStreamIndex), "-c:v", "copy",
			"-avoid_negative_ts", "make_zero")
		if command.VideoCodec == "hevc" {
			arguments = append(arguments, "-tag:v", "hvc1")
		}
		if isRetimed(command) {
			arguments = append(arguments, "-bsf:v", retimingFilter)
		}
	case nodeproto.StreamQuality:
		arguments = append(arguments, "-map", "0:"+strconv.Itoa(command.VideoStreamIndex))
		arguments = append(arguments,
			encoder.QualityArguments(command.Stream.BitRateKbps, command.Stream.Height, keyframeTimes(command))...)
	case nodeproto.StreamSound:
		sound := command.Stream.Sound
		arguments = append(arguments, "-map", "0:a:"+strconv.Itoa(sound.AudioTrackIndex),
			"-filter:a", "atrim=start="+seconds(soundTrimMs(command))+",asetpts=PTS-STARTPTS,"+
				encode.MixFilter(*sound, command.Stream.IsBoosted),
			"-c:a", "aac", "-b:a", strconv.Itoa(command.Stream.SoundKbps)+"k", "-ac", "2")
	case nodeproto.StreamSoundCopied:
		arguments = append(arguments, "-map", "0:a:"+strconv.Itoa(command.Stream.Sound.AudioTrackIndex), "-c:a", "copy")
		arguments = append(arguments, copiedSoundTimes()...)
	}
	return append(arguments, "-sn", "-dn",
		"-f", "segment", "-segment_times", cutTimes(command),
		"-segment_start_number", strconv.Itoa(firstRawNumber(command)+rawNumberShift(command)),
		"-segment_list", filepath.Join(rawPath, rawListName), "-segment_list_type", "csv",
		"-segment_format", "mp4",
		// write_tmcd=0: the MP4 muxer adds a timecode track for a picture carrying a timecode, as an
		// MXF's or a camera's MOV does, and a raw file must hold the one track (D360).
		"-segment_format_options", "movflags="+rawMovFlags(command)+":use_editlist=0:write_tmcd=0",
		filepath.Join(rawPath, "%05d.mp4"),
	)
}

// rawMovFlags are the MP4 muxer's flags for a run's raw files: fragmented, the init first. A sound
// copied waits for its first packet before the init (delay_moov), which AC-3 and E-AC-3 need: the
// muxer cannot describe them before it has read one ("Cannot write moov atom before AC3 packets").
func rawMovFlags(command nodeproto.MakeChunks) string {
	flags := "+frag_keyframe+empty_moov+default_base_moof"
	if command.Stream.Kind == nodeproto.StreamSoundCopied {
		flags += "+delay_moov"
	}
	return flags
}
