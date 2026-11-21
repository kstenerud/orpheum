// SPDX-License-Identifier: AGPL-3.0-or-later

package jobs

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/unexpected"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// IsPassingFailure is whether a job's failure is of a cause known to pass (D511 §1): a provider or
// a host not answering or failing on its side (ErrUnavailable, a rate limit, a 5xx), a timeout, or a
// node that did not say how its work ended. The queue never gives one up for good.
func IsPassingFailure(err error) bool {
	if errors.Is(err, outbound.ErrUnavailable) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, os.ErrDeadlineExceeded) || errors.Is(err, jobqueue.ErrOverdue) {
		return true
	}
	_, isLimited := errors.AsType[*outbound.RateLimitedError](err) //nolint:errcheck // AsType's second value is a bool
	if isLimited {
		return true
	}
	if failed, isFailed := errors.AsType[*outbound.StatusFailedError](err); isFailed && failed.IsServerFailure() {
		return true
	}
	netErr, isNet := errors.AsType[net.Error](err)
	return isNet && netErr.Timeout()
}

// theServer is the subject of a whole-server kind's job, which has no target.
const theServer = "the server"

// itemWord is a subject's first word when it names an item.
const itemWord = "item"

// targetSubject is one of a target's ids, and the word for it in a subject.
type targetSubject struct {
	word string
	id   int64
}

// targetSubjects are each of target's ids with its word, in the order Subject reads them.
func targetSubjects(target Target) []targetSubject {
	return []targetSubject{
		{itemWord, target.ItemID},
		{"stream", target.StreamID},
		{"fetched subtitle", target.FetchedSubtitleID},
		{"file", target.FileID},
		{"scan", target.ScanID},
		{"scan batch", target.ScanBatchID},
		{"chunk store", target.ChunkStoreID},
		{"season group", target.GroupID},
	}
}

// Subject is what a job on target works on, as its failure of unknown cause is kept: "item 12",
// "file 7", or "the server" for a whole-server kind's (D511 §2).
func Subject(target Target) string {
	for _, subject := range targetSubjects(target) {
		if subject.id != 0 {
			return subject.word + " " + strconv.FormatInt(subject.id, 10)
		}
	}
	return theServer
}

// ItemIDsOf are the items the subjects name, in order; a subject naming no item is passed over. A
// kind's Retry reads its subjects with it.
func ItemIDsOf(subjects []string) []int64 {
	ids := make([]int64, 0, len(subjects))
	for _, subject := range subjects {
		number, isItem := strings.CutPrefix(subject, itemWord+" ")
		if !isItem {
			continue
		}
		if id, err := strconv.ParseInt(number, 10, 64); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// unknownFailures has the queue's failures of unknown cause kept by the recorder, under the job's
// kind and Subject, and a job that succeeds take off the one kept for it.
type unknownFailures struct {
	recorder *unexpected.Recorder
	logger   interface {
		ErrorContext(ctx context.Context, msg string, args ...any)
	}
}

// Failed keeps a job's failure of unknown cause.
func (told unknownFailures) Failed(ctx context.Context, job jobqueue.Job[Target], failure string) {
	err := told.recorder.Record(ctx, unexpected.Failure{
		Kind: string(job.Kind), Subject: Subject(job.Target), Error: failure,
	})
	if err != nil {
		told.logger.ErrorContext(ctx, "could not keep a job's failure of unknown cause", "kind", job.Kind,
			"target", job.Target, "err", err)
	}
}

// Done takes off the failure kept for a job that succeeded.
func (told unknownFailures) Done(ctx context.Context, job jobqueue.Job[Target]) {
	if err := told.recorder.Resolved(ctx, string(job.Kind), Subject(job.Target)); err != nil {
		told.logger.ErrorContext(ctx, "could not take off a job's failure of unknown cause", "kind", job.Kind,
			"target", job.Target, "err", err)
	}
}

// isUnavailable is whether this start holds the work of kind on target unavailable for a failure of
// unknown cause: no request queues it again until the server starts again (D511 §3).
func (queue *Queue) isUnavailable(kind Kind, target Target) bool {
	return queue.unknownFailures != nil && queue.unknownFailures.IsUnavailable(string(kind), Subject(target))
}
