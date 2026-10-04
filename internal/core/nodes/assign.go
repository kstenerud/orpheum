// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// Attempt is one sending of a job's work to a node: what its node's word on it is resolved by.
type Attempt struct {
	Token  string
	JobID  int64
	Kind   jobs.Kind
	NodeID int64
	// Work is the work as sent, without the bytes it carried.
	Work   nodeproto.Work
	SentAt time.Time
}

// Resolver takes an assigned job's outcome as a handler's answer: jobs.Runner.
type Resolver interface {
	ResolveBy(ctx context.Context, id int64, decide func(job jobs.Job) error) (bool, error)
}

// Assignee is how one kind of job given to a node is resolved: its runner, which takes the outcome
// as a handler's answer, and the answers its handler would give (D234 §7).
type Assignee struct {
	Runner Resolver
	// Answer is the job's answer to the node's word on how its work ended, recorded by then.
	Answer func(ctx context.Context, job jobs.Job, attempt Attempt, result nodeproto.Result) error
	// Unanswered is the job's answer when its node said nothing in time, or lost the work by
	// starting again: err says which.
	Unanswered func(ctx context.Context, job jobs.Job, attempt Attempt, err error) error
	// Progress, when set, takes the node's word on where the work is; an error ends the job with it,
	// and the node is told to stop.
	Progress func(ctx context.Context, attempt Attempt, progress nodeproto.Progress) error
	// Cancelled, when set, is told of a job cancelled while its node held its work.
	Cancelled func(ctx context.Context, attempt Attempt) error
}

// heldAttempt is an attempt a node holds, and when the core last heard of it.
type heldAttempt struct {
	Attempt
	lastHeard time.Time
	// lastWord is where its node last said it is: waiting, running or paused; empty before it said.
	lastWord nodeproto.ProgressState
}

// Handle names how one kind of job given to a node is resolved. Set each before the channel serves.
func (all *Exchanges) Handle(kind jobs.Kind, assignee Assignee) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	all.assignees[kind] = assignee
}

// Assign gives a job's work to a node, which says later how it ended, and is the handler's answer:
// the job is assigned until then, or until assigned_job_limit_hours have passed (D234 §7). The work
// is named to the node by an attempt token, kept with the job's id in cache.db, and sent once the
// job is recorded assigned, so the node's word resolves the job however soon it comes, and after a
// restart. A node that is not connected parks the job, and nothing is sent.
func (all *Exchanges) Assign(ctx context.Context, nodeID int64, job jobs.Job, command nodeproto.Command) error {
	limit, err := all.settings.Duration(ctx, settings.AssignedJobLimitHours)
	if err != nil {
		return fmt.Errorf("read assigned_job_limit_hours: %w", err)
	}
	if notReady := all.sender.Ready(ctx, nodeID); notReady != nil {
		return notReady
	}
	if full := all.windowFull(ctx, nodeID, command.Work, job.Priority >= jobs.PriorityPlayNow); full != nil {
		return full
	}
	token, err := all.newToken()
	if err != nil {
		return err
	}
	command.JobID, command.AttemptToken, command.Priority = job.ID, token, job.Priority
	// Play-driven work pauses background work on its device (D240 §6).
	command.IsPlayDriven = job.Priority >= jobs.PriorityPlayNow
	now := all.clock.Now()
	attempt := Attempt{
		Token: token, JobID: job.ID, Kind: job.Kind, NodeID: nodeID, Work: command.WithoutBytes(), SentAt: now,
	}
	if err := all.keep(ctx, attempt); err != nil {
		return err
	}
	return jobs.AssignThen(now.Add(limit), func(context.Context) { all.sender.Enqueue(nodeID, command) })
}

