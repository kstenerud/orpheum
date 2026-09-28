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
	agent.answerInBackground(ctx, nodeID, background{
		job: command.Job, slot: agent.slots.measure, path: nodeproto.PathLoudness, what: "a loudness measurement",
		work: func(jobCtx context.Context) (any, string) {
			answer := agent.runMeasurement(jobCtx, command)
			return answer, answer.Refused + answer.Failed
		},
	})
}

// runMeasurement measures a video checked to be the one the core named (D100), or says why not:
// that FFmpeg could not, which is the file's, or that the node did not try.
func (agent *Agent) runMeasurement(ctx context.Context, command nodeproto.MeasureLoudness) nodeproto.Loudness {
	refuse := func(err error) nodeproto.Loudness {
		return nodeproto.Loudness{Job: command.Job, Refused: err.Error()}
	}
	if agent.workers.Loudness == nil {
		return refuse(errors.New("this node does not measure loudness"))
	}
	videoPath, err := agent.verifiedSource(command.Video)
	if err != nil {
		return refuse(err)
	}
	tracks, err := agent.workers.Loudness.Measure(ctx, videoPath, command.Audio)
	if err != nil {
		return nodeproto.Loudness{Job: command.Job, Failed: err.Error()}
	}
	return nodeproto.Loudness{Job: command.Job, Tracks: tracks}
}
