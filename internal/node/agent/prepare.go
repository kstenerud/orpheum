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
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Preparer makes a rendition; prepare.Preparer is the real one.
type Preparer interface {
	Prepare(
		ctx context.Context, input string, command nodeproto.Prepare, progress func(preparedMs int64),
	) (int64, error)
}

// progressEvery is how often a running prepare tells the core how far it is.
const progressEvery = 2 * time.Second

// slots are how many prepares run at once on this node (cut-a §5.3): one encode, and two
// remuxes, which are bound by the disk rather than the processor; and one subtitle extraction,
// which reads a whole video.
type slots struct {
	encode  chan struct{}
	remux   chan struct{}
	extract chan struct{}
}

func newSlots() slots {
	return slots{encode: make(chan struct{}, 1), remux: make(chan struct{}, 2), extract: make(chan struct{}, 1)}
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
	jobCtx, done := agent.working.start(ctx, command.Job)
	go func() {
		defer done()
		slot := agent.slots.of(command.Profile)
		select {
		case slot <- struct{}{}:
		case <-jobCtx.Done():
			return
		}
		defer func() { <-slot }()
		report := agent.runPrepare(jobCtx, nodeID, command)
		if jobCtx.Err() != nil && ctx.Err() == nil {
			agent.logger.InfoContext(ctx, "a prepare was cancelled", "job", command.Job)
			return
		}
		if report.State == nodeproto.PrepareFailed {
			agent.logger.WarnContext(ctx, "a prepare failed",
				"job", command.Job, "renditionDir", command.RenditionDir, "why", report.Error)
		}
		if err := agent.sendPrepare(ctx, nodeID, report); err != nil && ctx.Err() == nil {
			agent.logger.WarnContext(ctx, "could not tell the core a prepare ended", "job", command.Job, "err", err)
		}
	}()
}

// runPrepare runs one prepare to its end and says how it ended.
func (agent *Agent) runPrepare(
	ctx context.Context, nodeID int64, command nodeproto.Prepare,
) nodeproto.PrepareReport {
	failed := func(err error) nodeproto.PrepareReport {
		return nodeproto.PrepareReport{Job: command.Job, State: nodeproto.PrepareFailed, Error: err.Error()}
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
				Job: command.Job, State: nodeproto.PrepareRunning, PreparedMs: preparedMs,
			}
			if sendErr := agent.sendPrepare(ctx, nodeID, running); sendErr != nil && ctx.Err() == nil {
				agent.logger.DebugContext(ctx, "could not send a prepare's progress",
					"job", command.Job, "err", sendErr)
			}
		}
	}
	size, err := agent.workers.Preparer.Prepare(ctx, input, command, progress)
	if err != nil {
		return failed(err)
	}
	return nodeproto.PrepareReport{Job: command.Job, State: nodeproto.PrepareDone, SizeBytes: size}
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
	if hex.EncodeToString(identity.Hash[:]) != source.Hash {
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
