// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/backoff"
)

// answerRetry paces the tries of a result the core did not take: the first wait, and the longest one
// the doubling stops at, each moved at random by up to answerJitter of it either way, read from
// random, so that nodes a restarted core left waiting do not all try again in the same instant (the
// audit of 2026-10-25, G9). With no random, the waits are not moved.
type answerRetry struct {
	firstWait, longestWait time.Duration
	random                 io.Reader
}

// answerJitter is how far an answer's retry may move either way: the job queue's own retries' share
// (D231 §10), enough to spread nodes that failed together over a few seconds at the longest wait.
const answerJitter = 0.1

// waitAfter is the wait after tries failed tries, the first after one: doubled from the first wait
// each try, to the longest, and jittered.
func (retry answerRetry) waitAfter(tries int64) time.Duration {
	wait := backoff.Doubled(retry.firstWait, retry.longestWait, tries-1)
	if retry.random == nil {
		return wait
	}
	moved, err := backoff.Jittered(retry.random, wait, retry.longestWait, answerJitter)
	if err != nil {
		return wait // a retry still happens, unmoved
	}
	return moved
}

// defaultAnswerRetry tries a result again after a second, doubling to every 30 s: a core that did not
// take one is restarting or busy, which a second outlasts the most of the time, and 30 s keeps a
// core that is down for long from being asked more than twice a minute, well inside the silence
// limit (node_silent_after_seconds) its status ticks keep the node alive against. Internal bounds;
// the result is kept until taken, so they decide only how soon, never whether.
func defaultAnswerRetry() answerRetry {
	return answerRetry{firstWait: time.Second, longestWait: 30 * time.Second, random: rand.Reader}
}

// endedAttempts are the attempts whose work ended on this node, by token: those whose result the core
// has not yet acknowledged, and, for endedWorkKept after it did, when it did, so a command the core
// sends again for an attempt that ended is not run again (D238 §2). Held in memory, as the work is
// (D238 §3).
type endedAttempts struct {
	mutex          sync.Mutex
	unacknowledged map[string]int64     // guarded by mutex: each one's job, by token
	acknowledgedAt map[string]time.Time // guarded by mutex
	// cancelledAt is when a cancel came for each attempt the node did not hold yet: its work, sent
	// before it but held after, is never run (guarded by mutex).
	cancelledAt map[string]time.Time
	// helloSaid is closed at each hello, when every unacknowledged result is sent again at once.
	helloSaid chan struct{} // guarded by mutex
}

func newEndedAttempts() *endedAttempts {
	return &endedAttempts{
		unacknowledged: map[string]int64{}, acknowledgedAt: map[string]time.Time{}, helloSaid: make(chan struct{}),
		cancelledAt: map[string]time.Time{},
	}
}

// hasEnded is whether the attempt of token ended on this node, its result acknowledged or not.
func (ended *endedAttempts) hasEnded(token string) bool {
	ended.mutex.Lock()
	defer ended.mutex.Unlock()
	_, isAcknowledged := ended.acknowledgedAt[token]
	_, isUnacknowledged := ended.unacknowledged[token]
	return isUnacknowledged || isAcknowledged
}

// awaitAcknowledgement notes an attempt whose result is not yet acknowledged.
func (ended *endedAttempts) awaitAcknowledgement(held attempt) {
	ended.mutex.Lock()
	defer ended.mutex.Unlock()
	ended.unacknowledged[held.token] = held.jobID
}

// awaitingAcknowledgement are the attempts whose results the core has not yet acknowledged.
func (ended *endedAttempts) awaitingAcknowledgement() []nodeproto.HeldAttempt {
	ended.mutex.Lock()
	defer ended.mutex.Unlock()
	held := make([]nodeproto.HeldAttempt, 0, len(ended.unacknowledged))
	for token, jobID := range ended.unacknowledged {
		held = append(held, nodeproto.HeldAttempt{JobID: jobID, AttemptToken: token, State: nodeproto.HeldEnded})
	}
	return held
}

// acknowledge notes that the core acknowledged an attempt's result at now, and forgets the
// attempts acknowledged longer than kept ago.
func (ended *endedAttempts) acknowledge(token string, now time.Time, kept time.Duration) {
	ended.mutex.Lock()
	defer ended.mutex.Unlock()
	delete(ended.unacknowledged, token)
	ended.acknowledgedAt[token] = now
	for other, at := range ended.acknowledgedAt {
		if now.Sub(at) > kept {
			delete(ended.acknowledgedAt, other)
		}
	}
}

