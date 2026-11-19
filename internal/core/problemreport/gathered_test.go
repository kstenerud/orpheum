// SPDX-License-Identifier: AGPL-3.0-or-later

package problemreport

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// A played file's shape is read from its kept probe: the container, each stream's codec, profile,
// bit depth, size, rate, layout, language and dispositions; never the container's or a track's title
// (D74).
func TestShape_KeepsTheFilesShapeAndLeavesItsTitlesBehind(t *testing.T) {
	t.Parallel()
	file := gatheredFile{Path: "/media/films/a/a.mkv", SizeBytes: 4 << 30, Probe: nodeproto.Probe{
		Format: nodeproto.ProbeFormat{
			FormatNames: []string{"matroska", "webm"}, DurationMs: 5400000, ContainerTitle: "a container title",
		},
		Streams: []nodeproto.ProbeStream{
			{
				StreamIndex: 0, Kind: "video", Codec: "hevc", CodecProfile: "Main 10", ContainerTitle: "a track title",
				Video: nodeproto.ProbeVideo{
					Width: 3840, Height: 2160, PixelFormat: "yuv420p10le", FrameRate: "24000/1001",
				},
			},
			{
				StreamIndex: 1, Kind: "audio", Codec: "eac3", Language: "de", Disposition: []string{"default"},
				Audio: nodeproto.ProbeAudio{ChannelLayout: "5.1(side)"},
			},
			{StreamIndex: 2, Kind: "video", Codec: "mjpeg", Video: nodeproto.ProbeVideo{PixelFormat: "yuvj420p"}},
		},
	}}
	shape := file.shape()
	if shape.Container != "webm" || shape.Duration != 90*time.Minute || shape.SizeBytes != 4<<30 ||
		shape.Path != file.Path || len(shape.Streams) != 3 {
		t.Fatalf("shape = %+v; want the webm of 90 minutes, its size, path and three streams", shape)
	}
	want := []Stream{
		{
			Index: 0, Kind: "video", Codec: "hevc", Profile: "Main 10", BitDepth: 10, Width: 3840, Height: 2160,
			FrameRate: "24000/1001",
		},
		{
			Index: 1, Kind: "audio", Codec: "eac3", ChannelLayout: "5.1(side)", Language: "de",
			Dispositions: []string{"default"},
		},
		{Index: 2, Kind: "video", Codec: "mjpeg", BitDepth: 8},
	}
	for index, stream := range shape.Streams {
		if got, wanted := fmt.Sprintf("%+v", stream), fmt.Sprintf("%+v", want[index]); got != wanted {
			t.Errorf("stream %d = %s; want %s", index, got, wanted)
		}
	}
	if printed := fmt.Sprintf("%+v", *shape); strings.Contains(printed, "title") {
		t.Errorf("the shape carries a title: %s", printed)
	}
}
