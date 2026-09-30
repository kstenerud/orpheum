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
	video := filepath.Join(t.TempDir(), "film.mkv")
	if err := os.WriteFile(video, []byte("a film"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewMeasurer(stallingRunner{&watches}).Measure(t.Context(), video,
		[]nodeproto.LoudnessTrack{{StreamIndex: 1, ChannelCount: 2}})
	stalled, isStalled := errors.AsType[*execx.StalledError](err)
	if !isStalled || stalled.Cause != execx.StallStoodStill {
		t.Errorf("a stalled measurement: %v, want the stall", err)
	}
	if len(watches) != 1 || watches[0].StartGrace != 2*time.Minute || watches[0].StallWindow != 3*time.Minute ||
		watches[0].Deadline != execx.ReadingDeadline(video) {
		t.Errorf("a measurement was watched %+v; want two minutes to start, three standing still, and the "+
			"deadline of reading the file", watches)
	}
}
