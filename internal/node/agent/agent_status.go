// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync/atomic"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
	"github.com/kstenerud/orpheum/internal/platform/strictjson"
)

// postChanges posts what the watcher saw change while the session lasts, and those it held while
// there was none. A notice the core does not take is not sent again: the root's periodic scan finds
// what it named (nodeproto.PathChanged).
func (agent *Agent) postChanges(ctx context.Context, nodeID int64) {
	for {
		select {
		case <-ctx.Done():
			return
		case change := <-agent.workers.Watcher.Changes():
			encoded, err := json.Marshal(change)
			if err == nil {
				var answer struct{}
				err = agent.exchange(ctx, http.MethodPost, nodeproto.PathChanged, nodeID, bytes.NewReader(encoded),
					&answer)
			}
			if err != nil && ctx.Err() == nil {
				agent.logger.WarnContext(ctx, "the core did not take a change the watcher saw; the root's periodic "+
					"scan finds it", "rootPath", change.RootPath, "path", change.Path, "err", err)
			}
		}
	}
}

// postStatus posts the node's status, now (D234 §6).
func (agent *Agent) postStatus(ctx context.Context, nodeID int64) error {
	encoded, err := json.Marshal(agent.status(ctx))
	if err != nil {
		return fmt.Errorf("encode the status: %w", err)
	}
	var answer struct{}
	return agent.exchange(ctx, http.MethodPost, nodeproto.PathStatus, nodeID, bytes.NewReader(encoded), &answer)
}

// status is what the node is doing and holds, and its disks, now.
func (agent *Agent) status(ctx context.Context) nodeproto.Status {
	disks := agent.disks(ctx)
	selfUse := agent.selfUse(ctx)
	agent.mutex.Lock()
	defer agent.mutex.Unlock()
	takenCommandSeq, held := takenThenHeld(&agent.takenCommandSeq, agent.held)
	status := nodeproto.Status{
		SentAt: agent.clock.Now().UnixMilli(), Disks: disks, Held: held, TakenCommandSeq: takenCommandSeq,
		SelfUse: selfUse,
	}
	if agent.workers.Stuck != nil {
		status.Stuck = agent.workers.Stuck()
	}
	status.Stuck = append(status.Stuck, agent.stuckReads.all()...)
	status.Stuck = append(status.Stuck, stuckOfGate(agent.diskGate.Stuck(), status.Stuck)...)
	if agent.workers.Watcher != nil {
		status.Watches = agent.workers.Watcher.States()
	}
	if agent.workers.PreparedShelf != nil {
		if usage, known := agent.workers.PreparedShelf.LastUsage(); known {
			status.PreparedUsage = &nodeproto.PreparedUsage{
				UsedBytes: usage.UsedBytes, BudgetBytes: usage.BudgetBytes, DiskUsedPercent: usage.DiskUsedPercent,
				DiskUsedLimitPercent: usage.DiskUsedLimitPercent, DiskAvailableBytes: usage.DiskAvailableBytes,
				DiskMinFreeBytes: usage.DiskMinFreeBytes,
			}
		}
	}
	return status
}

// takenThenHeld is the last command the node took, then the work it holds, read in that order. A
// command is held before it counts as taken, so every command the status says was taken is among
// what it says is held, or among what has ended. Read the other way round, a command taken between
// the two reads is said taken and not held, and the core judges the work lost and gives it again
// later (D234 §6): a scan lost so at a server's start delayed identifying a library by minutes
// (TestM2_EveryFileHasAFiling…, under check-pr's load, 2026-10-08).
func takenThenHeld(
	takenCommandSeq *atomic.Int64, held func() []nodeproto.HeldAttempt,
) (int64, []nodeproto.HeldAttempt) {
	taken := takenCommandSeq.Load()
	return taken, held()
}

// held is every attempt the node holds, and every one whose result the core has not acknowledged,
// for the core to reconcile with what it gave the node (D234 §6).
func (agent *Agent) held() []nodeproto.HeldAttempt {
	states := agent.work.states()
	held := make([]nodeproto.HeldAttempt, 0, len(states))
	for _, each := range states {
		state := nodeproto.HeldWaiting
		switch {
		case each.isPaused:
			state = nodeproto.HeldPaused
		case each.isRunning:
			state = nodeproto.HeldRunning
		}
		held = append(held, nodeproto.HeldAttempt{
			JobID: each.jobID, AttemptToken: each.token, State: state, SinceMovedMs: each.sinceMoved.Milliseconds(),
		})
	}
	return append(held, agent.ended.awaitingAcknowledgement()...)
}

// exchange makes one request and decodes its JSON answer.
func (agent *Agent) exchange(ctx context.Context, method, path string, nodeID int64, body io.Reader, into any) error {
	request, err := http.NewRequestWithContext(ctx, method, coreURL+path, body)
	if err != nil {
		return fmt.Errorf("build %s %s: %w", method, path, err)
	}
	if nodeID != 0 {
		request.Header.Set(nodeHeader, strconv.FormatInt(nodeID, 10))
	}
	response, err := agent.client.Do(request)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // read-only, and decoded below
	if response.StatusCode != http.StatusOK {
		text, _ := io.ReadAll(io.LimitReader(response.Body, outbound.BodyExcerptBytes)) //nolint:errcheck // best effort
		return fmt.Errorf("%s %s: %s: %s", method, path, response.Status, bytes.TrimSpace(text))
	}
	if err := strictjson.Decode(response.Body, into); err != nil {
		return fmt.Errorf("%s %s: the answer is not JSON: %w", method, path, err)
	}
	return nil
}

// selfUse is what the node uses of its machine now, for its status; nil when it cannot say.
func (agent *Agent) selfUse(ctx context.Context) *nodeproto.SelfUse {
	if agent.workers.ReadSelfUse == nil {
		return nil
	}
	sample, err := agent.workers.ReadSelfUse()
	if err != nil {
		agent.selfUseFailing.Do(func() {
			agent.logger.WarnContext(ctx, "cannot read what the node uses of its machine; its status says "+
				"nothing of it", "err", err)
		})
		return nil
	}
	return &nodeproto.SelfUse{
		Scope: string(sample.Scope), CPUMicros: sample.CPUMicros, HeldBytes: sample.HeldBytes,
		CacheBytes: sample.CacheBytes, PeakBytes: sample.PeakBytes,
	}
}
