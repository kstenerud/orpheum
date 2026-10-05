// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/prepare"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// valueOf is the argument after flag, or "" when it is not there.
func valueOf(arguments []string, flag string) string {
	if index := slices.Index(arguments, flag); index >= 0 && index+1 < len(arguments) {
		return arguments[index+1]
	}
	return ""
}

// run is a run of chunks 10 to 12, which start at 98.849, 108.859 and 112.613 s and end at
// 122.581 s, as 12 Marrow Larks's grid has them (slow-links.md §10).
func run(stream nodeproto.ChunkStream) nodeproto.MakeChunks {
	return nodeproto.MakeChunks{
		VideoStreamIndex: 0, VideoCodec: "h264", Stream: stream, StoreDir: "7/chunks-x/0123456789abcdef",
		FirstChunk: 10, BoundsMs: []int64{98_849, 108_859, 112_613, 122_581},
	}
}

// A run reads from its first chunk to its last's end, and cuts at each chunk after the first and
// at the end, so what FFmpeg writes past it is a file of its own; a picture's cuts come a little
// early, so a keyframe at the time is never missed (slow-links.md §10).
func TestArguments_ReadTheRunsChunksAndCutAtEachAndAtTheEnd(t *testing.T) {
	t.Parallel()
	arguments := Arguments("/films/a.mp4", "/prepared/7/chunks-x/0123456789abcdef/raw-1",
		run(nodeproto.ChunkStream{Kind: nodeproto.StreamPicture}), prepare.EncoderX264)
	if start, end := valueOf(arguments, "-ss"), valueOf(arguments, "-to"); start != "98.849" || end != "122.581" {
		t.Errorf("the run reads %s to %s, want 98.849 to 122.581", start, end)
	}
	if slices.Index(arguments, "-ss") > slices.Index(arguments, "-i") {
		t.Error("the start is an output option; it must seek the input")
	}
	if cuts := valueOf(arguments, "-segment_times"); cuts != "9.960,13.714,23.682" {
		t.Errorf("the cuts are %s, want 9.960,13.714,23.682: each chunk after the first, and the end, 50 ms early",
			cuts)
	}
	if first := valueOf(arguments, "-segment_start_number"); first != "10" {
		t.Errorf("the first raw file is %s, want 10", first)
	}
	if codec, tag := valueOf(arguments, "-c:v"), valueOf(arguments, "-tag:v"); codec != "copy" || tag != "" {
		t.Errorf("the picture is %s, tagged %q; want copied, untagged for H.264", codec, tag)
	}
	if options := valueOf(arguments, "-segment_format_options"); !strings.Contains(options, "use_editlist=0") ||
		!strings.Contains(options, "frag_keyframe") || !strings.Contains(options, "empty_moov") {
		t.Errorf("the raw files are written with %q; want fragmented MP4 without an edit list", options)
	}
}

// A copied HEVC picture is tagged hvc1, which Safari requires (cut-a §6).
func TestArguments_TagACopiedHEVCPicture(t *testing.T) {
	t.Parallel()
	hevc := run(nodeproto.ChunkStream{Kind: nodeproto.StreamPicture})
	hevc.VideoCodec = "hevc"
	if tag := valueOf(Arguments("/a", "/r", hevc, prepare.EncoderX264), "-tag:v"); tag != "hvc1" {
		t.Errorf("a copied HEVC picture is tagged %q, want hvc1", tag)
	}
}

// A quality is encoded at its rate and height, with keyframes at its chunks' starts, relative to
// the run's, and nowhere else, so its chunks cut where the copied picture's do (D247 §10).
func TestArguments_EncodeAQualityWithKeyframesAtItsChunksStarts(t *testing.T) {
	t.Parallel()
	arguments := Arguments("/a", "/r",
		run(nodeproto.ChunkStream{Kind: nodeproto.StreamQuality, BitRateKbps: 1200, Height: 480}), prepare.EncoderX264)
	if keyframes := valueOf(arguments, "-force_key_frames"); keyframes != "0.000,10.010,13.764" {
		t.Errorf("the keyframes are at %s, want 0.000,10.010,13.764", keyframes)
	}
	rate, peak, scale := valueOf(arguments, "-b:v"), valueOf(arguments, "-maxrate"), valueOf(arguments, "-vf")
	if rate != "1200k" || peak != "1800k" || scale != "scale=-2:480" {
		t.Errorf("the quality is %s, peaking at %s, %s; want 1200k, 1800k, scale=-2:480", rate, peak, scale)
	}
	if valueOf(arguments, "-sc_threshold") != "0" || valueOf(arguments, "-g") != "100000" {
		t.Error("x264 may put keyframes of its own at scene changes or a GOP's end")
	}
}

// A sound past the film's first chunk starts a chunk early, at its lead-in, whose raw file is the
// one before the run's first; its cuts are at the bounds, where the next frame starts (slow-links.md
// §10).
func TestArguments_StartASoundAChunkEarlyAndCutItAtTheBounds(t *testing.T) {
	t.Parallel()
	sound := run(nodeproto.ChunkStream{
		Kind: nodeproto.StreamSound, IsBoosted: true,
		Sound: &nodeproto.PrepareAudio{AudioTrackIndex: 1, ChannelCount: 6},
	})
	sound.SoundLeadInMs = 88_839
	arguments := Arguments("/a", "/r", sound, prepare.EncoderX264)
	if start := valueOf(arguments, "-ss"); start != "88.839" {
		t.Errorf("the sound starts at %s, want its lead-in, 88.839", start)
	}
	if first := valueOf(arguments, "-segment_start_number"); first != "9" {
		t.Errorf("the first raw file is %s, want 9, the lead-in's", first)
	}
	if cuts := valueOf(arguments, "-segment_times"); cuts != "10.010,20.020,23.774,33.742" {
		t.Errorf("the cuts are %s, want 10.010,20.020,23.774,33.742", cuts)
	}
	if mapped, mix := valueOf(arguments, "-map"), valueOf(arguments, "-filter:a"); mapped != "0:a:1" ||
		mix != prepare.MixFilter(*sound.Stream.Sound, true) {
		t.Errorf("the sound is %s mixed by %q; want track 1's boosted mix", mapped, mix)
	}
}
