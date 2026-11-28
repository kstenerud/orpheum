// SPDX-License-Identifier: AGPL-3.0-or-later

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
	case readsWholeFiles(work) || work.MakeChunks != nil:
		return tierSlow
	}
	return tierFast
}

// bumpFor makes room for work of the incoming tier on a node's slot or device, which uses picks the
// attempts of: of those held there in a lower tier, the lowest tier, then the lowest priority, and
// of those one its node holds waiting before one it runs, and the newest, losing the least, is
// bumped (D255 §2, D324 §4). Its node is told
// to stop and its work is thrown away; its job goes back to the head of its priority's queue at no
// cost of an attempt. It reports whether one was.
func (all *Exchanges) bumpFor(ctx context.Context, nodeID int64, incoming tier, uses func(held Attempt) bool) bool {
	return all.bumpWhere(ctx, nodeID, func(held Attempt) bool {
		return uses(held) && tierOf(held.Work, held.Priority) < incoming
	})
}

// bumpWhere bumps, of the attempts its node holds that bumpable picks, the one bumped first
// (isBumpedBefore): its node is told to stop and its job goes back to the head of its priority's
// queue at no cost. It reports whether one was; bumpFor and displaceForPlayedTrickplay choose by it.
func (all *Exchanges) bumpWhere(ctx context.Context, nodeID int64, bumpable func(held Attempt) bool) bool {
	all.mutex.Lock()
	var chosen *heldAttempt
	for _, held := range all.attempts {
		if held.NodeID != nodeID || held.isBeingBumped || !bumpable(held.Attempt) {
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
	all.bump(ctx, attempt, fmt.Errorf("work of a higher priority took its place on node %d", nodeID))
	return true
}

// isBackgroundTrickplay is whether a held attempt is a trickplay run no one plays the film of: below
// the priority a played film's trickplay is told its node (priorityOnNode), which interest gives a
// film looked at or played alike.
func isBackgroundTrickplay(held Attempt) bool {
	return held.Work.MakeTrickplay != nil && held.Priority < priorityOnNode(held.Work, jobs.PriorityPlayNow)
}

// displaceForPlayedTrickplay makes way on its node for a played film's trickplay run, once its
// attempt is kept (D573): every trickplay run held there that no one raised, waiting or running, is
// bumped, so the played film's scrub-bar pictures are made first. Both stay background work on the
// node, at the idle scheduling class with background reads (priorityOnNode): only which film's
// pictures come first changes. A bumped run's job goes back to the head of its queue, and is given
// again to wait behind the played one; a run stopped loses at most the sheet it was making.
func (all *Exchanges) displaceForPlayedTrickplay(ctx context.Context, nodeID int64) {
	for all.bumpWhere(ctx, nodeID, isBackgroundTrickplay) {
	}
}

// bump stops an attempt a bump chose, marked isBeingBumped: its node is told to stop and its work is
// thrown away, and its job goes back to the head of its priority's queue at no cost, why.
func (all *Exchanges) bump(ctx context.Context, attempt Attempt, why error) {
	assignee, _ := all.assigneeOf(attempt.Kind)
	all.stop(ctx, attempt)
	if err := all.resolve(ctx, attempt, assignee, func(jobs.Job) error { return jobs.Bump(why) }); err != nil {
		all.logger.WarnContext(ctx, "could not bump a job", "jobId", attempt.JobID, "why", why, "err", err)
	}
	all.logger.InfoContext(ctx, "a job was bumped", "jobId", attempt.JobID, "kind", attempt.Kind,
		"nodeId", attempt.NodeID, "why", why)
}

// isBumpedBefore is whether one held attempt is bumped before another: the lower tier, then the
// lower priority, as the background's below the shown-interest rung's (D324 §4), then the one its
// node holds waiting, then the newer, and of two sent at once the later job.
func isBumpedBefore(one, other heldAttempt) bool {
	oneTier, otherTier := tierOf(one.Work, one.Priority), tierOf(other.Work, other.Priority)
	if oneTier != otherTier {
		return oneTier < otherTier
	}
	if one.Priority != other.Priority {
		return one.Priority < other.Priority
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