// windowFull parks work a node has no room for in the core's window (D240 §5): as many jobs of the
// work's slot as the node runs at once, and as many reading its device as may read it, each with
// node_jobs_beyond_free_slots more, so the next starts without waiting for the core. Play-driven work is
// never held back for its device's readers: the node pauses the background work reading it (D240
// §6). It is nil while there is room, and for a node whose resources are not known.
func (all *Exchanges) windowFull(ctx context.Context, nodeID int64, work nodeproto.Work, isPlayDriven bool) error {
	resources, isConnected, err := all.rooms.Resources(ctx, nodeID)
	if err != nil || !isConnected {
		return err
	}
	extra, err := all.settings.Int(ctx, settings.NodeJobsBeyondFreeSlots)
	if err != nil {
		return fmt.Errorf("read node_jobs_beyond_free_slots: %w", err)
	}
	slot, device := work.Slot(), resources.deviceOf(work)
	var inSlot, onDevice int
	all.mutex.Lock()
	for _, held := range all.attempts {
		if held.NodeID != nodeID {
			continue
		}
		if slot != "" && held.Work.Slot() == slot {
			inSlot++
		}
		if device != "" && resources.deviceOf(held.Work) == device {
			onDevice++
		}
	}
	all.mutex.Unlock()
	full := func(destination jobs.Destination) error {
		return jobs.Park(jobs.Parking{Destination: destination, Reason: "busy", AwaitsSignal: true},
			fmt.Errorf("node %d has no room for more %s work: %w", nodeID, slot, errWindowFull))
	}
	switch {
	case slot != "" && inSlot >= resources.Capacity.SlotCount(slot)+int(extra):
		return full(jobs.SlotDestination(nodeID, string(slot)))
	case device != "" && !isPlayDriven && onDevice >= resources.readerLimit(device)+int(extra):
		return full(jobs.ReaderDestination(nodeID, device))
	}
	return nil
}

// errWindowFull is work a node has no room for in the core's window.
var errWindowFull = errors.New("its window is full")

// unparkRoomFor brings back the work waiting for room a node's attempt held: its slot's, and its
// device's while the node is connected (D240 §5).
func (all *Exchanges) unparkRoomFor(ctx context.Context, attempt Attempt) {
	if slot := attempt.Work.Slot(); slot != "" {
		all.rooms.Unpark(ctx, jobs.SlotDestination(attempt.NodeID, string(slot)))
	}
	if resources, isConnected, err := all.rooms.Resources(ctx, attempt.NodeID); err == nil && isConnected {
		if device := resources.deviceOf(attempt.Work); device != "" {
			all.rooms.Unpark(ctx, jobs.ReaderDestination(attempt.NodeID, device))
		}
	}
}

// keep records an attempt, in cache.db and in memory.
func (all *Exchanges) keep(ctx context.Context, attempt Attempt) error {
	workJSON, err := json.Marshal(attempt.Work)
	if err != nil {
		return fmt.Errorf("encode job %d's work: %w", attempt.JobID, err)
	}
	err = all.cache.Write(ctx, func(queries *cachegen.Queries) error {
		return queries.InsertNodeAttempt(ctx, cachegen.InsertNodeAttemptParams{
			AttemptToken: attempt.Token, JobID: attempt.JobID, JobKind: string(attempt.Kind), NodeID: attempt.NodeID,
			WorkJSON: string(workJSON), SentAt: attempt.SentAt.UnixMilli(),
		})
	})
	if err != nil {
		return fmt.Errorf("keep job %d's attempt: %w", attempt.JobID, err)
	}
	all.mutex.Lock()
	defer all.mutex.Unlock()
	all.attempts[attempt.Token] = &heldAttempt{Attempt: attempt, lastHeard: attempt.SentAt}
	return nil
}

// Reprioritize tells the node holding a job's attempt its new priority (D234 §3): past its "wanted
// until", play-driven work falls to background priority on the node too, and pauses nothing more.
func (all *Exchanges) Reprioritize(jobID, priority int64) {
	all.mutex.Lock()
	var held []Attempt
	for _, attempt := range all.attempts {
		if attempt.JobID == jobID {
			held = append(held, attempt.Attempt)
		}
	}
	all.mutex.Unlock()
	for _, attempt := range held {
		all.sender.Enqueue(attempt.NodeID, ReprioritizeCommand(attempt.JobID, attempt.Token, priority))
	}
}

// NodeWordOf is where the node that holds a job's attempt last said it is, waiting for a slot or a
// reader, running, or paused for play-driven work (D240 §6), and whether a node holds one and said.
func (all *Exchanges) NodeWordOf(jobID int64) (nodeproto.ProgressState, bool) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	for _, held := range all.attempts {
		if held.JobID == jobID && held.lastWord != "" {
			return held.lastWord, true
		}
	}
	return "", false
}

// forgetAttempt drops an attempt, resolved or given up.
func (all *Exchanges) forgetAttempt(ctx context.Context, token string) error {
	all.mutex.Lock()
	held, isHeld := all.attempts[token]
	delete(all.attempts, token)
	all.mutex.Unlock()
	if isHeld {
		defer all.unparkRoomFor(ctx, held.Attempt)
	}
	err := all.cache.Write(ctx, func(queries *cachegen.Queries) error {
		return queries.DeleteNodeAttempt(ctx, token)
	})
	if err != nil {
		return fmt.Errorf("forget attempt %s: %w", token, err)
	}
	return nil
}

