// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/pathtree"
)

// ErrStalledOnDisk is work that stalled waiting on its disk (D233 §10).
var ErrStalledOnDisk = errors.New("its disk did not answer")

// UnreadableOnDisk begins the failure of work whose disk stalled on it twice: once, which made its
// root unavailable until a scan read it again, and again after. For a file it is the file's own
// failure, and its root stays available (D233 §10).
const UnreadableOnDisk = "its disk could not be read: "

// DiskStalls is what the core does when work stalls waiting on its disk: the root it reads is made
// unavailable, so what is under it is said to be so, and a scan of it is asked for, which brings it
// back once it reads it again (D233 §10, D241): library.Scans.
type DiskStalls interface {
	// MarkStalled makes the deepest place the scans know under rootPath that holds waitingOn
	// unavailable, and says which and since when (D328 §5).
	MarkStalled(
		ctx context.Context, nodeID int64, rootPath, waitingOn string,
	) (place string, since time.Time, err error)
}

// SetDiskStalls names what the core does when work stalls on its disk. Set it before the channel
// serves; without it a disk stall is a stall like any other.
func (all *Exchanges) SetDiskStalls(disks DiskStalls) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	all.disks = disks
}

// StallError is what the node said of work, what, that it stopped as stalled: ErrStalledOnDisk when
// it waited on its disk.
func StallError(nodeID int64, what string, stall nodeproto.Stall) error {
	stalled := fmt.Errorf("node %d stopped %s: %s at %d ms of its output, %d ms without moving, %s: %s",
		nodeID, what, stall.Cause, stall.LastPositionMs, stall.WithoutAdvanceMs, stall.Condition, stall.Detail)
	if stall.Condition == nodeproto.ConditionWaitingOnDisk {
		return fmt.Errorf("%w: %w", ErrStalledOnDisk, stalled)
	}
	return stalled
}

// parkOnStalledDisk answers work that stalled the first time waiting on its disk, when the work
// reads a root: the root is made unavailable and a scan asked for (DiskStalls), and the job parks on
// the root until that scan reads it, keeping that its tool failed, so the same stall again after is
// the file's own failure (StallOutcome). A listing or an examining is the scan itself, and is tried
// again rather than parked. It answers isAnswered false for any other result, which the job's kind
// answers.
func (all *Exchanges) parkOnStalledDisk(
	ctx context.Context, job jobs.Job, attempt Attempt, result nodeproto.Result,
) (isAnswered bool, answer error) {
	stall := result.Stall()
	file, readsMedia := attempt.Work.MediaFile()
	all.mutex.Lock()
	disks := all.disks
	all.mutex.Unlock()
	if stall == nil || stall.Condition != nodeproto.ConditionWaitingOnDisk || !readsMedia || disks == nil {
		return false, nil
	}
	if stall.WaitingOnPath != "" && !pathtree.IsAtOrUnder(stall.WaitingOnPath, file.RootPath) {
		return parkOnStalledPreparedDisk(attempt, *stall)
	}
	place, since, err := disks.MarkStalled(ctx, attempt.NodeID, file.RootPath, stall.WaitingOnPath)
	if err != nil {
		return true, fmt.Errorf("make %s unavailable after a stall on its disk: %w", file.RootPath, err)
	}
	if job.PreviousAttemptToolFailed || attempt.Work.List != nil || attempt.Work.Examine != nil {
		return false, nil
	}
	stalled := StallError(attempt.NodeID, fmt.Sprintf("%s job %d", attempt.Kind, attempt.JobID), *stall)
	return true, jobs.ParkOnMissingRoot(attempt.NodeID, place, since, jobs.MarkStalled(stalled))
}

// parkOnStalledPreparedDisk answers a prepare whose run waited on a disk that is not its source's,
// as its node said: the disk it writes, the prepared directory's. Its root stays available, and the
// node's prepares park on the prepared directory, a probe on the back-off finding when it answers
// again, at no cost of an attempt. A stall of other work outside its root is its kind's to answer.
func parkOnStalledPreparedDisk(attempt Attempt, stall nodeproto.Stall) (bool, error) {
	if attempt.Work.Prepare == nil {
		return false, nil
	}
	stalled := StallError(attempt.NodeID, fmt.Sprintf("%s job %d", attempt.Kind, attempt.JobID), stall)
	return true, jobs.Park(jobs.Parking{
		Destination: jobs.PreparedDirectoryDestination(attempt.NodeID),
		Reason:      "its prepared directory's disk did not answer",
	}, jobs.MarkStalled(stalled))
}

// FFmpegStalledTwice begins the failure kept for a target whose FFmpeg stalled on two attempts in a row.
const FFmpegStalledTwice = "FFmpeg stalled twice: "

// FFmpegStallOutcome is a handler's answer to a node's word that the FFmpeg working on job, what,
// stalled and was stopped: StallOutcome, with what the node saw.
func FFmpegStallOutcome(
	job jobs.Job, nodeID int64, what string, stall nodeproto.Stall, keepFailure func(failure string) error,
) error {
	return StallOutcome(job, StallError(nodeID, "the FFmpeg of "+what, stall), FFmpegStalledTwice, keepFailure)
}

// RefusalError is a node's refusal of work, what, that it did not try, which says nothing of the
// source and is tried again on the job's back-off. A file changed since the scan says so: the scan
// that sees the change ends the old file's jobs (S74).
func RefusalError(nodeID int64, what, refusal string, code nodeproto.RefusalCode) error {
	if code == nodeproto.RefusedChanged {
		return fmt.Errorf("node %d refused %s: its file changed since the scan, which the next scan "+
			"of its root ends the job for: %s", nodeID, what, refusal)
	}
	return fmt.Errorf("node %d refused %s: %s", nodeID, what, refusal)
}
