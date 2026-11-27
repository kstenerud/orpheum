// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package trickplay

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// The fixture: 21 s at 10 frames a second, a keyframe every 2.5 s, each frame's brightness ten times
// its time in seconds, so a picture says which frame it is.
const (
	fixtureSeconds     = 21
	fixtureKeyframeGap = 2.5
	lumaPerSecond      = 10
	// The fixture's count of sheets at a picture a second, four to a sheet: marks 0 to 20.
	fixtureSheetCount = 6
)

// fixtureCommand is one picture a second, 32 px wide, in sheets of 2×2.
func fixtureCommand() nodeproto.MakeTrickplay {
	return nodeproto.MakeTrickplay{IntervalMs: 1000, PictureWidth: 32, SheetColumns: 2, SheetRows: 2, WebPQuality: 60}
}

func generateFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "film.mp4")
	testutil.GenerateMedia(t, path, "-f", "lavfi",
		"-i", "color=c=black:s=64x36:r=10:d="+strconv.Itoa(fixtureSeconds)+",format=yuv420p,"+
			"geq=lum="+strconv.Itoa(lumaPerSecond)+"*T:cb=128:cr=128",
		"-c:v", "libx264", "-g", "25", "-keyint_min", "25", "-sc_threshold", "0")
	return path
}

// madeSheets runs Make and returns each sheet it gave, by index, in the order given.
func madeSheets(
	t *testing.T, runner *execx.Runner, film string, command nodeproto.MakeTrickplay,
) ([]int, map[int][]byte) {
	t.Helper()
	var order []int
	sheets := map[int][]byte{}
	err := NewMaker(runner, 20<<20).Make(t.Context(), execx.Film{Path: film}, command,
		func(index int, webp []byte) error {
			order = append(order, index)
			sheets[index] = webp
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	return order, sheets
}

// trickplay.md §7: each picture is the keyframe at or before its mark, within one keyframe gap of
// it, its sheets laid row by row, the last one short.
func TestMake_EachPictureIsTheKeyframeAtOrBeforeItsMark(t *testing.T) {
	t.Parallel()
	runner := newIntegrationRunner(t)
	film := generateFixture(t)
	order, sheets := madeSheets(t, runner, film, fixtureCommand())
	if len(order) != fixtureSheetCount {
		t.Fatalf("made sheets %v; want %d, 0 onwards", order, fixtureSheetCount)
	}
	for index, made := range order {
		if made != index {
			t.Fatalf("made sheets %v; want them in order", order)
		}
	}
	for index := range fixtureSheetCount {
		// 64 × 36 at 32 across is 18 high, the short last sheet's pictures too (trickplay.md §5).
		if height, err := PictureHeight(sheets[index], fixtureCommand().SheetRows); err != nil || height != 18 {
			t.Errorf("sheet %d's pictures are %d px high (%v); want 18, the picture's shape at 32 across", index,
				height, err)
		}
		lumas := tileLumas(t, runner, sheets[index])
		for tile, luma := range lumas {
			mark := float64(index*4 + tile)
			if mark > fixtureSeconds-1 {
				continue // the last sheet's empty tiles
			}
			shown := float64(luma) / lumaPerSecond
			keyframe := math.Floor(mark/fixtureKeyframeGap) * fixtureKeyframeGap
			if math.Abs(shown-keyframe) > 0.5 {
				t.Errorf("sheet %d's picture %d, mark %.0f s, shows %.1f s; want the keyframe at or before "+
					"it, %.1f s", index, tile, mark, shown, keyframe)
			}
		}
	}
}

// trickplay.md §7: a run that resumes makes the sheets not yet made, each as a run from the start
// makes it, and gives none of those made again: seeking to a sheet picks the same keyframes.
func TestMake_AResumeMakesEachSheetNotMadeAsARunFromTheStartDoesAndNoneMadeAgain(t *testing.T) {
	t.Parallel()
	runner := newIntegrationRunner(t)
	film := generateFixture(t)
	_, whole := madeSheets(t, runner, film, fixtureCommand())
	for _, check := range []struct {
		made, want []int
	}{
		{made: []int{0, 1, 2}, want: []int{3, 4, 5}}, // stopped after sheet 2
		{made: []int{0, 1, 3}, want: []int{2, 4, 5}}, // sheet 2 lost: sheet 3 is not given again
	} {
		command := fixtureCommand()
		command.SheetsMade = check.made
		order, resumed := madeSheets(t, runner, film, command)
		if len(order) != len(check.want) {
			t.Errorf("with %v made, a resume made %v; want %v", check.made, order, check.want)
			continue
		}
		for position, index := range check.want {
			if order[position] != index {
				t.Errorf("with %v made, a resume made %v; want %v", check.made, order, check.want)
				break
			}
			lumas, wholeLumas := tileLumas(t, runner, resumed[index]), tileLumas(t, runner, whole[index])
			if !bytes.Equal(lumas, wholeLumas) {
				t.Errorf("with %v made, sheet %d shows %v; a run from the start shows %v", check.made, index,
					lumas, wholeLumas)
			}
		}
	}
}

func newIntegrationRunner(t *testing.T) *execx.Runner {
	t.Helper()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

// tileLumas is the mean brightness of each of a 2×2 sheet's pictures, row by row: the sheet scaled
// to 2×2 by area, each pixel the mean of its picture, read from the Y plane the fixture's
// brightness was written to, in its own range.
func tileLumas(t *testing.T, runner *execx.Runner, webp []byte) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sheet.webp")
	if err := os.WriteFile(path, webp, 0o600); err != nil {
		t.Fatal(err)
	}
	arguments := append([]string{"-v", "error"}, execx.FileInput(path)...)
	arguments = append(arguments, "-vf", "scale=2:2:flags=area", "-f", "rawvideo", "-pix_fmt", "yuv420p", "pipe:1")
	const planes = 4 + 1 + 1 // Y, U and V of 2×2
	picture, err := runner.Run(t.Context(), execx.FFmpeg, confine.Access{Read: []string{path}}, arguments, planes)
	if err != nil {
		t.Fatal(err)
	}
	if len(picture) != planes {
		t.Fatalf("a sheet read as %d bytes; want %d", len(picture), planes)
	}
	return picture[:4]
}
