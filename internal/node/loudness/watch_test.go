// SPDX-License-Identifier: GPL-3.0-or-later

package loudness

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// stallingRunner records how each run was watched, and answers as a hung FFmpeg the watchdog
// stopped.
type stallingRunner struct{ watches *[]execx.Watch }

func (runner stallingRunner) RunWatched(_ context.Context, run execx.WatchedRun) (execx.Ran, error) {
	*runner.watches = append(*runner.watches, run.Watch)
	return execx.Ran{}, &execx.StalledError{Tool: execx.FFmpeg, Cause: execx.StallStoodStill}
}

func TestMeasure_IsWatchedForAPositionThatStandsStillAndSaysWhenItStalled(t *testing.T) {
	t.Parallel()
	var watches []execx.Watch
	// A large film, without taking the disk: its deadline is longer than the shortest any run gets.
	video := filepath.Join(t.TempDir(), "film.mkv")
	film, err := os.Create(video)
	if err != nil {
		t.Fatal(err)
	}
	_, err = film.WriteAt([]byte{0}, 60<<30-1)
	if closeErr := film.Close(); err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	const slowestRead = 20 << 20
	_, err = NewMeasurer(stallingRunner{&watches}, slowestRead).Measure(t.Context(), video,
		[]nodeproto.LoudnessTrack{{StreamIndex: 1, ChannelCount: 2}})
	stalled, isStalled := errors.AsType[*execx.StalledError](err)
	if !isStalled || stalled.Cause != execx.StallStoodStill {
		t.Errorf("a stalled measurement: %v, want the stall", err)
	}
	if len(watches) != 1 || watches[0].StartGrace != 2*time.Minute || watches[0].StallWindow != 3*time.Minute ||
		watches[0].Deadline != execx.ReadingDeadline(video, slowestRead) {
		t.Errorf("a measurement was watched %+v; want two minutes to start, three standing still, and the "+
			"deadline of reading the file at the node's slowest read", watches)
	}
}
