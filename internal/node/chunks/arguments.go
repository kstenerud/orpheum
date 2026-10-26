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
	if command.Stream.Kind == nodeproto.StreamSound && command.SoundLeadInMs > 0 {
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

// seekMs is where FFmpeg is told to seek: a copied picture's seek, past its first keyframe, so
// FFmpeg lands on it (nodeproto.MakeChunks.PictureSeekMs); a sound's soundSeekEarlyMs before its
// run's start, which its filter trims to (soundTrimMs); otherwise the run's start, which a decode
// trims to exactly.
func seekMs(command nodeproto.MakeChunks) int64 {
	switch command.Stream.Kind {
	case nodeproto.StreamPicture:
		return command.PictureSeekMs
	case nodeproto.StreamSound:
		return max(0, runStartMs(command)-soundSeekEarlyMs)
	case nodeproto.StreamQuality:
		return runStartMs(command)
	}
	return runStartMs(command)
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

// cutTimes are where the muxer cuts, relative to the run's start: every chunk boundary after it,
// the end of the last chunk among them, so what FFmpeg writes past it lands in a raw file that is
// dropped. A picture is cut a little early, so its keyframe is not missed; a sound is cut at the
// first frame at or after the time.
func cutTimes(command nodeproto.MakeChunks) string {
	start := runStartMs(command)
	early := int64(cutEarlyMs)
	if command.Stream.Kind == nodeproto.StreamSound {
		early = 0
	}
	var cuts []string
	for _, bound := range command.BoundsMs {
		if bound > start {
			cuts = append(cuts, seconds(bound-start-early))
		}
	}
	return strings.Join(cuts, ",")
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
	arguments := []string{"-v", "error", "-nostats", "-ss", seconds(seekMs(command)), "-to", seconds(end)}
	arguments = append(arguments, source.Input()...)
	switch command.Stream.Kind {
	case nodeproto.StreamPicture:
		arguments = append(arguments, "-map", "0:"+strconv.Itoa(command.VideoStreamIndex), "-c:v", "copy")
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
	}
	return append(arguments, "-sn", "-dn",
		"-f", "segment", "-segment_times", cutTimes(command),
		"-segment_start_number", strconv.Itoa(firstRawNumber(command)),
		"-segment_list", filepath.Join(rawPath, rawListName), "-segment_list_type", "csv",
		"-segment_format", "mp4",
		"-segment_format_options", "movflags=+frag_keyframe+empty_moov+default_base_moof:use_editlist=0",
		filepath.Join(rawPath, "%05d.mp4"),
	)
}
