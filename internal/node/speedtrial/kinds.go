// SPDX-License-Identifier: AGPL-3.0-or-later

package speedtrial

import (
	"fmt"
	"slices"
	"strconv"
)

// A clip is twelve seconds of a generated picture at 24 a second, two six-second chunks, with a
// keyframe every two seconds, as a film's are. Its trial forces a keyframe where each chunk starts, as
// a run of chunks does.
const (
	clipSeconds      = 12
	clipRate         = 24
	clipKeyframesAt  = "0,6"
	clipKeyframeGap  = 2 * clipRate
	clipFormatSuffix = ".mkv"
)

// The qualities each clip is made into: the top of its height's, the twin's 8 Mb/s at the clip's own
// height (the core's decide.TwinKbps, at most H.264's profile rate), and the lowest of the starting
// ladder, 400k@360p, the one encode.MeasureSpeed measures (D457).
const (
	topKbps      = 8000
	lowestKbps   = 400
	lowestHeight = 360
)

// Kind is one kind of film a node is tried on: its picture's codec, by the name FFmpeg's probe gives
// it, bit depth and height, and how FFmpeg makes a clip of it.
type Kind struct {
	Codec    string
	BitDepth int
	Height   int
	// Encoder is the FFmpeg encoder the clip is made with: a kind this FFmpeg has none of is not tried.
	Encoder string
	// options are the encoder's options for the clip, after its codec: its pixel format, its rate.
	options []string
}

// Kinds are what a node is tried on, in the order it tries them (start-quality-by-node-speed.md §1):
// H.264 and HEVC at 8 and 10 bits, VP9, AV1, MPEG-2 and MPEG-4 at 1080p, and HEVC Main 10 at 2160p,
// each at a usual film's rate for its codec and height. AV1 is 10-bit, as most AV1 films are.
func Kinds() []Kind {
	const hevc, x265Name, yuv420p, yuv420p10 = "hevc", "libx265", "yuv420p", "yuv420p10le"
	const preset, pixelFormatOption, bitrate = "-preset", "-pix_fmt", "-b:v"
	x264 := func(pixelFormat, profile string) []string {
		return []string{preset, "veryfast", pixelFormatOption, pixelFormat, "-profile:v", profile, bitrate, "10000k"}
	}
	x265 := func(pixelFormat, profile, kbps string) []string {
		return []string{
			preset, "superfast", pixelFormatOption, pixelFormat, "-profile:v", profile, bitrate, kbps,
			"-x265-params", "log-level=error",
		}
	}
	return []Kind{
		{Codec: "h264", BitDepth: 8, Height: 1080, Encoder: "libx264", options: x264(yuv420p, "high")},
		{Codec: "h264", BitDepth: 10, Height: 1080, Encoder: "libx264", options: x264(yuv420p10, "high10")},
		{Codec: hevc, BitDepth: 8, Height: 1080, Encoder: x265Name, options: x265(yuv420p, "main", "6000k")},
		{
			Codec: hevc, BitDepth: 10, Height: 1080, Encoder: x265Name,
			options: x265(yuv420p10, "main10", "6000k"),
		},
		{Codec: "vp9", BitDepth: 8, Height: 1080, Encoder: "libvpx-vp9", options: []string{
			"-deadline", "realtime", "-cpu-used", "8", "-row-mt", "1", pixelFormatOption, yuv420p, bitrate, "6000k",
		}},
		{Codec: "av1", BitDepth: 10, Height: 1080, Encoder: "libsvtav1", options: []string{
			preset, "12", pixelFormatOption, yuv420p10, bitrate, "4000k",
		}},
		{Codec: "mpeg2video", BitDepth: 8, Height: 1080, Encoder: "mpeg2video", options: []string{
			pixelFormatOption, yuv420p, bitrate, "15000k", "-maxrate", "20000k", "-bufsize", "9781k",
		}},
		{Codec: "mpeg4", BitDepth: 8, Height: 1080, Encoder: "mpeg4", options: []string{
			pixelFormatOption, yuv420p, bitrate, "8000k",
		}},
		{
			Codec: hevc, BitDepth: 10, Height: 2160, Encoder: x265Name,
			options: x265(yuv420p10, "main10", "20000k"),
		},
	}
}

// quality is one quality a clip is made into.
type quality struct{ kbps, height int }

// qualities are the qualities kind's clip is made into: the top of its height's, then the lowest.
func (kind Kind) qualities() []quality {
	return []quality{{kbps: topKbps, height: kind.Height}, {kbps: lowestKbps, height: lowestHeight}}
}

// name is the kind's clip's file name, which says what it holds.
func (kind Kind) name() string {
	return fmt.Sprintf("%s-%dbit-%dp%s", kind.Codec, kind.BitDepth, kind.Height, clipFormatSuffix)
}

// source is the clip's generated picture: FFmpeg's test pattern at the kind's size, 16:9, with noise
// that changes every picture, so it is as hard to compress and to decode as a film's, where the bare
// pattern is all but still.
func (kind Kind) source() string {
	return "testsrc2=size=" + strconv.Itoa(kind.Height*16/9) + "x" + strconv.Itoa(kind.Height) +
		":rate=" + strconv.Itoa(clipRate) + ":duration=" + strconv.Itoa(clipSeconds) + ",noise=alls=12:allf=t+u"
}

// clipArguments make the kind's clip at output, a Matroska file, after execx's hardening flags.
func (kind Kind) clipArguments(output string) []string {
	return slices.Concat(
		[]string{"-v", "error", "-nostats", "-f", "lavfi", "-i", kind.source(), "-c:v", kind.Encoder},
		kind.options, []string{"-g", strconv.Itoa(clipKeyframeGap), "-an", "-f", "matroska", "-y", output},
	)
}
