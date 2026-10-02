// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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

// progressEvery is how often a running prepare tells the core how far it is.
const progressEvery = 2 * time.Second

// slotForProfile is the slot a prepare runs in: a remux's, or an encode's.
func slotForProfile(profile nodeproto.PrepareProfile) slotKind {
	if profile == nodeproto.ProfileRemux {
		return slotRemux
	}
	return slotEncode
}

// prepareWork is a prepare held until a slot of its kind is free, which reports its progress and
// its end to the core. The source is checked to be the file the core named before FFmpeg reads it
// (D100).
func (agent *Agent) prepareWork(ctx context.Context, nodeID int64, held attempt, command nodeproto.Prepare) heldWork {
	jobID := held.jobID
	return heldWork{slot: slotForProfile(command.Profile), saysWaiting: true, run: func(jobCtx context.Context) {
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
		case nodeproto.PrepareWaiting, nodeproto.PrepareRunning, nodeproto.PrepareDone:
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
	if _, err := agent.workers.PreparedShelf.Evict(); err != nil {
		agent.logger.WarnContext(ctx, "could not keep the prepared directory within its budget", "err", err)
	}
}

// sayWaiting tells the core a prepare is waiting for a slot.
func (agent *Agent) sayWaiting(ctx context.Context, nodeID int64, held attempt) {
	waiting := nodeproto.PrepareReport{State: nodeproto.PrepareWaiting}
	if err := agent.sendPrepareReport(ctx, nodeID, held, waiting); err != nil && ctx.Err() == nil {
		agent.logger.WarnContext(ctx, "could not tell the core a prepare is waiting", "jobId", held.jobID, "err", err)
	}
}

// runPrepare runs one prepare to its end and says how it ended: done, FFmpeg failing on the
// source, or refused without trying, which says nothing of the source's bytes.
func (agent *Agent) runPrepare(
	ctx context.Context, nodeID int64, held attempt, command nodeproto.Prepare,
) nodeproto.PrepareReport {
	refused := func(err error) nodeproto.PrepareReport {
		return nodeproto.PrepareReport{State: nodeproto.PrepareRefused, Refusal: err.Error()}
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
		if now := agent.clock.Now(); now.Sub(lastProgressSentAt) >= progressEvery {
			lastProgressSentAt = now
			running := nodeproto.PrepareReport{
				State: nodeproto.PrepareRunning, PreparedThroughMs: preparedThroughMs,
			}
			if sendErr := agent.sendPrepareReport(ctx, nodeID, held, running); sendErr != nil && ctx.Err() == nil {
				agent.logger.DebugContext(ctx, "could not send a prepare's progress",
					"jobId", held.jobID, "err", sendErr)
			}
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

// sendPrepareReport posts a prepare's progress, or that it waits for a slot, once: the newest
// supersedes it, so a lost one is not tried again.
func (agent *Agent) sendPrepareReport(
	ctx context.Context, nodeID int64, held attempt, report nodeproto.PrepareReport,
) error {
	encoded, err := json.Marshal(held.named(nodeproto.Result{Prepare: &report}))
	if err != nil {
		return fmt.Errorf("encode a prepare report: %w", err)
	}
	var accepted struct{}
	return agent.exchange(ctx, http.MethodPost, nodeproto.PathResult, nodeID, bytes.NewReader(encoded), &accepted)
}