// Load takes back the attempts cache.db kept through a restart: each, heard of now, is resolved by
// its node's word, or by its silence (D234 §7, S82).
func (all *Exchanges) Load(ctx context.Context) error {
	rows, err := all.cache.Read().ListNodeAttempts(ctx)
	if err != nil {
		return fmt.Errorf("list the attempts given to nodes: %w", err)
	}
	now := all.clock.Now()
	all.mutex.Lock()
	defer all.mutex.Unlock()
	for _, row := range rows {
		var work nodeproto.Work
		if err := json.Unmarshal([]byte(row.WorkJSON), &work); err != nil {
			return fmt.Errorf("read attempt %s's work: %w", row.AttemptToken, err)
		}
		all.attempts[row.AttemptToken] = &heldAttempt{
			Attempt: Attempt{
				Token: row.AttemptToken, JobID: row.JobID, Kind: jobs.Kind(row.JobKind), NodeID: row.NodeID, Work: work,
				SentAt: time.UnixMilli(row.SentAt),
			},
			lastHeard: now,
		}
	}
	return nil
}

// resolvable is an attempt a node holds, and how its kind is resolved.
type resolvable struct {
	Attempt
	assignee Assignee
}

// attemptOf is the attempt a token names, from a node, and whether it names one. An attempt whose
// kind no assignee resolves yet, before the server's runners start, is ErrNotResolvedYet.
func (all *Exchanges) attemptOf(nodeID int64, token string) (resolvable, bool, error) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	held, found := all.attempts[token]
	if !found || held.NodeID != nodeID {
		return resolvable{}, false, nil
	}
	assignee, isHandled := all.assignees[held.Kind]
	if !isHandled {
		return resolvable{}, true, fmt.Errorf("%s job %d: %w", held.Kind, held.JobID, ErrNotResolvedYet)
	}
	return resolvable{Attempt: held.Attempt, assignee: assignee}, true, nil
}

// ErrNotResolvedYet is a node's word on an attempt the core cannot resolve yet: its runners have not
// started. It leaves the result unacknowledged, for the node to send again.
var ErrNotResolvedYet = errors.New("the core does not resolve this kind of job yet")

// resolve settles an attempt's job with the answer decide gives it, and forgets the attempt. A job no
// longer assigned, cancelled or past its time, takes nothing.
func (all *Exchanges) resolve(
	ctx context.Context, attempt Attempt, assignee Assignee, decide func(job jobs.Job) error,
) error {
	_, err := assignee.Runner.ResolveBy(ctx, attempt.JobID, decide)
	return errors.Join(err, all.forgetAttempt(ctx, attempt.Token))
}

// resolveResult settles an attempt's job with its node's word on how the work ended.
func (all *Exchanges) resolveResult(
	ctx context.Context, attempt Attempt, assignee Assignee, result nodeproto.Result,
) error {
	return all.resolve(ctx, attempt, assignee, func(job jobs.Job) error {
		if result.Refusal != "" {
			// The node took no part of the command: it needs an update (D233 §9).
			return jobs.ParkOnNodeNeedingUpdate(attempt.NodeID,
				fmt.Errorf("it refused %s job %d: %s", attempt.Kind, attempt.JobID, result.Refusal))
		}
		if isAnswered, answer := all.parkOnStalledDisk(ctx, job, attempt, result); isAnswered {
			return answer
		}
		return assignee.Answer(ctx, job, attempt, result)
	})
}

// takeProgress hears a node's word on where an attempt is: the attempt is heard of, and its kind's
// progress taken; one its kind refuses ends the job, and the node is told to stop.
func (all *Exchanges) takeProgress(ctx context.Context, nodeID int64, progress nodeproto.Progress) bool {
	held, isHeld, err := all.attemptOf(nodeID, progress.AttemptToken)
	if !isHeld || err != nil {
		return isHeld // an attempt not resolved yet is heard of when its result comes
	}
	attempt, assignee := held.Attempt, held.assignee
	all.mutex.Lock()
	if held, isHeld := all.attempts[attempt.Token]; isHeld {
		held.lastHeard, held.lastWord = all.clock.Now(), progress.State
	}
	all.mutex.Unlock()
	if assignee.Progress == nil {
		return true
	}
	if refused := assignee.Progress(ctx, attempt, progress); refused != nil {
		all.stop(ctx, attempt)
		if err := all.resolve(ctx, attempt, assignee, func(jobs.Job) error { return refused }); err != nil {
			all.logger.WarnContext(ctx, "could not end a job its progress ended", "jobId", attempt.JobID, "err", err)
		}
	}
	return true
}

