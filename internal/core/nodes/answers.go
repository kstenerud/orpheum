// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"context"
	"log/slog"
	"sync"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// AnswerRouter hands a node's answers to the job that is waiting for them, by the job's id: the
// one way a job that asked a node for work waits for what the node says (cut-a §5.3).
type AnswerRouter[Answer any] struct {
	logger     *slog.Logger
	answerName string // "a prepare report", for the log

	mutex        sync.Mutex
	answersByJob map[int64]chan Answer // guarded by mutex
}

// NewAnswerRouter returns a router for answers of one kind, named for the log by answerName.
func NewAnswerRouter[Answer any](logger *slog.Logger, answerName string) *AnswerRouter[Answer] {
	return &AnswerRouter[Answer]{logger: logger, answerName: answerName, answersByJob: map[int64]chan Answer{}}
}

// Await starts waiting for a job's answers; Forget ends it.
func (all *AnswerRouter[Answer]) Await(jobID int64) <-chan Answer {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	answers := make(chan Answer, 8)
	all.answersByJob[jobID] = answers
	return answers
}

// Forget ends the wait Await began.
func (all *AnswerRouter[Answer]) Forget(jobID int64) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	delete(all.answersByJob, jobID)
}

// Deliver hands an answer to the job waiting for it. One nothing waits for, or one that finds the
// job's answers unread, is dropped: progress is only ever the latest, and the end is sent once, to
// a job that is reading.
func (all *AnswerRouter[Answer]) Deliver(ctx context.Context, nodeID, jobID int64, answer Answer) {
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

// CancelCommand tells a node to stop a job it is working on (cut-a §5.3): a job whose wait for its
// answer is cancelled sends it, so the node stops the work too.
func CancelCommand(jobID int64) nodeproto.Command {
	return nodeproto.Command{Type: nodeproto.CommandCancel, Cancel: &nodeproto.Cancel{JobID: jobID}}
}
