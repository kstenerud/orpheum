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
// keyframe to keyframe: an HEVC keyframe of the owner's film is shown 83 to 250 ms after it is decoded, so
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
// picture past the film's first chunk, the chunk before it, whose chunk is dropped (its LeadInMs,
// the film's start for a chunk 0 starting there; IsLedIn); or, for a sound copied whose film's first
// chunk starts well after the film does (isLedInFromFilmStart), the film's start.
func runStartMs(command nodeproto.MakeChunks) int64 {
	if command.IsLedIn {
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

// seekMs is where, in the film's time, FFmpeg is told to seek (fileSeekMs): a copied picture's
// cutEarlyMs before its run's first keyframe (copiedPictureStart); a sound's soundSeekEarlyMs before
// its run's start, or an interleaved one's interleavedSoundSeekEarlyMs, which its filter trims to
// (soundTrimMs); otherwise the run's start, which a decode trims to exactly, and from which a sound
// copied copies what FFmpeg lands on, a little before it.
func seekMs(command nodeproto.MakeChunks) int64 {
	switch command.Stream.Kind {
	case nodeproto.StreamPicture:
		return max(0, runStartMs(command)-cutEarlyMs)
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

// copiedPictureStart starts a copied picture's run at its first keyframe whatever the demuxer's seek
// lands on (D500 §1): the seek is a little before the keyframe (seekMs), and the copy drops every
// packet until a keyframe shown at or after the seek (-copypriorss 0), so it starts at the run's
// keyframe, never at the one before it or the one after. A seek lands by the file's index, and FFmpeg
// moves one into Matroska, AVI or MPEG-TS 130 ms earlier, so a copy seeking past its keyframe landed
// on it only where the index is true: MPEG-TS lands on any frame, and an AVI whose index marks every
// frame a keyframe on the frame at the seek, from which a copy, starting at the first packet its
// parser finds a keyframe, began at the next keyframe; every run of file 17099 began a keyframe late
// and was refused at its first raw file (staging, 2026-11-16; idiosyncrasies.md). The copy's times
// count from the seek, cutEarlyMs before the keyframe, so a keyframe at 10.009999 s is kept for one
// at 10.010 s; they start at zero at the keyframe (-avoid_negative_ts make_zero), so the muxer's cuts
// and list are timed from it as before. A run reading from the film's start, unsought, copies from
// its first keyframe.
func copiedPictureStart(command nodeproto.MakeChunks) []string {
	if !isSeeking(command) {
		return nil
	}
	return []string{"-copypriorss:v", "0"}
}

// isSeeking is whether FFmpeg is told to seek at all: not to the film's start, which a run reads from
// unsought. FFmpeg moves a seek into a film with B-frames 3/23 s earlier, before the start, and an
// AVI seeking there lands past its first keyframe, losing the film's first packets: the
// first 21 frames of a sound interleaved ahead of its picture, which put a sound, timed by its
// samples, half a second early for all its run (the owner's series, staging, 2026-11-01), and the picture's
// first keyframe; and RealMedia loses its first super-block, as any seek loses the one it lands in
// (idiosyncrasies.md).
func isSeeking(command nodeproto.MakeChunks) bool {
	return fileSeekMs(command) > 0
}

// soundStretchMs is, for a run of a sound made whose track's first packet alone lasts seconds, how
// much longer it lasts than a frame (nodeproto.MakeChunks.SoundFirstPacketStretchMs): a wrong time,
// the samples after it belonging straight after it, with the picture (D534 §1). The run reads its
// sound that much later in the file than the film's time: the film's time t is the file's t plus the
// stretch, past the first packet, whose samples a seek to the stretch drops, as the gap fill
// (soundGapFill) puts a frame of silence in their place. Zero for every other run.
func soundStretchMs(command nodeproto.MakeChunks) int64 {
	if command.Stream.Kind != nodeproto.StreamSound {
		return 0
	}
	return max(0, command.SoundFirstPacketStretchMs)
}

// fileSeekMs is where FFmpeg is told to seek in the file: seekMs, the film's time, a sound read
// soundStretchMs later, so a run of a sound whose first packet is stretched always seeks, from the
// film's start to the stretch's end.
func fileSeekMs(command nodeproto.MakeChunks) int64 {
	return seekMs(command) + soundStretchMs(command)
}

// fileReadEndMs is where FFmpeg stops reading the file: readEndMs, the film's time, a sound read
// soundStretchMs later.
func fileReadEndMs(command nodeproto.MakeChunks) int64 {
	return readEndMs(command) + soundStretchMs(command)
}

// soundGapFill fills a gap in a film's sound with silence, so a sound made is cut on its grid
// however the film's packets are timed (D533 §1): the encode counts samples, not timestamps, and a
// first AAC packet lasting 31.9 s, every other 23 ms, gave its chunk 0 46 ms and started chunk 1 at
// 31.9 s, refused at its bounds (file 18306, staging, 2026-11-22). It puts the decode's first sample
// at zero, where the seek (or the film's start, unsought) is, so it comes before the trim, whose
// start counts from there: after it, it would fill the trimmed time with silence.
const soundGapFill = "aresample=async=1:first_pts=0"

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

// keyframeTimes are where an encode puts its keyframes, relative to the run's start, at its first
// picture, where the encode's times start and the muxer counts its cuts from: at each of its chunks'
// starts, so its chunks cut where the copied picture's do, and, for a run that reads past
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

// StretchOf is the stretch of its film a run reads: from its seek to where it stops reading, to which
// a long MP4 index is cut down (execx.OpenedFilm.Stretch, D512).
func (*Maker) StretchOf(command nodeproto.MakeChunks) execx.Stretch {
	return execx.Stretch{FromMs: fileSeekMs(command), ToMs: fileReadEndMs(command)}
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

// encodeTimeBase keeps an encode in its source's time base, so its MP4 track counts in the copied
// picture's timescale: the MP4 muxer makes a track's timescale from its stream's time base, 1/16000
// for Matroska's 1/1000 (doubled until past 10,000), 1/90000 for MPEG-TS's, and the encoder's
// default, 1/its frame rate, made every encoded rung's 1/24000 whatever the source; and hls.js, given
// a chunk at a timescale other than the one it holds, takes a new time offset for the whole stream,
// and every switch between the copy and an encoded rung left a hole of 0.08 to 0.46 s (film 1504
// on the Fire TV, staging, 2026-11-13; design/plans/navigation-and-widgets-answers-2026-10-27.md,
// item 5). Each picture is passed once at its own time (vfr): the segment muxer declares no variable
// frame rate, so FFmpeg would hold a constant one, counted in the time base's ticks, and at 1/90000
// encoded each picture 3,753 times (idiosyncrasies.md).
func encodeTimeBase() []string {
	return []string{"-enc_time_base:v", "demux", "-fps_mode:v", "vfr"}
}

// encodeCutTimes keeps the segment muxer's clock an encode's own, its pictures' times as the encoder
// was given them, where its keyframes are forced: an encoder with B-frames, x264 and h264_vaapi
// alike, decodes its first pictures before they are shown and gives the first a decode time before
// zero, as far before it as the next one or two pictures are after it, and FFmpeg moves every packet
// later by that much before the muxer sees it (avoid_negative_ts), so the muxer cut and listed every
// raw file that much late. A picture or two at the film's rate passed the list's tolerance, but a film
// whose first picture is followed by a gap did not: an RV20 film's second picture a third of a second
// after its first moved a run from its start 284 ms later under VAAPI and 347 ms under x264, its
// chunk 0 listed ending at 2,307 ms and never made (file 17137, staging, 2026-11-16;
// idiosyncrasies.md). Unmoved, the muxer cuts and lists at the forced keyframes; the MP4 muxer,
// which writes no edit list here, still starts each raw file's decode times at zero itself, as
// before.
func encodeCutTimes() []string {
	return []string{"-avoid_negative_ts", "disabled"}
}

// encodeStartDecodeTimes are the bitstream filter on an encode's output that decodes a run's first
// picture at its shown time when the encoder put it more than a tenth of a second before: hls.js
// places a stream by its first chunk's first decode time, the run's chunk 0's at a film's start, and
// shows its first picture as long after the start as its composition offset, a hole hls.js steps over
// only once playing and stalled, never from a paused start, beyond its maxBufferHole, 0.1 s. An
// encoder with B-frames decodes the first pictures before they are shown by as long as the next one
// or two are after the first, and a film whose first picture is followed by a gap had its first shown
// 0.532 s after its decode time, its sound 0.511 s after the start, and never started playing until
// skipped forward (file 17477, staging, 2026-11-18; idiosyncrasies.md). A film's even pictures
// reorder a picture or two, under the hole (83 ms at 24 a second), and are left as encoded.
//
// Each packet's decode time is raised, never lowered, to no less than the first picture's shown time
// sixteen ticks early, a tick later for each packet: H.264 reorders at most sixteen pictures, so every
// packet is still shown at or after its decode time, the decode times still rise, and from the packet
// the encoder decodes at the first picture's shown time on none moves. The shown times, and so the
// muxer's cuts and its list, are the encoder's.
func encodeStartDecodeTimes() []string {
	return []string{"-bsf:v", `setts=pts=PTS:dts=if(gt(STARTPTS-STARTDTS\,0.1/TB)\,max(DTS\,STARTPTS-16+N)\,DTS)`}
}

// Arguments is FFmpeg's command line for one run, after execx's hardening flags: the source film, the
// run's raw directory, where the segment muxer writes, and the node's encoder.
func Arguments(source execx.Film, rawPath string, command nodeproto.MakeChunks, encoder encode.Encoder) []string {
	arguments := []string{"-v", "error", "-nostats"}
	if command.Stream.Kind == nodeproto.StreamSoundCopied {
		arguments = append(arguments, "-copyts", "-start_at_zero")
	}
	if isSeeking(command) {
		arguments = append(arguments, "-ss", seconds(fileSeekMs(command)))
	}
	arguments = append(arguments, "-to", seconds(fileReadEndMs(command)))
	if isRetimed(command) && command.Stream.Kind == nodeproto.StreamQuality {
		// The picture's stream alone, by its index: a cover picture beside it is no H.264 or HEVC.
		arguments = append(arguments, "-bsf:"+strconv.Itoa(command.VideoStreamIndex), encodeRetimingFilters)
	}
	arguments = append(arguments, source.Input()...)
	switch command.Stream.Kind {
	case nodeproto.StreamPicture:
		// make_zero: a copy's times start at its first keyframe, not as far after zero as the seek was
		// before it, which would have the list time every raw file that much late (idiosyncrasies.md).
		arguments = append(arguments, "-map", "0:"+strconv.Itoa(command.VideoStreamIndex), "-c:v", "copy",
			"-avoid_negative_ts", "make_zero")
		arguments = append(arguments, copiedPictureStart(command)...)
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
		arguments = append(arguments, encodeTimeBase()...)
		arguments = append(arguments, encodeCutTimes()...)
		arguments = append(arguments, encodeStartDecodeTimes()...)
	case nodeproto.StreamSound:
		sound := command.Stream.Sound
		arguments = append(arguments, "-map", "0:a:"+strconv.Itoa(sound.AudioTrackIndex),
			"-filter:a", soundGapFill+",atrim=start="+seconds(soundTrimMs(command))+",asetpts=PTS-STARTPTS,"+
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
