// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/kstenerud/orpheum/internal/node/hash"
	"github.com/kstenerud/orpheum/internal/node/prepare"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Preparer makes a rendition; prepare.Preparer is the real one.
type Preparer interface {
	Prepare(
		ctx context.Context, input string, command nodeproto.Prepare, progress func(preparedMs int64),
	) (prepare.Made, error)
}

// progressEvery is how often a running prepare tells the core how far it is.
const progressEvery = 2 * time.Second

// slots are how many prepares run at once on this node (cut-a §5.3): one encode, and two
// remuxes, which are bound by the disk rather than the processor; one subtitle extraction and one
// loudness measurement, each of which reads a whole video.
type slots struct {
	encode  chan struct{}
	remux   chan struct{}
	extract chan struct{}
	measure chan struct{}
}

func newSlots() slots {
	return slots{
		encode: make(chan struct{}, nodeproto.EncodeSlots), remux: make(chan struct{}, nodeproto.RemuxSlots),
		extract: make(chan struct{}, 1), measure: make(chan struct{}, 1),
	}
}

func (all slots) of(profile nodeproto.PrepareProfile) chan struct{} {
	if profile == nodeproto.ProfileRemux {
		return all.remux
	}
	return all.encode
}

// prepare starts a prepare beside the node's other work, once a slot of its kind is free, and
// reports its progress and its end to the core. The source is checked to be the file the core
// named before FFmpeg reads it (D100).
func (agent *Agent) prepare(ctx context.Context, nodeID int64, command nodeproto.Prepare) {
	jobCtx, done := agent.working.start(ctx, command.JobID)
	go func() {
		defer done()
		if !agent.takeSlot(jobCtx, nodeID, command) {
			return
		}
		defer func() { <-agent.slots.of(command.Profile) }()
		report := agent.runPrepare(jobCtx, nodeID, command)
		if jobCtx.Err() != nil && ctx.Err() == nil {
			agent.logger.InfoContext(ctx, "a prepare was cancelled", "jobId", command.JobID)
			return
		}
		if report.State == nodeproto.PrepareFailed {
			agent.logger.WarnContext(ctx, "a prepare failed",
				"jobId", command.JobID, "renditionDir", command.RenditionDir, "why", report.Failure)
		}
		if err := agent.sendPrepare(ctx, nodeID, report); err != nil && ctx.Err() == nil {
			agent.logger.WarnContext(ctx, "could not tell the core a prepare ended", "jobId", command.JobID, "err", err)
		}
		agent.evict(ctx)
	}()
}

// evict keeps the prepared directory within its budget after a prepare has added to it (D217 §5).
func (agent *Agent) evict(ctx context.Context) {
	if agent.workers.Keeper == nil {
		return
	}
	if _, err := agent.workers.Keeper.Evict(); err != nil {
		agent.logger.WarnContext(ctx, "could not keep the prepared directory within its budget", "err", err)
	}
}

// takeSlot takes a slot of the prepare's kind, and reports false when the job ended first. A
// prepare that has to wait says so at once and at each status tick while it waits (sayWaiting),
// so the core keeps waiting for it (cut-a §5.3).
func (agent *Agent) takeSlot(ctx context.Context, nodeID int64, command nodeproto.Prepare) bool {
	slot := agent.slots.of(command.Profile)
	select {
	case slot <- struct{}{}:
		return true
	default:
	}
	agent.working.wait(command.JobID, true)
	defer agent.working.wait(command.JobID, false)
	agent.sayWaiting(ctx, nodeID, command.JobID)
	select {
	case slot <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

// sayWaiting tells the core a prepare is waiting for a slot.
func (agent *Agent) sayWaiting(ctx context.Context, nodeID, job int64) {
	waiting := nodeproto.PrepareReport{JobID: job, State: nodeproto.PrepareWaiting}
	if err := agent.sendPrepare(ctx, nodeID, waiting); err != nil && ctx.Err() == nil {
		agent.logger.WarnContext(ctx, "could not tell the core a prepare is waiting", "jobId", job, "err", err)
	}
}

// runPrepare runs one prepare to its end and says how it ended.
func (agent *Agent) runPrepare(
	ctx context.Context, nodeID int64, command nodeproto.Prepare,
) nodeproto.PrepareReport {
	failed := func(err error) nodeproto.PrepareReport {
		return nodeproto.PrepareReport{JobID: command.JobID, State: nodeproto.PrepareFailed, Failure: err.Error()}
	}
	if agent.workers.Preparer == nil {
		return failed(errors.New("this node does not prepare"))
	}
	input, err := agent.verifiedSource(command.Source)
	if err != nil {
		return failed(err)
	}
	var last time.Time
	progress := func(preparedMs int64) {
		if now := agent.clock.Now(); now.Sub(last) >= progressEvery {
			last = now
			running := nodeproto.PrepareReport{
				JobID: command.JobID, State: nodeproto.PrepareRunning, PreparedThroughMs: preparedMs,
			}
			if sendErr := agent.sendPrepare(ctx, nodeID, running); sendErr != nil && ctx.Err() == nil {
				agent.logger.DebugContext(ctx, "could not send a prepare's progress",
					"jobId", command.JobID, "err", sendErr)
			}
		}
	}
	made, err := agent.workers.Preparer.Prepare(ctx, input, command, progress)
	if err != nil {
		return failed(err)
	}
	return nodeproto.PrepareReport{
		JobID: command.JobID, State: nodeproto.PrepareDone, SizeBytes: made.SizeBytes, VideoEncoder: made.VideoEncoder,
		RealtimeFactor: made.Speed,
	}
}

// verifiedSource is the absolute path of the file source names, once it is under one of the
// node's roots and its payload hash is the one named.
func (agent *Agent) verifiedSource(source nodeproto.BytesFile) (string, error) {
	root, known := agent.roots[source.Root]
	if !known {
		return "", errors.New("the source is under none of this node's roots")
	}
	relative := string(source.Path)
	file, err := root.Open(relative)
	if err != nil {
		return "", fmt.Errorf("open the source: %w", err)
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // read-only
	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("stat the source: %w", err)
	}
	identity, err := hash.Payload(file, info.Size())
	if err != nil {
		return "", fmt.Errorf("hash the source: %w", err)
	}
	if hex.EncodeToString(identity.Hash[:]) != source.PayloadHash {
		return "", errors.New("the file at this path is not the one named; it changed since the scan")
	}
	return root.Path() + "/" + relative, nil
}

func (agent *Agent) sendPrepare(ctx context.Context, nodeID int64, report nodeproto.PrepareReport) error {
	encoded, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("encode a prepare report: %w", err)
	}
	var accepted struct{}
	return agent.exchange(ctx, http.MethodPost, nodeproto.PathPrepare, nodeID, bytes.NewReader(encoded), &accepted)
}
