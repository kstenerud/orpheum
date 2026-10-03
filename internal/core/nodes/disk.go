// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
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
	RootStalled(ctx context.Context, nodeID int64, rootPath string) (since time.Time, err error)
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
	since, err := disks.RootStalled(ctx, attempt.NodeID, file.RootPath)
	if err != nil {
		return true, fmt.Errorf("make root %s unavailable after a stall on its disk: %w", file.RootPath, err)
	}
	if job.PreviousAttemptToolFailed || attempt.Work.List != nil || attempt.Work.Examine != nil {
		return false, nil
	}
	stalled := StallError(attempt.NodeID, fmt.Sprintf("%s job %d", attempt.Kind, attempt.JobID), *stall)
	return true, jobs.ParkOnMissingRoot(attempt.NodeID, file.RootPath, since, jobs.MarkStalled(stalled))
}
