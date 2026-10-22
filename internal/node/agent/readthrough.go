// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"errors"
	"time"

	"github.com/kstenerud/orpheum/internal/node/container"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/readthrough"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/readonly"
)

// ThroughReader reads a video through once and learns what the core asked of it: the loudness of
// its audio tracks, the timeline of its picture, or both. The real one is readthrough.Reader.
type ThroughReader interface {
	Read(
		ctx context.Context, film execx.Film, loudnessTracks []nodeproto.LoudnessTrack,
		timeline *nodeproto.VideoTimelineAsk, duration time.Duration, onProgress func(readThroughMs int64),
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
			var lastSaidAt time.Time
			sayRead := func(readThroughMs int64) {
				if now := agent.clock.Now(); now.Sub(lastSaidAt) >= agent.progressEvery() {
					lastSaidAt = now
					running := held.progress(nodeproto.ProgressRunning)
					running.ReadThroughMs = readThroughMs
					agent.sayProgress(ctx, nodeID, running)
				}
			}
			answer := agent.runReadThrough(jobCtx, command, sayRead)
			return nodeproto.Result{ReadThrough: &answer}, answer.Refusal + answer.Failure + stallDetail(answer.Stall)
		},
	})
}

// runReadThrough reads a video checked to be the one the core named (D100) through, or says why
// not: that FFmpeg could not, which is the file's, or that the node did not try.
func (agent *Agent) runReadThrough(
	ctx context.Context, command nodeproto.ReadThrough, sayRead func(readThroughMs int64),
) nodeproto.ReadThroughResult {
	refuse := func(err error) nodeproto.ReadThroughResult {
		return nodeproto.ReadThroughResult{Refusal: err.Error(), RefusalCode: refusalCodeOf(err)}
	}
	if agent.workers.ThroughReader == nil {
		return refuse(errors.New("this node does not read videos through"))
	}
	if command.VideoTimeline != nil && command.VideoTimeline.IsFromIndexOnly {
		keyframes, err := agent.indexedKeyframes(ctx, command)
		switch stall := stallOf(err); {
		case stall != nil:
			return nodeproto.ReadThroughResult{Stall: stall}
		case errors.Is(err, container.ErrNoIndex):
			return nodeproto.ReadThroughResult{HasNoIndex: true}
		case err != nil:
			return refuse(err)
		}
		return nodeproto.ReadThroughResult{VideoTimeline: &nodeproto.VideoTimeline{KeyframesMs: keyframes}}
	}
	film, closeFilm, err := agent.openFilm(ctx, command.Video)
	if stall := stallOf(err); stall != nil {
		return nodeproto.ReadThroughResult{Stall: stall}
	}
	if err != nil {
		return refuse(err)
	}
	defer closeFilm()
	findings, err := agent.workers.ThroughReader.Read(ctx, film, command.LoudnessTracks, command.VideoTimeline,
		time.Duration(command.DurationMs)*time.Millisecond, sayRead)
	if stall := stallOf(err); stall != nil {
		return nodeproto.ReadThroughResult{Stall: stall}
	}
	if err != nil {
		return nodeproto.ReadThroughResult{Failure: err.Error()}
	}
	return nodeproto.ReadThroughResult{Loudness: findings.Loudness, VideoTimeline: findings.VideoTimeline}
}

// indexedKeyframes are the picture's keyframes from the file's own index, for a read-through that
// asks for them from the index alone, as a player waiting on its film's grid does (D259 §2): read in
// a fraction of a second, without reading the film, and without its bytes in each second, which only
// a read through finds. A file with no index the reader follows, Matroska's cues or an MP4's sync
// samples, is container.ErrNoIndex: its film is cut on time (D269 §2). A file that cannot be opened
// or read is the error that says so, a refusal tried again.
func (agent *Agent) indexedKeyframes(ctx context.Context, command nodeproto.ReadThrough) ([]int64, error) {
	file, facts, err := agent.openNamed(ctx, command.Video)
	if err != nil {
		return nil, err
	}
	defer readonly.Close(file)
	return container.KeyframesMs(file, facts.Size(), command.VideoTimeline.VideoStreamIndex)
}
