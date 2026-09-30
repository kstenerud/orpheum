// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// answerRetry paces the tries of an answer the core did not take: the first wait, the longest one
// the doubling stops at, and how long the waits may add up to before the answer is given up.
type answerRetry struct {
	firstWait, longestWait, patience time.Duration
}

// defaultAnswerRetry gives an answer at most nodeproto.ArtworkAnswerWait of waits: as long as the
// core waits for an artwork answer, and within the five minutes it waits to hear of a prepare,
// after which the core tries the job again and a later answer has nothing to go to.
func defaultAnswerRetry() answerRetry {
	return answerRetry{firstWait: time.Second, longestWait: 30 * time.Second, patience: nodeproto.ArtworkAnswerWait}
}

// postAnswer posts a job's answer to the core at path. A post that fails, the core busy or the
// connection broken, is tried again with a doubling wait until the waits reach the retry's
// patience or the session ends. An answer that never arrives is logged and costs only its own job,
// which the core tries again: it never ends the session, whose running work it would stop.
func (agent *Agent) postAnswer(ctx context.Context, nodeID int64, path, jobKind string, jobID int64, answer any) {
	encoded, err := json.Marshal(answer)
	if err != nil {
		agent.logger.ErrorContext(ctx, "could not encode a job's answer", "jobKind", jobKind, "jobId", jobID,
			"err", err)
		return
	}
	wait, waited := agent.answerRetry.firstWait, time.Duration(0)
	for {
		var accepted struct{}
		err = agent.exchange(ctx, http.MethodPost, path, nodeID, bytes.NewReader(encoded), &accepted)
		if err == nil || ctx.Err() != nil {
			return
		}
		if waited+wait > agent.answerRetry.patience {
			agent.logger.WarnContext(ctx, "gave up sending a job's answer to the core", "jobKind", jobKind,
				"jobId", jobID, "waited", waited.String(), "err", err)
			return
		}
		agent.logger.InfoContext(ctx, "could not send a job's answer to the core; trying again", "jobKind", jobKind,
			"jobId", jobID, "wait", wait.String(), "err", err)
		if !sleepUnlessDone(ctx, wait) {
			return
		}
		waited += wait
		wait = min(2*wait, agent.answerRetry.longestWait)
	}
}

// sleepUnlessDone waits for wait, and reports false when ctx ended first.
func sleepUnlessDone(ctx context.Context, wait time.Duration) bool {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
