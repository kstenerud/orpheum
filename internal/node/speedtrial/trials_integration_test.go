// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package speedtrial

import (
	"log/slog"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/encode"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// The real FFmpeg makes a small clip of one kind and makes it into both its qualities with a run of
// chunks' arguments, each measured on the CPU at some speed: the arguments are ones FFmpeg takes.
func TestRun_TheRealFFmpegMakesAClipAndMeasuresIt(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	trials := New(Config{
		Runner: runner, Encoder: encode.EncoderX264, Key: testKey(), ClipsDir: t.TempDir(), KeptDir: t.TempDir(),
		Clock: clock.System{}, Logger: slog.New(slog.DiscardHandler),
		Kinds: []Kind{{Codec: "mpeg4", BitDepth: 8, Height: 144, Encoder: "mpeg4", options: []string{
			"-pix_fmt", "yuv420p", "-b:v", "500k",
		}}},
	})
	if err := trials.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	results := trials.Results()
	if len(results) != 2 {
		t.Fatalf("the results are %+v; want two", results)
	}
	for _, row := range results {
		if row.SpeedPercent <= 0 || row.DecodedOn != "cpu" || row.Failure != "" || row.NotTried != "" {
			t.Errorf("trial %+v; want measured on the CPU", row)
		}
	}
}
