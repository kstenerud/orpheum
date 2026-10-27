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

// rawListName is the segment muxer's list of the raw files it has finished, one line each, in the
// run's raw directory.
const rawListName = "list.csv"

// runStartMs is where a run starts reading: its first chunk's start, or, for sound past the
// film's first chunk, the chunk before it, whose chunk is dropped.
func runStartMs(command nodeproto.MakeChunks) int64 {
	if command.Stream.Kind.IsSound() && command.SoundLeadInMs > 0 {
		return command.SoundLeadInMs
	}
	return command.BoundsMs[0]
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

// isSeeking is whether FFmpeg is told to seek at all: always, but for an interleaved sound decoded
// from the film's start, which is read from it unsought, as a seek to the start loses the first
// super-block as any seek loses the one it lands in (idiosyncrasies.md). A sound copied is never
// interleaved RealAudio, which no browser plays (D368), and seeks as it always does whatever it says.
func isSeeking(command nodeproto.MakeChunks) bool {
	return command.Stream.Kind != nodeproto.StreamSound || !command.IsSoundInterleaved || seekMs(command) > 0
}

// soundTrimMs is how much a sound's decode drops before its run's start: what it was decoded early.
func soundTrimMs(command nodeproto.MakeChunks) int64 {
	return runStartMs(command) - seekMs(command)
}

// firstRawNumber is the number of the run's first raw file: its first chunk's, or one before it
// for a sound's lead-in.
func firstRawNumber(command nodeproto.MakeChunks) int {
	if runStartMs(command) != command.BoundsMs[0] {
		return command.FirstChunk - 1
	}
	return command.FirstChunk
}

// seconds is milliseconds as FFmpeg's seconds.
func seconds(ms int64) string { return strconv.FormatFloat(float64(ms)/1000, 'f', 3, 64) }

// cutTimes are where the muxer cuts: every chunk boundary after the run's start, the end of the
// last chunk among them, so what FFmpeg writes past it lands in a raw file that is dropped. The
// muxer times its cuts from the run's first packet: relative to the run's start, its first packet's
// time, but for a sound copied, whose first packet is put a tick before the film's start
// (copiedSoundTimes), so it is cut at the film's own times, a tick early alike in every run. A
// picture is cut a little early, so its keyframe is not missed;
// a sound is cut at the first frame at or after the time.
func cutTimes(command nodeproto.MakeChunks) string {
	origin := runStartMs(command)
	if command.Stream.Kind == nodeproto.StreamSoundCopied {
		origin = 0
	}
	early := int64(cutEarlyMs)
	if command.Stream.Kind.IsSound() {
		early = 0
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
// its chunks' starts, so its chunks cut where the copied picture's do.
func keyframeTimes(command nodeproto.MakeChunks) string {
	start := runStartMs(command)
	times := make([]string, 0, len(command.BoundsMs)-1)
	for _, bound := range command.BoundsMs[:len(command.BoundsMs)-1] {
		times = append(times, seconds(bound-start))
	}
	return strings.Join(times, ",")
}

// Arguments is FFmpeg's command line for one run, after execx's hardening flags: the source film, the
// run's raw directory, where the segment muxer writes, and the node's encoder.
func Arguments(source execx.Film, rawPath string, command nodeproto.MakeChunks, encoder encode.Encoder) []string {
	end := command.BoundsMs[len(command.BoundsMs)-1]
	arguments := []string{"-v", "error", "-nostats"}
	if command.Stream.Kind == nodeproto.StreamSoundCopied {
		arguments = append(arguments, "-copyts", "-start_at_zero")
	}
	if isSeeking(command) {
		arguments = append(arguments, "-ss", seconds(seekMs(command)))
	}
	arguments = append(arguments, "-to", seconds(end))
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
		"-segment_start_number", strconv.Itoa(firstRawNumber(command)),
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
