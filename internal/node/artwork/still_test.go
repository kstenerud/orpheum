// SPDX-License-Identifier: AGPL-3.0-or-later

package artwork

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A still is taken from the film it is given and nothing else, by the file protocol alone, from its
// place, of the picture stream named, black frames dropped before the thumbnail filter chooses, one
// PNG with none of the video's metadata (D362 §1).
func TestStill_FFmpegReadsOnlyTheFilmFromItsPlaceAndSkipsTheBlack(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stdout: "a png"})
	runner, err := execx.NewRunner(fake.Directory)
	if err != nil {
		t.Fatal(err)
	}
	deriver := NewDeriver(runner, time.Minute)
	still, err := deriver.Still(t.Context(), execx.Film{Path: "/media/film.mkv", Size: 1 << 30}, 2, 90*time.Second)
	if err != nil || string(still) != "a png" {
		t.Fatalf("still %q, %v; want FFmpeg's output", still, err)
	}
	arguments := fake.OnlyCall(t).Arguments
	for flag, want := range map[string]string{
		"-protocol_whitelist": "file", "-i": "file:/media/film.mkv", "-ss": "90.000", "-t": "20.000",
		"-map": "0:2", "-map_metadata": "-1", "-frames:v": "1", "-c:v": "png",
	} {
		if got := argumentAfter(arguments, flag); got != want {
			t.Errorf("FFmpeg ran with %s %q, want %q: the film by the file protocol alone, from its place for "+
				"the stretch, one PNG of stream 2 with no metadata (%q)", flag, got, want, arguments)
		}
	}
	input := slices.Index(arguments, "-i")
	if slices.Index(arguments, "-ss") > input || slices.Index(arguments, "-t") > input {
		t.Errorf("FFmpeg ran with %q; want the place and the stretch bounding the input, before it", arguments)
	}
	filters := argumentAfter(arguments, "-vf")
	black, thumbnail := strings.Index(filters, "blackframe=amount=0"), strings.Index(filters, "thumbnail=")
	isSelected := strings.Contains(filters, "metadata=mode=select:key=lavfi.blackframe.pblack")
	if black < 0 || thumbnail < black || !isSelected {
		t.Errorf("FFmpeg filtered with %q; want black frames dropped before the thumbnail filter chooses", filters)
	}
}

// argumentAfter is the argument after flag, or "" when flag is not among them.
func argumentAfter(arguments []string, flag string) string {
	index := slices.Index(arguments, flag)
	if index < 0 || index+1 >= len(arguments) {
		return ""
	}
	return arguments[index+1]
}

// A video whose stretch has no picture that is not black writes nothing, which is its answer, not a
// failure (D362 §5).
func TestStill_NothingWrittenIsAVideoWithNoFrame(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{})
	runner, err := execx.NewRunner(fake.Directory)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewDeriver(runner, time.Minute).Still(t.Context(), execx.Film{Path: "/media/black.mkv"}, 0, 0)
	if !errors.Is(err, ErrNoFrame) {
		t.Errorf("a run that wrote nothing: %v; want ErrNoFrame", err)
	}
	failing := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{ExitStatus: 1, Stderr: "Invalid data"})
	if runner, err = execx.NewRunner(failing.Directory); err != nil {
		t.Fatal(err)
	}
	_, err = NewDeriver(runner, time.Minute).Still(t.Context(), execx.Film{Path: "/media/broken.mkv"}, 0, 0)
	if err == nil || errors.Is(err, ErrNoFrame) {
		t.Errorf("a run that failed: %v; want its failure, not a video with no frame", err)
	}
}
