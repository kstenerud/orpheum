// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/kstenerud/orpheum/internal/node/namedfile"
	"github.com/kstenerud/orpheum/internal/node/prepare"
	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/readonly"
)

// Preparer makes a rendition; prepare.Preparer is the real one.
type Preparer interface {
	Prepare(
		ctx context.Context, sourcePath string, command nodeproto.Prepare, onProgress func(preparedThroughMs int64),
	) (prepare.Made, error)
}

// prepareWork is a prepare held until a slot of its kind is free, which reports its progress and
// its end to the core. The source is checked to be the file the core named before FFmpeg reads it
// (D100).
func (agent *Agent) prepareWork(ctx context.Context, nodeID int64, held attempt, command nodeproto.Prepare) heldWork {
	jobID := held.jobID
	return heldWork{saysHowFar: true, run: func(jobCtx context.Context) {
		outcome := agent.runPrepare(jobCtx, nodeID, held, command)
		if jobCtx.Err() != nil && ctx.Err() == nil {
			agent.logger.InfoContext(ctx, "a prepare was cancelled", "jobId", jobID)
			return
		}
		switch outcome.State {
		case nodeproto.PrepareFailed:
			agent.logger.WarnContext(ctx, "a prepare failed",
				"jobId", jobID, "renditionDir", command.RenditionDir, "failure", outcome.Failure)
		case nodeproto.PrepareRefused:
			agent.logger.WarnContext(ctx, "a prepare was refused",
				"jobId", jobID, "renditionDir", command.RenditionDir, "refusal", outcome.Refusal)
		case nodeproto.PrepareStalled:
			agent.logger.WarnContext(ctx, "a prepare stalled and was stopped",
				"jobId", jobID, "renditionDir", command.RenditionDir, "stall", outcome.Stall.Detail)
		case nodeproto.PrepareDone:
		}
		agent.postResult(ctx, nodeID, "a prepare", held.named(nodeproto.Result{Prepare: &outcome}))
		agent.evict(ctx)
	}}
}

// evict keeps the prepared directory within its budget after a prepare has added to it (D217 §5).
func (agent *Agent) evict(ctx context.Context) {
	if agent.workers.PreparedShelf == nil {
		return
	}
	if _, err := agent.workers.PreparedShelf.Evict(ctx); err != nil {
		agent.logger.WarnContext(ctx, "could not keep the prepared directory within its budget", "err", err)
	}
}

// runPrepare runs one prepare to its end and says how it ended: done, FFmpeg failing on the
// source, or refused without trying, which says nothing of the source's bytes.
func (agent *Agent) runPrepare(
	ctx context.Context, nodeID int64, held attempt, command nodeproto.Prepare,
) nodeproto.PrepareReport {
	refused := func(err error) nodeproto.PrepareReport {
		return nodeproto.PrepareReport{
			State: nodeproto.PrepareRefused, Refusal: err.Error(), RefusalCode: refusalCodeOf(err),
		}
	}
	if agent.workers.Preparer == nil {
		return refused(errors.New("this node does not prepare"))
	}
	sourcePath, err := agent.verifiedSourcePath(command.Video)
	if err != nil {
		return refused(err)
	}
	var lastProgressSentAt time.Time
	onProgress := func(preparedThroughMs int64) {
		if now := agent.clock.Now(); now.Sub(lastProgressSentAt) >= agent.workers.ProgressEvery {
			lastProgressSentAt = now
			running := held.progress(nodeproto.ProgressRunning)
			running.PreparedThroughMs = preparedThroughMs
			agent.sayProgress(ctx, nodeID, running)
		}
	}
	made, err := agent.workers.Preparer.Prepare(ctx, sourcePath, command, onProgress)
	if errors.Is(err, prepared.ErrNoRoom) {
		noRoom := refused(err)
		noRoom.RefusalCode = nodeproto.RefusedNoRoom
		return noRoom
	}
	if stall := stallOf(err); stall != nil {
		return nodeproto.PrepareReport{State: nodeproto.PrepareStalled, Stall: stall}
	}
	if err != nil {
		return nodeproto.PrepareReport{State: nodeproto.PrepareFailed, Failure: err.Error()}
	}
	return nodeproto.PrepareReport{
		State: nodeproto.PrepareDone, SizeBytes: made.SizeBytes, VideoEncoder: made.VideoEncoder,
		RealtimeFactor: made.RealtimeFactor,
	}
}

// refusalCodeOf names a refusal the core says apart: a file changed since the scan.
func refusalCodeOf(err error) nodeproto.RefusalCode {
	if errors.Is(err, namedfile.ErrChanged) {
		return nodeproto.RefusedChanged
	}
	return ""
}

// verifiedSourcePath is the absolute path of the file source names, once it is under one of the
// node's roots and its payload hash is the one named.
func (agent *Agent) verifiedSourcePath(source nodeproto.BytesFile) (string, error) {
	file, err := agent.openNamed(source)
	if err != nil {
		return "", err
	}
	readonly.Close(file)
	return agent.roots[source.RootPath].Path() + "/" + string(source.Path), nil
}

// openNamed opens the regular file named, under one of the node's roots, once its payload hash is
// the one named (D100). The caller closes it.
func (agent *Agent) openNamed(named nodeproto.BytesFile) (*os.File, error) {
	file, _, err := namedfile.OpenNamed(agent.roots, named)
	if err != nil {
		return nil, fmt.Errorf("open the file named: %w", err)
	}
	return file, nil
}
