// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package probe

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/disc/disctest"
	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// madeByThePinnedFFmpeg is two seconds of a test picture and a tone, made by the pinned FFmpeg in
// format with the picture's codec, as a disc holds them: a DVD's program stream, a Blu-ray's M2TS.
func madeByThePinnedFFmpeg(t *testing.T, picture string, format ...string) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stream")
	arguments := []string{
		"-f", "lavfi", "-i", "testsrc2=size=720x576:rate=25:duration=2",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:v", picture, "-c:a", "ac3",
	}
	testutil.GenerateMedia(t, path, append(arguments, format...)...)
	stream, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return stream
}

// probeImage writes image as an .iso and probes it as a node that streams films does.
func probeImage(t *testing.T, runner *execx.Runner, image []byte) nodeproto.Probe {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Film (2001).iso")
	if err := os.WriteFile(path, image, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() }) //nolint:errcheck // read only
	facts, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	film, err := runner.FilmOf(t.Context(), path, execx.OpenedFilm{
		File: file, Facts: facts, Disks: diskwatch.NewGate(clock.System{}), Root: filepath.Dir(path),
		Window: time.Minute, IsDiscImage: true,
	}, execx.ReadsIndex)
	if err != nil {
		t.Fatal(err)
	}
	record, err := NewProber(runner, time.Minute).Probe(t.Context(), film)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

// The pinned ffprobe, served a DVD's and a Blu-ray's main title through fdproxy from images holding
// streams the pinned FFmpeg made, reads them as the program stream and the M2TS they are, and the
// record carries the disc's languages and duration (D350).
func TestProbe_ThePinnedFFprobeReadsADiscsMainTitleServedFromItsImage(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	runner.StreamFilms()
	vob := madeByThePinnedFFmpeg(t, "mpeg2video", "-f", "vob")
	half := len(vob) / 2 / 2048 * 2048
	dvd := disctest.DVD{
		Titles: []disctest.DVDTitle{{Cells: []disctest.DVDCell{
			{Data: vob[:half], DurationMs: 1000}, {Data: vob[half:], DurationMs: 1000},
		}, VOBBytes: half}},
		Audio: []disctest.DVDAudio{{Coding: 0, Stream: 0, Language: "en"}},
	}
	m2ts := madeByThePinnedFFmpeg(t, "libx264", "-f", "mpegts", "-mpegts_m2ts_mode", "1")
	bluRay := disctest.BluRay{
		Playlists: []disctest.BluRayPlaylist{{
			Number: 800, Items: []disctest.BluRayItem{{Clip: "00001", OutMs: 2000}},
			Sounds: []disctest.BluRayStream{{PID: 0x1100, Coding: 0x81, Language: "eng"}},
		}},
		Clips: map[string][]byte{"00001": m2ts},
	}
	for name, example := range map[string]struct {
		image                 []byte
		format, picture, lang string
	}{
		"a DVD":     {disctest.Volume{Files: dvd.Files(), SplitBytes: 64 << 10}.Bytes(), "mpeg", "mpeg2video", "en"},
		"a Blu-ray": {disctest.Volume{Files: bluRay.Files(), Metadata: true}.Bytes(), "mpegts", "h264", "eng"},
	} {
		record := probeImage(t, runner, example.image)
		if !slices.Contains(record.Format.FormatNames, example.format) || len(record.Streams) != 2 ||
			record.Streams[0].Codec != example.picture || record.Streams[1].Codec != "ac3" {
			t.Fatalf("%s's title probed as %+v; want %s with %s and AC-3",
				name, record, example.format, example.picture)
		}
		if record.Streams[1].ContainerLanguage != example.lang || record.Streams[1].Language != "en" ||
			record.Format.Disc == nil || record.Format.DurationMs != 2000 {
			t.Errorf("%s's title is %+v, its sound %+v; want the disc's duration and its sound's language",
				name, record.Format, record.Streams[1])
		}
	}
}
