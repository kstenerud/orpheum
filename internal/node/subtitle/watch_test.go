// SPDX-License-Identifier: GPL-3.0-or-later

package subtitle

import (
	"context"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/execx"
)

// watchingRunner records how each run was watched, and answers with a text.
type watchingRunner struct{ watches *[]execx.Watch }

func (runner watchingRunner) RunWatched(_ context.Context, run execx.WatchedRun) (execx.Ran, error) {
	*runner.watches = append(*runner.watches, run.Watch)
	return execx.Ran{Stdout: []byte("text")}, nil
}

func TestExtract_EveryRunIsWatchedATracksLongEnoughForItsQuietStretches(t *testing.T) {
	t.Parallel()
	var watches []execx.Watch
	extractor := NewExtractor(watchingRunner{&watches}, 20<<20)
	if _, err := extractor.ExtractTrack(t.Context(), "/media/film.mkv", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := extractor.ConvertText(t.Context(), []byte("the sidecar")); err != nil {
		t.Fatal(err)
	}
	if len(watches) != 4 {
		t.Fatalf("%d runs watched, want a track and its WebVTT, a sidecar and its WebVTT", len(watches))
	}
	if watches[0].StallWindow != 10*time.Minute || watches[0].StartGrace != 10*time.Minute ||
		watches[0].Deadline != execx.ReadingDeadline("/media/film.mkv", 20<<20) {
		t.Errorf("a track's run was watched %+v; want ten minutes for a stretch without a subtitle", watches[0])
	}
	for _, conversion := range watches[1:] {
		if conversion.StallWindow != time.Minute || conversion.Deadline != 5*time.Minute {
			t.Errorf("a conversion was watched %+v; want a minute standing still, five in all", conversion)
		}
	}
}
