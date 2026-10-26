// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

import (
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/disc"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// discTitleOutput is ffprobe's output, in the shape the pinned ffprobe writes it, for a DVD's main
// title served as a program stream: its streams named by their identifiers, no language on any but
// one, no chapters, and a duration the demuxer estimated.
const discTitleOutput = `{
  "streams": [
    {"index": 0, "id": "0x1e0", "codec_type": "video", "codec_name": "mpeg2video", "width": 720, "height": 576},
    {"index": 1, "id": "0x80", "codec_type": "audio", "codec_name": "ac3", "channels": 6},
    {"index": 2, "id": "0x89", "codec_type": "audio", "codec_name": "dts", "channels": 6},
    {"index": 3, "id": "0x20", "codec_type": "subtitle", "codec_name": "dvd_subtitle"},
    {"index": 4, "id": "0x81", "codec_type": "audio", "codec_name": "ac3", "tags": {"language": "ita"}}
  ],
  "chapters": [],
  "format": {"format_name": "mpeg", "duration": "12.300000"}
}`

// A disc's main title is probed for what is in its stream and told what only the disc says: which
// disc and title it is, its duration, its chapters, and its streams' languages by their identifiers,
// a stream's own language kept (D350 §4).
func TestProbe_ADiscsTitleCarriesWhatOnlyTheDiscSays(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stdout: discTitleOutput})
	runner, err := execx.NewRunner(fake.Directory)
	if err != nil {
		t.Fatal(err)
	}
	title := disc.Title{
		Kind: disc.DVD, Number: 2, DurationMs: 6_200_000, ChaptersMs: []int64{0, 3_000_000},
		Tracks: []disc.Track{
			{ID: 0x20, Kind: "subtitle", Language: "de"},
			{ID: 0x80, Kind: "audio", Language: "en"},
			{ID: 0x81, Kind: "audio", Language: "fr"},
			{ID: 0x89, Kind: "audio", Language: "ja"},
		},
	}
	prober := NewProber(runner, time.Minute)
	record, err := prober.Probe(t.Context(), execx.Film{Path: "/media/film.iso", Disc: &title})
	if err != nil {
		t.Fatal(err)
	}
	if record.Format.Disc == nil || *record.Format.Disc != (nodeproto.ProbeDisc{Kind: "dvd", TitleNumber: 2}) ||
		record.Format.DurationMs != 6_200_000 {
		t.Errorf("the title's format is %+v; want DVD title 2, playing 6,200,000 ms as the disc says", record.Format)
	}
	wantChapters := []nodeproto.ProbeChapter{{StartMs: 0, EndMs: 3_000_000}, {StartMs: 3_000_000, EndMs: 6_200_000}}
	if !slices.Equal(record.Chapters, wantChapters) {
		t.Errorf("its chapters are %+v; want the disc's, %+v", record.Chapters, wantChapters)
	}
	var languages []string
	for _, stream := range record.Streams {
		languages = append(languages, stream.ContainerLanguage+"/"+stream.Language)
	}
	if want := []string{"/", "en/en", "ja/ja", "de/de", "ita/it"}; !slices.Equal(languages, want) {
		t.Errorf("its streams' languages are %q; want %q, the disc's by identifier, a stream's own kept",
			languages, want)
	}
	plain, err := prober.Probe(t.Context(), execx.Film{Path: "/media/film.vob"})
	if err != nil || plain.Format.Disc != nil || plain.Format.DurationMs != 12_300 || plain.Streams[1].Language != "" {
		t.Errorf("a film that is no disc probed as %+v (%v); want ffprobe's record alone", plain.Format, err)
	}
}
