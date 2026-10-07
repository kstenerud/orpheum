// SPDX-License-Identifier: GPL-3.0-or-later

package prepare_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/prepare"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// after is what follows flag in arguments, or "" when flag is absent.
func after(arguments []string, flag string) string {
	if at := slices.Index(arguments, flag); at >= 0 && at+1 < len(arguments) {
		return arguments[at+1]
	}
	return ""
}

func TestArguments_CarryEveryTrackWithABoostedMixWhereThereIsACentre(t *testing.T) {
	t.Parallel()
	command := nodeproto.Prepare{
		Profile: nodeproto.ProfileRemux, VideoCodec: "h264", RenditionDir: "7/remux/0123456789abcdef",
		AudioTracks: []nodeproto.PrepareAudio{
			{AudioTrackIndex: 0, Language: "en", ChannelCount: 6},
			{AudioTrackIndex: 1, Language: "de", ChannelCount: 2},
			{AudioTrackIndex: 2, Language: "bad tag,x", ChannelCount: 8},
		},
	}
	arguments := prepare.Arguments("/media/a film.mkv", "/prep/7/remux/0123456789abcdef", command, prepare.EncoderX264)
	if after(arguments, "-i") != "file:/media/a film.mkv" || after(arguments, "-protocol_whitelist") != "file" {
		t.Errorf("the input is %q, want file: and only the file protocol", after(arguments, "-i"))
	}
	if after(arguments, "-map") != "0:0" {
		t.Errorf("the picture is mapped as %q, want by its stream index", after(arguments, "-map"))
	}
	if after(arguments, "-c:v") != "copy" || slices.Contains(arguments, "-tag:v") {
		t.Errorf("remuxed H.264 is %v, want copied untagged", arguments)
	}
	wantMap := "v:0,agroup:audio,name:video " +
		"a:0,agroup:audio,language:en,name:audio0,default:yes a:1,agroup:audio,language:en,name:audio0-boost " +
		"a:2,agroup:audio,language:de,name:audio1 " +
		"a:3,agroup:audio,language:und,name:audio2 a:4,agroup:audio,language:und,name:audio2-boost"
	if got := after(arguments, "-var_stream_map"); got != wantMap {
		t.Errorf("the stream map is\n%s\nwant\n%s", got, wantMap)
	}
	if !strings.Contains(after(arguments, "-filter:a:1"), "1.414*FC") ||
		!strings.Contains(after(arguments, "-filter:a:0"), "0.707*FC") ||
		after(arguments, "-filter:a:2") != "aformat=channel_layouts=stereo" {
		t.Errorf("the mixes are %q, %q, %q", after(arguments, "-filter:a:0"), after(arguments, "-filter:a:1"),
			after(arguments, "-filter:a:2"))
	}
	if after(arguments, "-hls_segment_type") != "fmp4" || after(arguments, "-hls_playlist_type") != "event" ||
		arguments[len(arguments)-1] != "/prep/7/remux/0123456789abcdef/%v/stream.m3u8" {
		t.Errorf("the output is %v, want fMP4 HLS as an EVENT playlist under the rendition", arguments)
	}
}

func TestArguments_EachMixOfAMeasuredTrackCarriesItsLevellingGain(t *testing.T) {
	t.Parallel()
	command := nodeproto.Prepare{
		Profile: nodeproto.ProfileRemux, VideoCodec: "h264", RenditionDir: "7/remux/0123456789abcdef",
		AudioTracks: []nodeproto.PrepareAudio{
			{AudioTrackIndex: 0, Language: "en", ChannelCount: 6, CarriedGainDB: 12.7},
			{AudioTrackIndex: 1, Language: "de", ChannelCount: 2, CarriedGainDB: -3.4},
			{AudioTrackIndex: 2, Language: "fr", ChannelCount: 2},
		},
	}
	arguments := prepare.Arguments("/media/film.mkv", "/prep/7/remux/0123456789abcdef", command, prepare.EncoderX264)
	for slot, want := range []string{
		prepare.StandardMix + ",volume=12.7dB", ",volume=12.7dB", prepare.StereoAsIsMix + ",volume=-3.4dB",
		prepare.StereoAsIsMix,
	} {
		got := after(arguments, "-filter:a:"+strconv.Itoa(slot))
		if !strings.HasSuffix(got, want) || (slot == 3 && strings.Contains(got, "volume")) {
			t.Errorf("mix %d is %q, want it to end %q: the gain after the mix it was measured as (D226 §4)",
				slot, got, want)
		}
	}
}

