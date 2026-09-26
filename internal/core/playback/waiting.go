// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
	"log/slog"
	"sync"
)

// waiting hands a node's answers to the job that is waiting for them, by job id.
type waiting[Answer any] struct {
	logger     *slog.Logger
	answerName string // "a prepare report", for the log

	mutex sync.Mutex
	jobs  map[int64]chan Answer // guarded by mutex
}

func newWaiting[Answer any](logger *slog.Logger, answerName string) *waiting[Answer] {
	return &waiting[Answer]{logger: logger, answerName: answerName, jobs: map[int64]chan Answer{}}
}

// await starts waiting for job's answers; forget ends it.
func (all *waiting[Answer]) await(job int64) <-chan Answer {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	answers := make(chan Answer, 8)
	all.jobs[job] = answers
	return answers
}

func (all *waiting[Answer]) forget(job int64) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	delete(all.jobs, job)
}

// deliver hands an answer to the job waiting for it. One nothing waits for, or one that finds the
// job's answers unread, is dropped: progress is only ever the latest, and the end is sent once, to
// a job that is reading.
func (all *waiting[Answer]) deliver(ctx context.Context, nodeID, job int64, answer Answer) {
	all.mutex.Lock()
	answers, found := all.jobs[job]
	all.mutex.Unlock()
	if !found {
		all.logger.InfoContext(ctx, "a node's answer came for no waiting job",
			"answer", all.answerName, "nodeId", nodeID, "job", job)
		return
	}
	select {
	case answers <- answer:
	default:
		all.logger.DebugContext(ctx, "a node's answer was dropped: its job is behind",
			"answer", all.answerName, "job", job)
	}
}
