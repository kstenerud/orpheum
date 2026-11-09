// SPDX-License-Identifier: AGPL-3.0-or-later

package encode

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
)

// The speed trial encodes ten seconds of a generated 1080p picture, a film's usual size, scaled to
// the lowest quality chunkQualities offers by default, 400k@360p: as fast as anything this node
// would encode for a player (D457).
const (
	speedTrialKbps   = 400
	speedTrialHeight = 360
	speedTrialSource = "testsrc2=size=1920x1080:rate=24:duration=10"
)

// speedTrialTimeout bounds the speed trial: a machine that has not made its ten seconds in a minute
// encodes at under a sixth of real time, which its last word on its speed says.
const speedTrialTimeout = time.Minute

// SpeedRunner runs an FFmpeg run watched, telling its progress as it goes: execx.Runner.
type SpeedRunner interface {
	RunWatched(ctx context.Context, run execx.WatchedRun) (execx.Ran, error)
}

// MeasureSpeed is how fast encoder makes the lowest quality on this machine, in percent of real
// time, as FFmpeg last said while encoding the speed trial, confined as a run of chunks is (D457).
// A trial cut short by its timeout gives the speed it had reached; one that never said a speed is
// an error.
func MeasureSpeed(ctx context.Context, runner SpeedRunner, encoder Encoder) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, speedTrialTimeout)
	defer cancel()
	var (
		mutex  sync.Mutex
		latest float64
	)
	arguments := append([]string{"-v", "error", "-f", "lavfi", "-i", speedTrialSource},
		encoder.QualityArguments(speedTrialKbps, speedTrialHeight, "0")...)
	_, err := runner.RunWatched(ctx, execx.WatchedRun{
		Access:    confine.Access{UsesGPU: encoder.UsesGPU()},
		Arguments: append(arguments, "-f", "null", "-"),
		Watch: execx.Watch{
			StartGrace: speedTrialTimeout, StallWindow: speedTrialTimeout, Deadline: speedTrialTimeout,
			OnProgress: func(progress execx.Progress) {
				if progress.RealtimeFactor > 0 {
					mutex.Lock()
					defer mutex.Unlock()
					latest = progress.RealtimeFactor
				}
			},
		},
	})
	mutex.Lock()
	defer mutex.Unlock()
	if latest > 0 {
		return max(1, int(math.Round(latest*100))), nil
	}
	if err != nil {
		return 0, fmt.Errorf("measure %s's speed: %w", encoder, err)
	}
	return 0, fmt.Errorf("measure %s's speed: FFmpeg said none", encoder)
}