func TestArguments_TagCopiedHEVCForSafariAndReEncodeTo8BitH264(t *testing.T) {
	t.Parallel()
	hevc := prepare.Arguments(
		"/a", "/b", nodeproto.Prepare{Profile: nodeproto.ProfileRemux, VideoCodec: "hevc"}, prepare.EncoderX264)
	if after(hevc, "-tag:v") != "hvc1" {
		t.Errorf("copied HEVC is %v, want tagged hvc1", hevc)
	}
	encoded := prepare.Arguments(
		"/a", "/b", nodeproto.Prepare{Profile: nodeproto.ProfileH264At8M, VideoCodec: "hevc"}, prepare.EncoderX264)
	if after(encoded, "-c:v") != "libx264" || after(encoded, "-pix_fmt") != "yuv420p" ||
		after(encoded, "-b:v") != "8000k" || after(encoded, "-force_key_frames") != "expr:gte(t,n_forced*2)" {
		t.Errorf("h264-8m is %v, want 8-bit H.264 at 8M with a keyframe every 2 s", encoded)
	}
}

// An encode is made at the rate its command names for its picture, its peak at most a quarter more
// (D256).
func TestArguments_AnEncodeIsMadeAtTheRateItsCommandNames(t *testing.T) {
	t.Parallel()
	command := nodeproto.Prepare{Profile: nodeproto.ProfileH264At8M, VideoCodec: "mpeg4", VideoKbps: 1000}
	encoded := prepare.Arguments("/a", "/b", command, prepare.EncoderX264)
	if after(encoded, "-b:v") != "1000k" || after(encoded, "-maxrate") != "1250k" ||
		after(encoded, "-bufsize") != "2000k" {
		t.Errorf("an encode told 1000 kb/s is %v; want 1000k, its peak 1250k and its buffer 2000k", encoded)
	}
}

func TestArguments_SegmentLengthIsTheOneAskedForWithinBounds(t *testing.T) {
	t.Parallel()
	for asked, want := range map[int]string{0: "2", 1: "1", 6: "6", 10: "10", 11: "2", 60: "2"} {
		command := nodeproto.Prepare{Profile: nodeproto.ProfileRemux, SegmentLengthSeconds: asked}
		if got := after(prepare.Arguments("/a", "/b", command, prepare.EncoderX264), "-hls_time"); got != want {
			t.Errorf("asked for %d-second segments: -hls_time %s, want %s", asked, got, want)
		}
	}
}

// An HLS segment is cut only at a keyframe, so an encode puts one at every segment length: a
// 3-second length gets 3-second segments, not the 4 that keyframes every 2 seconds would make.
func TestArguments_AnEncodePutsAKeyframeAtEverySegmentLength(t *testing.T) {
	t.Parallel()
	for asked, want := range map[int]string{
		1: "expr:gte(t,n_forced*1)", 3: "expr:gte(t,n_forced*3)", 10: "expr:gte(t,n_forced*10)",
		0: "expr:gte(t,n_forced*2)", // out of bounds: the segments' own fallback length
	} {
		command := nodeproto.Prepare{Profile: nodeproto.ProfileH264At8M, SegmentLengthSeconds: asked}
		arguments := prepare.Arguments("/a", "/b", command, prepare.EncoderX264)
		if got := after(arguments, "-force_key_frames"); got != want {
			t.Errorf("%d-second segments: -force_key_frames %s, want %s (-hls_time %s)", asked, got, want,
				after(arguments, "-hls_time"))
		}
	}
}

// A prepare names no -progress of its own: the watched run puts FFmpeg's on descriptor 3, and a
// later one would win and leave the watchdog nothing to see.
func TestArguments_LeaveTheProgressToTheWatchedRun(t *testing.T) {
	t.Parallel()
	command := nodeproto.Prepare{Profile: nodeproto.ProfileRemux, RenditionDir: "7/remux/0123456789abcdef"}
	arguments := prepare.Arguments("/media/film.mkv", "/prep/7/remux/0123456789abcdef", command, prepare.EncoderX264)
	if slices.Contains(arguments, "-progress") {
		t.Errorf("a prepare's arguments %q name a -progress, which would take FFmpeg's from the watchdog",
			arguments)
	}
}
