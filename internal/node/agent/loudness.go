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

// measureLoudness starts a measurement beside the node's other work, one at a time, since it reads
// the whole video, and posts the answer to the core (D226 §2).
func (agent *Agent) measureLoudness(ctx context.Context, nodeID int64, command nodeproto.MeasureLoudness) {
	agent.answerInBackground(ctx, nodeID, backgroundJob{
		jobID: command.JobID, slot: agent.slots.measure, answerPath: nodeproto.PathLoudness,
		jobKind: "a loudness measurement",
		makeAnswer: func(jobCtx context.Context) (any, string) {
			answer := agent.runMeasurement(jobCtx, command)
			return answer, answer.Refusal + answer.Failure
		},
	})
}

// runMeasurement measures a video checked to be the one the core named (D100), or says why not:
// that FFmpeg could not, which is the file's, or that the node did not try.
func (agent *Agent) runMeasurement(ctx context.Context, command nodeproto.MeasureLoudness) nodeproto.LoudnessResult {
	refuse := func(err error) nodeproto.LoudnessResult {
		return nodeproto.LoudnessResult{JobID: command.JobID, Refusal: err.Error()}
	}
	if agent.workers.LoudnessMeasurer == nil {
		return refuse(errors.New("this node does not measure loudness"))
	}
	videoPath, err := agent.verifiedSourcePath(command.Video)
	if err != nil {
		return refuse(err)
	}
	tracks, err := agent.workers.LoudnessMeasurer.Measure(ctx, videoPath, command.Tracks)
	if err != nil {
		return nodeproto.LoudnessResult{JobID: command.JobID, Failure: err.Error()}
	}
	return nodeproto.LoudnessResult{JobID: command.JobID, Tracks: tracks}
}
