// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package readthrough_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/disc/disctest"
	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/readthrough"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// streamedFilm is the file at path as a node that streams films makes it a run's film, a disc
// image's its main title.
func streamedFilm(t *testing.T, runner *execx.Runner, path string, isDiscImage bool) execx.Film {
	t.Helper()
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
		Window: time.Minute, IsDiscImage: isDiscImage,
	}, execx.ReadsWholeFilm)
	if err != nil {
		t.Fatal(err)
	}
	return film
}

// A Blu-ray's main title, served through fdproxy from its image, reads through as the M2TS clip it
// holds does on its own: the same keyframes and bytes in each second, which its chunks are cut by
// (D350, D247 §10).
func TestRead_ADiscsMainTitleReadsThroughAsTheStreamItHolds(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	runner.StreamFilms()
	directory := t.TempDir()
	clip := filepath.Join(directory, "00001.m2ts")
	testutil.GenerateMedia(t, clip,
		"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25:duration=6",
		"-c:v", "libx264", "-preset", "ultrafast", "-x264-params", "keyint=50:min-keyint=50:scenecut=0",
		"-f", "mpegts", "-mpegts_m2ts_mode", "1")
	stream, err := os.ReadFile(clip)
	if err != nil {
		t.Fatal(err)
	}
	bluRay := disctest.BluRay{
		Playlists: []disctest.BluRayPlaylist{{Number: 1, Items: []disctest.BluRayItem{{Clip: "00001", OutMs: 6000}}}},
		Clips:     map[string][]byte{"00001": stream},
	}
	image := filepath.Join(directory, "Film.iso")
	written := disctest.Volume{Files: bluRay.Files(), Metadata: true, SplitBytes: 1 << 20}.Bytes()
	if err = os.WriteFile(image, written, 0o600); err != nil {
		t.Fatal(err)
	}
	reader := readthrough.NewReader(runner, 20<<20)
	ask := &nodeproto.VideoTimelineAsk{VideoStreamIndex: 0}
	fromImage, err := reader.Read(t.Context(), streamedFilm(t, runner, image, true), nil, ask, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	onItsOwn, err := reader.Read(t.Context(), streamedFilm(t, runner, clip, false), nil, ask, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fromImage.VideoTimeline == nil || len(fromImage.VideoTimeline.KeyframesMs) != 3 ||
		!slices.Equal(fromImage.VideoTimeline.KeyframesMs, onItsOwn.VideoTimeline.KeyframesMs) ||
		!slices.Equal(fromImage.VideoTimeline.BytesInEachSecond, onItsOwn.VideoTimeline.BytesInEachSecond) {
		t.Errorf("the disc's title read through as %+v; want its clip's own timeline, %+v, three keyframes",
			fromImage.VideoTimeline, onItsOwn.VideoTimeline)
	}
}
