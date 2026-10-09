// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// tier is how much a piece of work on a node matters, lowest first (D255 §1): an operation expected
// to be slow, one expected to be fast, and work a person asked for.
type tier int

const (
	tierSlow tier = iota
	tierFast
	tierPersons
)

// tierOf is work's tier: a person's, by its job's priority; else slow when it reads whole films or
// encodes them, and fast otherwise, as a scan's listing and examining are.
func tierOf(work nodeproto.Work, priority int64) tier {
	switch {
	case priority >= jobs.PriorityPersonsRequest:
		return tierPersons
	case readsWholeFiles(work) || work.Prepare != nil || work.MakeChunks != nil:
		return tierSlow
	}
	return tierFast
}

// bumpFor makes room for work of the incoming tier on a node's slot or device, which uses picks the
// attempts of: of those held there in a lower tier, the lowest, and of those one its node holds
// waiting before one it runs, and the newest, losing the least, is bumped (D255 §2). Its node is told
// to stop and its work is thrown away; its job goes back to the head of its priority's queue at no
// cost of an attempt. It reports whether one was.
func (all *Exchanges) bumpFor(ctx context.Context, nodeID int64, incoming tier, uses func(held Attempt) bool) bool {
	all.mutex.Lock()
	var chosen *heldAttempt
	for _, held := range all.attempts {
		if held.NodeID != nodeID || held.isBeingBumped || !uses(held.Attempt) ||
			tierOf(held.Work, held.Priority) >= incoming {
			continue
		}
		if _, isHandled := all.assignees[held.Kind]; !isHandled {
			continue
		}
		if chosen == nil || isBumpedBefore(*held, *chosen) {
			chosen = held
		}
	}
	if chosen == nil {
		all.mutex.Unlock()
		return false
	}
	chosen.isBeingBumped = true
	attempt := chosen.Attempt
	all.mutex.Unlock()
	assignee, _ := all.assigneeOf(attempt.Kind)
	all.stop(ctx, attempt)
	why := fmt.Errorf("work of a higher priority took its place on node %d", nodeID)
	if err := all.resolve(ctx, attempt, assignee, func(jobs.Job) error { return jobs.Bump(why) }); err != nil {
		all.logger.WarnContext(ctx, "could not bump a job for work of a higher priority", "jobId", attempt.JobID,
			"err", err)
	}
	all.logger.InfoContext(ctx, "a job was bumped for work of a higher priority", "jobId", attempt.JobID,
		"kind", attempt.Kind, "nodeId", nodeID)
	return true
}

// isBumpedBefore is whether one held attempt is bumped before another: the lower tier, then the one
// its node holds waiting, then the newer, and of two sent at once the later job.
func isBumpedBefore(one, other heldAttempt) bool {
	oneTier, otherTier := tierOf(one.Work, one.Priority), tierOf(other.Work, other.Priority)
	if oneTier != otherTier {
		return oneTier < otherTier
	}
	oneWaits, otherWaits := one.lastWord == nodeproto.ProgressWaiting, other.lastWord == nodeproto.ProgressWaiting
	if oneWaits != otherWaits {
		return oneWaits
	}
	if !one.SentAt.Equal(other.SentAt) {
		return one.SentAt.After(other.SentAt)
	}
	return one.JobID > other.JobID
}
