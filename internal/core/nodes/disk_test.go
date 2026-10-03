// SPDX-License-Identifier: GPL-3.0-or-later

package nodes_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// stalledRoots is the core's answer to a stall on a disk, noting each root made unavailable.
type stalledRoots struct {
	since time.Time
	roots []string
}

func (stalled *stalledRoots) RootStalled(_ context.Context, _ int64, rootPath string) (time.Time, error) {
	stalled.roots = append(stalled.roots, rootPath)
	return stalled.since, nil
}

// answersAfter is an assignee's runner whose jobs' last attempt's tool failed, or not, as told.
type answersAfter struct {
	answers
	isAfterToolFailure bool
}

func (resolved answersAfter) ResolveBy(_ context.Context, id int64, decide func(jobs.Job) error) (bool, error) {
	resolved.answers <- decide(jobs.Job{
		ID: id, Kind: jobs.KindPrepare, PreviousAttemptToolFailed: resolved.isAfterToolFailure,
	})
	return true, nil
}

// diskStall is a prepare's word that its FFmpeg stalled waiting on the disk.
func diskStall(sent nodeproto.Command) nodeproto.Result {
	return nodeproto.Result{
		JobID: 7, AttemptToken: sent.AttemptToken, Work: &sent.Work,
		Prepare: &nodeproto.PrepareReport{State: nodeproto.PrepareStalled, Stall: &nodeproto.Stall{
			Cause: nodeproto.StallStoodStill, Condition: nodeproto.ConditionWaitingOnDisk, IsLeftRunning: true,
		}},
	}
}

// Work that stalls waiting on its disk the first time makes the root it reads unavailable, which
// asks for a scan, and parks on the root as the tool's failure, until that scan reads it; after, the
// same stall is the job's kind's to answer, which makes it the target's failure (D233 §10).
func TestExchange_AStallOnTheDiskParksOnItsRootTheFirstTimeAndIsTheKindsToAnswerAfter(t *testing.T) {
	t.Parallel()
	for _, isAfterToolFailure := range []bool{false, true} {
		bench := newExchanges(t)
		disks := &stalledRoots{since: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)}
		bench.exchanges.SetDiskStalls(disks)
		resolved := answersAfter{answers: make(answers, 1), isAfterToolFailure: isAfterToolFailure}
		assignee := assignedPrepare(resolved.answers, nil)
		assignee.Runner = resolved
		bench.exchanges.Handle(jobs.KindPrepare, assignee)
		command := nodeproto.Command{Work: nodeproto.Work{Type: nodeproto.CommandPrepare, Prepare: &nodeproto.Prepare{
			Video: nodeproto.BytesFile{RootPath: "/media/films", Path: []byte("Wing.mkv")},
		}}}
		handed := bench.exchanges.Assign(t.Context(), 1, jobs.Job{ID: 7, Kind: jobs.KindPrepare}, command)
		jobqueue.Hand(t.Context(), handed)
		sent := testutil.Receive(t, bench.sender.sent, "the prepare command")
		deliver(t, bench.exchanges, 1, diskStall(sent))
		answer := testutil.Receive(t, resolved.answers, "job 7's answer")
		if len(disks.roots) != 1 || disks.roots[0] != "/media/films" {
			t.Errorf("roots made unavailable: %q; want the prepare's, /media/films", disks.roots)
		}
		parking, isParked := jobqueue.ParkingOf(answer)
		switch {
		case !isAfterToolFailure && (!isParked || parking.Destination != jobs.RootDestination(1, "/media/films") ||
			!parking.Since.Equal(disks.since) || !strings.Contains(answer.Error(), "stalled: ")):
			t.Errorf("a first stall on the disk was answered %v; want parked on its root as a stall", answer)
		case isAfterToolFailure && (isParked || answer == nil || answer.Error() != "the node said stalled"):
			t.Errorf("a stall on the disk after a tool failure was answered %v; want the kind's answer", answer)
		}
	}
}

// A second stall on the disk is the target's failure, its disk unreadable, not "stalled twice".
func TestStallOutcome_ASecondStallOnTheDiskIsTheTargetsUnreadableDisk(t *testing.T) {
	t.Parallel()
	stall := nodeproto.Stall{Cause: nodeproto.StallStoodStill, Condition: nodeproto.ConditionWaitingOnDisk}
	stalled := nodes.StallError(1, "the FFmpeg of prepare 3", stall)
	if !errors.Is(stalled, nodes.ErrStalledOnDisk) {
		t.Fatalf("a stall on the disk = %v; want ErrStalledOnDisk", stalled)
	}
	var kept string
	answer := nodes.StallOutcome(jobs.Job{PreviousAttemptToolFailed: true}, stalled, "FFmpeg stalled twice: ",
		func(failure string) error { kept = failure; return nil })
	if !strings.HasPrefix(kept, nodes.UnreadableOnDisk) || !strings.Contains(answer.Error(), nodes.UnreadableOnDisk) {
		t.Errorf("a second stall on the disk kept %q, answered %v; want its disk unreadable", kept, answer)
	}
	spinning := nodes.StallError(1, "the FFmpeg of prepare 3", nodeproto.Stall{Condition: nodeproto.ConditionSpinning})
	if errors.Is(spinning, nodes.ErrStalledOnDisk) {
		t.Errorf("a spinning stall = %v; want it not a stall on the disk", spinning)
	}
}
