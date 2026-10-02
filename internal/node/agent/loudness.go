// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"errors"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Measurer measures a video's audio tracks; loudness.Measurer is the real one.
type Measurer interface {
	Measure(ctx context.Context, videoPath string, tracks []nodeproto.LoudnessTrack) ([]nodeproto.TrackLoudness, error)
}

// loudnessWork is a measurement held until the node's one measurement slot is free, since it reads
// the whole video, which posts its answer to the core (D226 §2).
func (agent *Agent) loudnessWork(
	ctx context.Context, nodeID int64, held attempt, command nodeproto.MeasureLoudness,
) heldWork {
	return agent.answeringWork(ctx, nodeID, answeringJob{
		attempt: held, slot: slotMeasure, jobKind: "a loudness measurement",
		makeAnswer: func(jobCtx context.Context) (nodeproto.Result, string) {
			answer := agent.runMeasurement(jobCtx, command)
			return nodeproto.Result{Loudness: &answer}, answer.Refusal + answer.Failure + stallDetail(answer.Stall)
		},
	})
}

// runMeasurement measures a video checked to be the one the core named (D100), or says why not:
// that FFmpeg could not, which is the file's, or that the node did not try.
func (agent *Agent) runMeasurement(ctx context.Context, command nodeproto.MeasureLoudness) nodeproto.LoudnessResult {
	refuse := func(err error) nodeproto.LoudnessResult {
		return nodeproto.LoudnessResult{Refusal: err.Error()}
	}
	if agent.workers.LoudnessMeasurer == nil {
		return refuse(errors.New("this node does not measure loudness"))
	}
	videoPath, err := agent.verifiedSourcePath(command.Video)
	if err != nil {
		return refuse(err)
	}
	tracks, err := agent.workers.LoudnessMeasurer.Measure(ctx, videoPath, command.Tracks)
	if stall := stallOf(err); stall != nil {
		return nodeproto.LoudnessResult{Stall: stall}
	}
	if err != nil {
		return nodeproto.LoudnessResult{Failure: err.Error()}
	}
	return nodeproto.LoudnessResult{Tracks: tracks}
}
