// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
	"log/slog"
	"sync"
)

// answerRouter hands a node's answers to the job that is waiting for them, by job id.
type answerRouter[Answer any] struct {
	logger     *slog.Logger
	answerName string // "a prepare report", for the log

	mutex        sync.Mutex
	answersByJob map[int64]chan Answer // guarded by mutex
}

func newAnswerRouter[Answer any](logger *slog.Logger, answerName string) *answerRouter[Answer] {
	return &answerRouter[Answer]{logger: logger, answerName: answerName, answersByJob: map[int64]chan Answer{}}
}

// await starts waiting for job's answers; forget ends it.
func (all *answerRouter[Answer]) await(jobID int64) <-chan Answer {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	answers := make(chan Answer, 8)
	all.answersByJob[jobID] = answers
	return answers
}

func (all *answerRouter[Answer]) forget(jobID int64) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	delete(all.answersByJob, jobID)
}

// deliver hands an answer to the job waiting for it. One nothing waits for, or one that finds the
// job's answers unread, is dropped: progress is only ever the latest, and the end is sent once, to
// a job that is reading.
func (all *answerRouter[Answer]) deliver(ctx context.Context, nodeID, jobID int64, answer Answer) {
	all.mutex.Lock()
	answers, found := all.answersByJob[jobID]
	all.mutex.Unlock()
	if !found {
		all.logger.InfoContext(ctx, "a node's answer came for no waiting job",
			"answerKind", all.answerName, "nodeId", nodeID, "jobId", jobID)
		return
	}
	select {
	case answers <- answer:
	default:
		all.logger.DebugContext(ctx, "a node's answer was dropped: its job is behind",
			"answerKind", all.answerName, "jobId", jobID)
	}
}