// stop tells an attempt's node to stop its work, or, when the node has not taken it yet, withdraws
// it, so it is never sent (S27).
func (all *Exchanges) stop(_ context.Context, attempt Attempt) {
	if all.sender.Withdraw(attempt.NodeID, attempt.Token) {
		return
	}
	all.sender.Enqueue(attempt.NodeID, CancelCommand(attempt.JobID, attempt.Token))
}

// unanswered settles each attempt match picks with its kind's answer to err, telling its node to stop
// when tellNode says so.
func (all *Exchanges) unanswered(
	ctx context.Context, match func(attempt heldAttempt) bool, err func(attempt Attempt) error, tellNode bool,
) {
	all.mutex.Lock()
	var picked []heldAttempt
	for _, held := range all.attempts {
		if match(*held) {
			picked = append(picked, *held)
		}
	}
	all.mutex.Unlock()
	for _, held := range picked {
		attempt := held.Attempt
		assignee, isHandled := all.assigneeOf(attempt.Kind)
		if !isHandled {
			continue
		}
		if tellNode {
			all.stop(ctx, attempt)
		}
		why := err(attempt)
		resolveErr := all.resolve(ctx, attempt, assignee, func(job jobs.Job) error {
			return assignee.Unanswered(ctx, job, attempt, why)
		})
		if resolveErr != nil {
			all.logger.WarnContext(ctx, "could not end a job its node did not answer", "jobId", attempt.JobID,
				"err", resolveErr)
		}
	}
}

func (all *Exchanges) assigneeOf(kind jobs.Kind) (Assignee, bool) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	assignee, found := all.assignees[kind]
	return assignee, found
}

// Watch settles, at each tick until ctx ends, every attempt the core has not heard of for the silence
// limit: its node went away, which parks the job, or it is the job's own failure (D234 §6). A node
// says where each attempt it holds is at every status tick, so only a silence ends one.
func (all *Exchanges) Watch(ctx context.Context, tick <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			now := all.clock.Now()
			all.unanswered(ctx, func(held heldAttempt) bool { return now.Sub(held.lastHeard) >= all.maxSilence },
				func(attempt Attempt) error {
					silent := fmt.Errorf("node %d said nothing about %s job %d for %s: %w", attempt.NodeID,
						attempt.Kind, attempt.JobID, all.maxSilence, ErrNodeSilent)
					return all.sender.Unanswered(ctx, attempt.NodeID, silent)
				}, true)
		}
	}
}

// attemptsOfJob are the attempts at a job's work its nodes hold.
func (all *Exchanges) attemptsOfJob(jobID int64) []Attempt {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	var held []Attempt
	for _, each := range all.attempts {
		if each.JobID == jobID {
			held = append(held, each.Attempt)
		}
	}
	return held
}

// Overdue tells the node holding the work of a job whose time ran out (assigned_job_limit_hours)
// to stop it, and forgets the attempt: the queue has ended it, failed or to be tried again, and a
// word from the node on it after is a word on work no job waits for (G4, D244 §6). The queue's
// overdue hook (jobs.Queue.OnOverdue).
func (all *Exchanges) Overdue(ctx context.Context, jobID int64) {
	for _, attempt := range all.attemptsOfJob(jobID) {
		all.stop(ctx, attempt)
		if err := all.forgetAttempt(ctx, attempt.Token); err != nil {
			all.logger.WarnContext(ctx, "could not forget an overdue job's work", "jobId", jobID, "err", err)
		}
	}
}

// Cancelled tells the node holding a cancelled job's work to stop it, and its kind that it was
// cancelled (S91): the queue's cancel hook (jobs.Queue.OnCancel).
func (all *Exchanges) Cancelled(ctx context.Context, jobID int64) {
	for _, attempt := range all.attemptsOfJob(jobID) {
		all.stop(ctx, attempt)
		assignee, isHandled := all.assigneeOf(attempt.Kind)
		var err error
		if isHandled && assignee.Cancelled != nil {
			err = assignee.Cancelled(ctx, attempt)
		}
		if err = errors.Join(err, all.forgetAttempt(ctx, attempt.Token)); err != nil {
			all.logger.WarnContext(ctx, "could not end a cancelled job's work", "jobId", jobID, "err", err)
		}
	}
}
