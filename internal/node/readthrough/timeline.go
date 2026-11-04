// SPDX-License-Identifier: AGPL-3.0-or-later

package readthrough

import (
	"bytes"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/node/container"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// timelineOutput is the output that reads the picture's timeline: its packets copied, never
// decoded, into FFmpeg's framecrc muxer on stdout, which writes one line for each packet with its
// time, its size and, when they are not a keyframe's alone, its flags. The stream is named by its
// index among all the file's streams, as ffprobe numbers them.
func timelineOutput(videoStreamIndex int) []string {
	return []string{"-map", "0:" + strconv.Itoa(videoStreamIndex), "-c", "copy", "-f", "framecrc", "pipe:1"}
}

// The packet flags framecrc writes as "F=0x..": a keyframe's, and those that make a packet one no
// player shows as it is.
const (
	packetFlagKey     = 0x1
	packetFlagCorrupt = 0x2
	packetFlagDiscard = 0x4
)

// longestTimeline is the latest a packet may be in a film: far past any film, so only a broken
// timestamp reaches it, which would otherwise ask for an entry for every second up to it.
const longestTimeline = 7 * 24 * time.Hour

// packetLines reads framecrc's lines as FFmpeg writes them, keeping only the keyframes' times and
// the bytes in each second, so a film's millions of packets are never held. It is an io.Writer that
// never fails: what it cannot read it says when asked for the timeline.
type packetLines struct {
	// unended is the start of a line whose end has not come yet.
	unended []byte
	// timeBase is the stream's time base, from framecrc's "#tb 0: 1/24000" header: a packet's time
	// times it is seconds. Zero until the header comes.
	timeBaseNumerator, timeBaseDenominator int64
	keyframesMs                            []int64
	// hasKeyframeBeforeStart is a keyframe shown before the film's start, which a container's edit
	// flags discarded: the film's first pictures are decoded from it (startingKeyframes).
	hasKeyframeBeforeStart bool
	bytesInEachSecond      []int64
	// unreadable is the first line it could not read, or the first packet past longestTimeline.
	unreadable error
}

// Write reads every whole line in data, and keeps the rest for the next write.
func (lines *packetLines) Write(data []byte) (int, error) {
	pending := slices.Concat(lines.unended, data)
	for {
		end := bytes.IndexByte(pending, '\n')
		if end < 0 {
			break
		}
		lines.read(string(pending[:end]))
		pending = pending[end+1:]
	}
	lines.unended = append([]byte(nil), pending...)
	return len(data), nil
}

// read reads one line: the time base's header, another header, or a packet.
func (lines *packetLines) read(line string) {
	line = strings.TrimSpace(line)
	switch {
	case line == "" || lines.unreadable != nil:
	case strings.HasPrefix(line, "#tb 0:"):
		numerator, denominator, isRead := strings.Cut(strings.TrimSpace(strings.TrimPrefix(line, "#tb 0:")), "/")
		lines.timeBaseNumerator, _ = strconv.ParseInt(numerator, 10, 64)     //nolint:errcheck // zero is checked below
		lines.timeBaseDenominator, _ = strconv.ParseInt(denominator, 10, 64) //nolint:errcheck // zero is checked below
		if !isRead || lines.timeBaseNumerator <= 0 || lines.timeBaseDenominator <= 0 {
			lines.unreadable = fmt.Errorf("FFmpeg wrote a time base this cannot read: %q", line)
		}
	case strings.HasPrefix(line, "#"):
	default:
		lines.unreadable = lines.readPacket(line)
	}
}

// readPacket reads one packet's line, "0, dts, pts, duration, size, checksum[, F=0x..][, side
// data]": its bytes count in the second it is shown in, and a keyframe's time is kept. A packet
// with no time (framecrc writes the smallest int64) counts nowhere.
func (lines *packetLines) readPacket(line string) error {
	fields := strings.Split(line, ",")
	if len(fields) < 6 || strings.TrimSpace(fields[0]) != "0" {
		return fmt.Errorf("FFmpeg wrote a packet this cannot read: %q", line)
	}
	if lines.timeBaseDenominator == 0 {
		return fmt.Errorf("FFmpeg wrote a packet before its time base: %q", line)
	}
	pts, ptsErr := strconv.ParseInt(strings.TrimSpace(fields[2]), 10, 64)
	size, sizeErr := strconv.ParseInt(strings.TrimSpace(fields[4]), 10, 64)
	if ptsErr != nil || sizeErr != nil {
		return fmt.Errorf("FFmpeg wrote a packet this cannot read: %q", line)
	}
	flags := int64(packetFlagKey)
	for _, field := range fields[6:] {
		if hexFlags, isFlags := strings.CutPrefix(strings.TrimSpace(field), "F=0x"); isFlags {
			parsed, err := strconv.ParseInt(hexFlags, 16, 64)
			if err != nil {
				return fmt.Errorf("FFmpeg wrote a packet's flags this cannot read: %q", line)
			}
			flags = parsed
		}
	}
	if pts == math.MinInt64 {
		return nil
	}
	shownAtMs := int64(math.Round(float64(pts) * float64(lines.timeBaseNumerator) * 1000 /
		float64(lines.timeBaseDenominator)))
	if shownAtMs >= longestTimeline.Milliseconds() {
		return fmt.Errorf("FFmpeg wrote a packet at %d ms, past any film's end", shownAtMs)
	}
	second := max(shownAtMs/1000, 0)
	for int64(len(lines.bytesInEachSecond)) <= second {
		lines.bytesInEachSecond = append(lines.bytesInEachSecond, 0)
	}
	lines.bytesInEachSecond[second] += size
	lines.keepKeyframe(flags, shownAtMs)
	return nil
}

// keepKeyframe keeps a whole keyframe's time. One flagged discarded is left out, but one before
// the film's start is noted, to count at the start when no other is (container.StartingKeyframes).
func (lines *packetLines) keepKeyframe(flags, shownAtMs int64) {
	switch {
	case flags&packetFlagKey == 0 || flags&packetFlagCorrupt != 0:
	case flags&packetFlagDiscard == 0:
		lines.keyframesMs = append(lines.keyframesMs, shownAtMs)
	case shownAtMs < 0:
		lines.hasKeyframeBeforeStart = true
	}
}

// timeline is what the lines said, once FFmpeg has ended: the keyframes in time order, each once,
// and the bytes in each second. A picture with no keyframe has an empty timeline of keyframes,
// which can be cut nowhere.
func (lines *packetLines) timeline() (nodeproto.VideoTimeline, error) {
	if len(lines.unended) > 0 {
		lines.read(string(lines.unended))
		lines.unended = nil
	}
	if lines.unreadable != nil {
		return nodeproto.VideoTimeline{}, lines.unreadable
	}
	if lines.timeBaseDenominator == 0 {
		return nodeproto.VideoTimeline{}, fmt.Errorf("FFmpeg wrote no time base for the picture")
	}
	keyframesMs := container.StartingKeyframes(slices.Compact(slices.Sorted(slices.Values(lines.keyframesMs))),
		lines.hasKeyframeBeforeStart)
	// Never nil, so the wire says an empty list rather than none.
	return nodeproto.VideoTimeline{
		KeyframesMs:       append([]int64{}, keyframesMs...),
		BytesInEachSecond: append([]int64{}, lines.bytesInEachSecond...),
	}, nil
}
