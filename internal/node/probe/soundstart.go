// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// audioKind is ffprobe's codec_type of a sound.
const audioKind = "audio"

// leastStretchedFirstPacketMs is how long a sound's first packet must last, the packets after it a
// frame each, for the probe to take the length as a wrong time (D534 §1): longer than a second, which
// no sound's frame lasts.
const leastStretchedFirstPacketMs = 1_000

// longestSoundFrameMs is the longest a packet after a stretched first one may last and still be one
// frame of the sound: an AAC frame lasts 21 to 46 ms, AC-3's 32 ms, Opus's at most 120 ms, FLAC's
// at most 93 ms at 44.1 kHz.
const longestSoundFrameMs = 250

// leastSoundPacketsJudged is how many of a track's first packets, its first among them, the probe
// must have read to say its first alone lasts seconds: the packets after it are a frame each, not a
// few that happen to be.
const leastSoundPacketsJudged = 9

// soundPacketsRead is how many of a track's first packets ffprobe lists of it alone, when the file's
// first packetsRead hold its first, lasting seconds, and too few after it to judge by (file 18306's
// first 200 packets hold its sound's first alone, the picture's 199 filling the 31.9 s it lasts).
const soundPacketsRead = 25

// mostSoundsListedAlone bounds how many of a file's tracks the probe lists the first packets of
// alone, each an ffprobe run: a file whose every one of hundreds of tracks starts with a long packet
// is hostile or broken, and its tracks past these are taken as they are timed.
const mostSoundsListedAlone = 16

// soundArguments are ffprobe's, for film, read as its Input names it: the times and lengths of the
// first soundPacketsRead packets of its stream streamIndex alone.
func soundArguments(film execx.Film, streamIndex int) []string {
	return append([]string{
		"-v", "error",
		"-print_format", "json",
		"-select_streams", strconv.Itoa(streamIndex),
		"-show_entries", "packet=stream_index,pts,dts,duration",
		"-read_intervals", "%+#" + strconv.Itoa(soundPacketsRead),
	}, film.Input()...)
}

// withSoundsListedAlone is record with each sound whose first packet, among the file's first ffprobe
// listed, lasts seconds, with too few of its packets after it listed to judge by, judged by its first
// packets listed alone (soundArguments).
func (prober *Prober) withSoundsListedAlone(
	ctx context.Context, film execx.Film, raw rawOutput, record nodeproto.Probe,
) (nodeproto.Probe, error) {
	listed := 0
	for position, stream := range raw.Streams {
		if listed == mostSoundsListedAlone || !stream.hasALongFirstPacketUnjudged(raw.Packets) {
			continue
		}
		listed++
		arguments := soundArguments(film, stream.Index)
		output, err := prober.runner.RunOnFilm(ctx, execx.FFprobe, film, arguments, maxOutputBytes)
		if err != nil {
			return nodeproto.Probe{}, fmt.Errorf("probe %s, the first packets of its stream %d: %w",
				film.Path, stream.Index, err)
		}
		alone, err := parseRaw(output)
		if err != nil {
			return nodeproto.Probe{}, err
		}
		record.Streams[position].Audio.FirstPacketStretchMs = stream.firstPacketStretchMs(alone.Packets)
	}
	return record, nil
}

// packetDurations are how long each of the stream's packets among packets lasts, in its time base, in
// order, at most soundPacketsRead of them; false when one says none.
func (stream rawStream) packetDurations(packets []rawPacket) ([]int64, bool) {
	var durations []int64
	for _, packet := range packets {
		if packet.StreamIndex != stream.Index {
			continue
		}
		if packet.Duration == nil {
			return nil, false
		}
		durations = append(durations, *packet.Duration)
		if len(durations) == soundPacketsRead {
			break
		}
	}
	return durations, true
}

// hasALongFirstPacketUnjudged is whether the stream is a sound whose first packet among packets lasts
// more than leastStretchedFirstPacketMs, with too few of its packets after it among them to judge
// whether its first alone lasts seconds (firstPacketStretchMs).
func (stream rawStream) hasALongFirstPacketUnjudged(packets []rawPacket) bool {
	durations, isTimed := stream.packetDurations(packets)
	if stream.CodecType != audioKind || !isTimed || len(durations) == 0 || len(durations) >= leastSoundPacketsJudged {
		return false
	}
	first, isKnown := stream.ticksAsMs(durations[0])
	return isKnown && first > leastStretchedFirstPacketMs
}

// firstPacketStretchMs is, for a sound whose first packet among packets alone lasts seconds, more than
// leastStretchedFirstPacketMs, while each of the packets after it, at least leastSoundPacketsJudged
// with it, lasts a frame, at most longestSoundFrameMs, how much longer its first lasts than its second:
// the stretch D534 §1 takes as a wrong time (nodeproto.ProbeAudio.FirstPacketStretchMs). Zero for
// every other stream.
func (stream rawStream) firstPacketStretchMs(packets []rawPacket) int64 {
	durations, isTimed := stream.packetDurations(packets)
	if stream.CodecType != audioKind || !isTimed || len(durations) < leastSoundPacketsJudged {
		return 0
	}
	if first, isKnown := stream.ticksAsMs(durations[0]); !isKnown || first <= leastStretchedFirstPacketMs {
		return 0
	}
	for _, duration := range durations[1:] {
		if frame, isKnown := stream.ticksAsMs(duration); !isKnown || duration <= 0 || frame > longestSoundFrameMs {
			return 0
		}
	}
	stretch, _ := stream.ticksAsMs(durations[0] - durations[1])
	return stretch
}

// ticksAsMs is ticks of the stream's time base ("1/44100") in milliseconds, rounded; false for a time
// base that is none or nonsense, or a length out of bounds.
func (stream rawStream) ticksAsMs(ticks int64) (int64, bool) {
	numerator, denominator, isRatio := strings.Cut(stream.TimeBase, "/")
	over, overErr := strconv.ParseInt(numerator, 10, 64)
	under, underErr := strconv.ParseInt(denominator, 10, 64)
	if !isRatio || overErr != nil || underErr != nil || over <= 0 || under <= 0 || ticks < 0 {
		return 0, false
	}
	ms := float64(ticks) * float64(over) * 1000 / float64(under)
	if math.IsNaN(ms) || ms > math.MaxInt64/2 {
		return 0, false
	}
	return int64(math.Round(ms)), true
}
