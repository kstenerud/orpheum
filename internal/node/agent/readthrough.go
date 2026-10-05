// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"errors"
	"time"

	"github.com/kstenerud/orpheum/internal/node/readthrough"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// ThroughReader reads a video through once and learns what the core asked of it: the loudness of
// its audio tracks, the timeline of its picture, or both. The real one is readthrough.Reader.
type ThroughReader interface {
	Read(
		ctx context.Context, videoPath string, loudnessTracks []nodeproto.LoudnessTrack,
		timeline *nodeproto.VideoTimelineAsk, duration time.Duration,
	) (readthrough.Findings, error)
}

// readThroughWork is a read-through held until the node's one read-through slot is free, since it
// reads the whole video, which posts its answer to the core (D226 §2, D247 §10).
func (agent *Agent) readThroughWork(
	ctx context.Context, nodeID int64, held attempt, command nodeproto.ReadThrough,
) heldWork {
	return agent.answeringWork(ctx, nodeID, answeringJob{
		attempt: held, jobKind: "a read-through",
		makeAnswer: func(jobCtx context.Context) (nodeproto.Result, string) {
			answer := agent.runReadThrough(jobCtx, command)
			return nodeproto.Result{ReadThrough: &answer}, answer.Refusal + answer.Failure + stallDetail(answer.Stall)
		},
	})
}

// runReadThrough reads a video checked to be the one the core named (D100) through, or says why
// not: that FFmpeg could not, which is the file's, or that the node did not try.
func (agent *Agent) runReadThrough(ctx context.Context, command nodeproto.ReadThrough) nodeproto.ReadThroughResult {
	refuse := func(err error) nodeproto.ReadThroughResult {
		return nodeproto.ReadThroughResult{Refusal: err.Error(), RefusalCode: refusalCodeOf(err)}
	}
	if agent.workers.ThroughReader == nil {
		return refuse(errors.New("this node does not read videos through"))
	}
	videoPath, err := agent.verifiedSourcePath(command.Video)
	if err != nil {
		return refuse(err)
	}
	findings, err := agent.workers.ThroughReader.Read(ctx, videoPath, command.LoudnessTracks, command.VideoTimeline,
		time.Duration(command.DurationMs)*time.Millisecond)
	if stall := stallOf(err); stall != nil {
		return nodeproto.ReadThroughResult{Stall: stall}
	}
	if err != nil {
		return nodeproto.ReadThroughResult{Failure: err.Error()}
	}
	return nodeproto.ReadThroughResult{Loudness: findings.Loudness, VideoTimeline: findings.VideoTimeline}
}