// cancelledUnheld notes a cancel for an attempt the node does not hold at now, and forgets those
// noted longer than kept ago.
func (ended *endedAttempts) cancelledUnheld(token string, now time.Time, kept time.Duration) {
	ended.mutex.Lock()
	defer ended.mutex.Unlock()
	ended.cancelledAt[token] = now
	for other, at := range ended.cancelledAt {
		if now.Sub(at) > kept {
			delete(ended.cancelledAt, other)
		}
	}
}

// wasCancelled is whether a cancel came for an attempt before the node held it.
func (ended *endedAttempts) wasCancelled(token string) bool {
	ended.mutex.Lock()
	defer ended.mutex.Unlock()
	_, isCancelled := ended.cancelledAt[token]
	return isCancelled
}

// nextHello is closed at the next hello.
func (ended *endedAttempts) nextHello() <-chan struct{} {
	ended.mutex.Lock()
	defer ended.mutex.Unlock()
	return ended.helloSaid
}

// saidHello has every unacknowledged result sent again at once.
func (ended *endedAttempts) saidHello() {
	ended.mutex.Lock()
	defer ended.mutex.Unlock()
	close(ended.helloSaid)
	ended.helloSaid = make(chan struct{})
}

// postResult notes that an attempt ended, and posts its result to the core beside the node's other
// work until the core acknowledges it, which it does once it has recorded it (D234 §5): a post that
// fails, the core busy, gone or restarting, is tried again with a doubling wait, and at once at the
// node's next hello, for as long as the node runs. It holds no slot, and never ends the session,
// whose running work it would stop.
func (agent *Agent) postResult(ctx context.Context, nodeID int64, jobKind string, result nodeproto.Result) {
	agent.ended.awaitAcknowledgement(attempt{jobID: result.JobID, token: result.AttemptToken})
	go agent.postUntilAcknowledged(ctx, nodeID, jobKind, result)
}

func (agent *Agent) postUntilAcknowledged(ctx context.Context, nodeID int64, jobKind string, result nodeproto.Result) {
	token := result.AttemptToken
	encoded, err := agent.encodeToPost(result)
	if err != nil {
		agent.logger.ErrorContext(ctx, "could not encode a job's answer", "jobKind", jobKind, "jobId", result.JobID,
			"err", err)
		return
	}
	defer agent.workers.HeldResults.forget(token)
	for tries := int64(1); ; tries++ {
		wait := agent.answerRetry.waitAfter(tries)
		// Taken before the post, so a hello said while the post fails still wakes the next try.
		nextHello := agent.ended.nextHello()
		var accepted struct{}
		err = agent.exchange(ctx, http.MethodPost, nodeproto.PathResult, nodeID, bytes.NewReader(encoded), &accepted)
		if err == nil {
			agent.ended.acknowledge(token, agent.clock.Now(), agent.workers.EndedWorkKept)
			return
		}
		if ctx.Err() != nil {
			return
		}
		agent.logger.InfoContext(ctx, "the core has not taken a job's answer; trying again", "jobKind", jobKind,
			"jobId", result.JobID, "wait", wait.String(), "err", err)
		if !sleepUntilHelloOrDone(ctx, wait, nextHello) {
			return
		}
	}
}

// encodeToPost is a result as it is posted: whole, or, when its JSON is larger than
// nodeproto.MaxPostedResultBytes, held for the core to fetch from the byte socket and posted as its
// job, attempt and work alone, naming what is held.
func (agent *Agent) encodeToPost(result nodeproto.Result) ([]byte, error) {
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode the result: %w", err)
	}
	if len(encoded) <= nodeproto.MaxPostedResultBytes {
		return encoded, nil
	}
	held := agent.workers.HeldResults.hold(result.AttemptToken, encoded)
	posted, err := json.Marshal(nodeproto.Result{
		JobID: result.JobID, AttemptToken: result.AttemptToken, Work: result.Work, Held: &held,
	})
	if err != nil {
		return nil, fmt.Errorf("encode the held result's name: %w", err)
	}
	return posted, nil
}

// sleepUntilHelloOrDone waits for wait, or until the next hello, and reports false when ctx ended
// first.
func sleepUntilHelloOrDone(ctx context.Context, wait time.Duration, hello <-chan struct{}) bool {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-hello:
		return true
	case <-ctx.Done():
		return false
	}
}
