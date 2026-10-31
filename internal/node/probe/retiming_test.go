// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

import (
	"encoding/json"
	"testing"
)

// ffprobeOutputOf is ffprobe's output for a file of one picture, of codec, saying it reorders
// hasBFrames frames, and an audio stream, whose first packets interleave the picture's, given by
// their times, with the sound's: each picture packet's pts and dts, nil for none.
func ffprobeOutputOf(t *testing.T, codec string, hasBFrames int, picture [][2]*int64) []byte {
	t.Helper()
	type packet struct {
		StreamIndex int    `json:"stream_index"`
		Pts         *int64 `json:"pts,omitempty"`
		Dts         *int64 `json:"dts,omitempty"`
	}
	packets := make([]packet, 0, 2*len(picture))
	for index, times := range picture {
		sound := int64(index) * 1024
		packets = append(packets, packet{StreamIndex: 0, Pts: times[0], Dts: times[1]},
			packet{StreamIndex: 1, Pts: &sound, Dts: &sound})
	}
	output, err := json.Marshal(map[string]any{
		"format": map[string]any{"format_name": "mov,mp4,m4a,3gp,3g2,mj2"},
		"streams": []map[string]any{
			{"index": 0, "codec_type": "video", "codec_name": codec, "has_b_frames": hasBFrames},
			{"index": 1, "codec_type": "audio", "codec_name": "aac"},
		},
		"packets": packets,
	})
	if err != nil {
		t.Fatal(err)
	}
	return output
}

// flatPicture is count packets of a picture each shown at its decode time.
func flatPicture(count int) [][2]*int64 {
	picture := make([][2]*int64, count)
	for index := range picture {
		at := int64(index) * 512
		picture[index] = [2]*int64{&at, &at}
	}
	return picture
}

// reorderedPicture is count packets of a picture whose B-frames are shown after they are decoded,
// as an intact H.264 or HEVC file says.
func reorderedPicture(count int) [][2]*int64 {
	picture := make([][2]*int64, count)
	for index := range picture {
		decoded, shown := int64(index)*512, int64(index+2)*512
		if index%4 != 0 {
			shown -= 1024
		}
		picture[index] = [2]*int64{&shown, &decoded}
	}
	return picture
}

// A picture is marked for re-timing when it is H.264 or HEVC, says it reorders frames, and every
// one of its packets among the first ffprobe lists, at least a second's worth, is shown at its
// decode time: file 5667, whose B-frames carry no display times (design/plans/
// audio-switch-and-retiming-2026-11-01.md §2.4). Nothing else is.
func TestParse_APictureWhoseBFramesCarryNoDisplayTimesNeedsRetiming(t *testing.T) {
	t.Parallel()
	withoutPts := flatPicture(100)
	withoutPts[50][0] = nil
	for _, test := range []struct {
		name       string
		codec      string
		hasBFrames int
		picture    [][2]*int64
		want       bool
	}{
		{"HEVC with B-frames, every packet shown when decoded", "hevc", 2, flatPicture(100), true},
		{"H.264 with B-frames, every packet shown when decoded", "h264", 3, flatPicture(100), true},
		{"HEVC whose B-frames are shown after they are decoded", "hevc", 2, reorderedPicture(100), false},
		{"a picture saying it reorders nothing", "hevc", 0, flatPicture(100), false},
		{"a codec dts2pts cannot re-time", "mpeg2video", 2, flatPicture(100), false},
		{"fewer than a second's packets read", "hevc", 2, flatPicture(23), false},
		{"a packet without a display time", "hevc", 2, withoutPts, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			record, err := Parse(ffprobeOutputOf(t, test.codec, test.hasBFrames, test.picture))
			if err != nil {
				t.Fatal(err)
			}
			if got := record.Streams[0].Video.NeedsRetiming; got != test.want {
				t.Errorf("the picture needs re-timing: %v, want %v", got, test.want)
			}
			if record.Streams[1].Video.NeedsRetiming {
				t.Error("the sound is marked for re-timing")
			}
		})
	}
}

// Only the first packetsRead packets are judged, as ffprobe was asked for: a hostile ffprobe output
// listing more cannot make the node judge by them.
func TestParse_JudgesRetimingByTheFirstPacketsAlone(t *testing.T) {
	t.Parallel()
	// 100 picture packets interleaved with 100 of sound fill the 200; the reordered ones after are not read.
	picture := append(flatPicture(100), reorderedPicture(50)...)
	record, err := Parse(ffprobeOutputOf(t, "hevc", 2, picture))
	if err != nil {
		t.Fatal(err)
	}
	if !record.Streams[0].Video.NeedsRetiming {
		t.Error("a picture flat through its first 200 packets is not marked; packets past them were judged")
	}
}
