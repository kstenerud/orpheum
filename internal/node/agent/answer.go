// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// answerRetry paces the tries of a result the core did not take: the first wait, and the longest one
// the doubling stops at.
type answerRetry struct {
	firstWait, longestWait time.Duration
}

func defaultAnswerRetry() answerRetry {
	return answerRetry{firstWait: time.Second, longestWait: 30 * time.Second}
}

// endedAttempts are the attempts whose work ended on this node, by token: those whose result the core
// has not yet acknowledged, and, for endedWorkKept after it did, when it did, so a command the core
// sends again for an attempt that ended is not run again (D238 §2). Held in memory, as the work is
// (D238 §3).
type endedAttempts struct {
	mutex          sync.Mutex
	unacknowledged map[string]int64     // guarded by mutex: each one's job, by token
	acknowledgedAt map[string]time.Time // guarded by mutex
	// helloSaid is closed at each hello, when every unacknowledged result is sent again at once.
	helloSaid chan struct{} // guarded by mutex
}

func newEndedAttempts() *endedAttempts {
	return &endedAttempts{
		unacknowledged: map[string]int64{}, acknowledgedAt: map[string]time.Time{}, helloSaid: make(chan struct{}),
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
	wait := agent.answerRetry.firstWait
	for {
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
		wait = min(2*wait, agent.answerRetry.longestWait)
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
