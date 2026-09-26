// SPDX-License-Identifier: GPL-3.0-or-later

package prepare_test

import (
	"slices"
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
		Profile: nodeproto.ProfileRemux, VideoCodec: "h264", Dir: "7/remux",
		Audio: []nodeproto.PrepareAudio{
			{Index: 0, Language: "en", Channels: 6},
			{Index: 1, Language: "de", Channels: 2},
			{Index: 2, Language: "bad tag,x", Channels: 8},
		},
	}
	arguments := prepare.Arguments("/media/a film.mkv", "/prep/7/remux", command)
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
	if after(arguments, "-hls_time") != "2" {
		t.Errorf("segments are %q s, want 2: switching the boost waits for the next one", after(arguments, "-hls_time"))
	}
	if after(arguments, "-hls_segment_type") != "fmp4" || after(arguments, "-hls_playlist_type") != "event" ||
		arguments[len(arguments)-1] != "/prep/7/remux/%v/stream.m3u8" {
		t.Errorf("the output is %v, want fMP4 HLS as an EVENT playlist under the rendition", arguments)
	}
}

func TestArguments_TagCopiedHEVCForSafariAndReEncodeTo8BitH264(t *testing.T) {
	t.Parallel()
	hevc := prepare.Arguments("/a", "/b", nodeproto.Prepare{Profile: nodeproto.ProfileRemux, VideoCodec: "hevc"})
	if after(hevc, "-tag:v") != "hvc1" {
		t.Errorf("copied HEVC is %v, want tagged hvc1", hevc)
	}
	encoded := prepare.Arguments("/a", "/b", nodeproto.Prepare{Profile: nodeproto.ProfileH264At8M, VideoCodec: "hevc"})
	if after(encoded, "-c:v") != "libx264" || after(encoded, "-pix_fmt") != "yuv420p" ||
		after(encoded, "-b:v") != "8M" || after(encoded, "-force_key_frames") != "expr:gte(t,n_forced*2)" {
		t.Errorf("h264-8m is %v, want 8-bit H.264 at 8M with a keyframe every 2 s", encoded)
	}
}
