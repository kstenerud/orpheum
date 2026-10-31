// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// stalledRoots is the core's answer to a stall on a disk, noting each root made unavailable and
// what the stall waited on; the place it makes unavailable is place, or the root when empty.
type stalledRoots struct {
	since      time.Time
	place      string
	roots      []string
	waitingOns []string
}

func (stalled *stalledRoots) MarkStalled(
	_ context.Context, _ int64, rootPath, waitingOn string,
) (string, time.Time, error) {
	stalled.roots = append(stalled.roots, rootPath)
	stalled.waitingOns = append(stalled.waitingOns, waitingOn)
	if stalled.place != "" {
		return stalled.place, stalled.since, nil
	}
	return rootPath, stalled.since, nil
}

// answersAfter is an assignee's runner whose jobs' last attempt's tool failed, or not, as told.
type answersAfter struct {
	answers
	isAfterToolFailure bool
}

func (resolved answersAfter) ResolveBy(_ context.Context, id int64, decide func(jobs.Job) error) (bool, error) {
	resolved.answers <- decide(jobs.Job{
		ID: id, Kind: jobs.KindMakeChunks, PreviousAttemptToolFailed: resolved.isAfterToolFailure,
	})
	return true, nil
}

// diskStall is a run's word that its FFmpeg stalled waiting on the disk.
func diskStall(sent nodeproto.Command) nodeproto.Result {
	return nodeproto.Result{
		JobID: 7, AttemptToken: sent.AttemptToken, Work: &sent.Work,
		MakeChunks: &nodeproto.MakeChunksReport{State: nodeproto.RunStalled, Stall: &nodeproto.Stall{
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
		assignee := assignedRun(resolved.answers, nil)
		assignee.Runner = resolved
		bench.exchanges.Handle(jobs.KindMakeChunks, assignee)
		command := nodeproto.Command{Work: nodeproto.Work{
			Type: nodeproto.CommandMakeChunks, MakeChunks: &nodeproto.MakeChunks{
				Video: nodeproto.BytesFile{RootPath: "/media/films", Path: []byte("Arkon.mkv")},
			},
		}}
		handed := bench.exchanges.Assign(t.Context(), 1, jobs.Job{ID: 7, Kind: jobs.KindMakeChunks}, command)
		jobqueue.HandOver(t.Context(), handed)
		sent := testutil.Receive(t, bench.sender.sent, "the run's command")
		deliver(t, bench.exchanges, 1, diskStall(sent))
		answer := testutil.Receive(t, resolved.answers, "job 7's answer")
		if len(disks.roots) != 1 || disks.roots[0] != "/media/films" {
			t.Errorf("roots made unavailable: %q; want the run's, /media/films", disks.roots)
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

// A stall waiting on a disk mounted inside the root tells the core what it waited on, and parks on
// the place the core made unavailable for it, the disk, which its coming back unparks (D328 §5).
func TestExchange_AStallOnADiskInsideTheRootParksOnThatDisk(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	disks := &stalledRoots{since: time.Date(2026, 10, 21, 22, 0, 0, 0, time.UTC), place: "/media/films/disk1"}
	bench.exchanges.SetDiskStalls(disks)
	resolved := answersAfter{answers: make(answers, 1)}
	assignee := assignedRun(resolved.answers, nil)
	assignee.Runner = resolved
	bench.exchanges.Handle(jobs.KindMakeChunks, assignee)
	command := nodeproto.Command{Work: nodeproto.Work{
		Type: nodeproto.CommandMakeChunks, MakeChunks: &nodeproto.MakeChunks{
			Video: nodeproto.BytesFile{RootPath: "/media/films", Path: []byte("disk1/Arkon.mkv")},
		},
	}}
	handed := bench.exchanges.Assign(t.Context(), 1, jobs.Job{ID: 7, Kind: jobs.KindMakeChunks}, command)
	jobqueue.HandOver(t.Context(), handed)
	sent := testutil.Receive(t, bench.sender.sent, "the run's command")
	stalled := diskStall(sent)
	stalled.MakeChunks.Stall.WaitingOnPath = "/media/films/disk1/Arkon.mkv"
	deliver(t, bench.exchanges, 1, stalled)
	answer := testutil.Receive(t, resolved.answers, "job 7's answer")
	parking, isParked := jobqueue.ParkingOf(answer)
	if !slices.Equal(disks.waitingOns, []string{"/media/films/disk1/Arkon.mkv"}) || !isParked ||
		parking.Destination != jobs.RootDestination(1, "/media/films/disk1") {
		t.Errorf("waited on %q, answered %v; want the disk's path told, and parked on the disk", disks.waitingOns,
			answer)
	}
}

// A second stall on the disk is the target's failure, its disk unreadable, not "stalled twice".
func TestStallOutcome_ASecondStallOnTheDiskIsTheTargetsUnreadableDisk(t *testing.T) {
	t.Parallel()
	stall := nodeproto.Stall{Cause: nodeproto.StallStoodStill, Condition: nodeproto.ConditionWaitingOnDisk}
	stalled := nodes.StallError(1, "the FFmpeg of read-through 3", stall)
	if !errors.Is(stalled, nodes.ErrStalledOnDisk) {
		t.Fatalf("a stall on the disk = %v; want ErrStalledOnDisk", stalled)
	}
	var kept string
	answer := nodes.StallOutcome(jobs.Job{PreviousAttemptToolFailed: true}, stalled, "FFmpeg stalled twice: ",
		func(failure string) error { kept = failure; return nil })
	if !strings.HasPrefix(kept, nodes.UnreadableOnDisk) || !strings.Contains(answer.Error(), nodes.UnreadableOnDisk) {
		t.Errorf("a second stall on the disk kept %q, answered %v; want its disk unreadable", kept, answer)
	}
	spinning := nodes.StallError(1, "the FFmpeg of read-through 3",
		nodeproto.Stall{Condition: nodeproto.ConditionSpinning})
	if errors.Is(spinning, nodes.ErrStalledOnDisk) {
		t.Errorf("a spinning stall = %v; want it not a stall on the disk", spinning)
	}
}

// A run of chunks whose FFmpeg waited on a disk that is not its source's, the prepared directory's as
// its node said, leaves its root available and is not parked on it: the stall is its kind's to
// answer, the media are not what failed (D344 §1).
func TestExchange_AStallOutsideTheRootLeavesTheRootAndIsTheKindsToAnswer(t *testing.T) {
	t.Parallel()
	bench := newExchanges(t)
	disks := &stalledRoots{since: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)}
	bench.exchanges.SetDiskStalls(disks)
	resolved := answersAfter{answers: make(answers, 1)}
	assignee := assignedRun(resolved.answers, nil)
	assignee.Runner = resolved
	bench.exchanges.Handle(jobs.KindMakeChunks, assignee)
	command := nodeproto.Command{Work: nodeproto.Work{
		Type: nodeproto.CommandMakeChunks, MakeChunks: &nodeproto.MakeChunks{
			Video: nodeproto.BytesFile{RootPath: "/media/films", Path: []byte("Arkon.mkv")},
		},
	}}
	assigned := bench.exchanges.Assign(t.Context(), 1, jobs.Job{ID: 7, Kind: jobs.KindMakeChunks}, command)
	jobqueue.HandOver(t.Context(), assigned)
	sent := testutil.Receive(t, bench.sender.sent, "the run's command")
	stalled := diskStall(sent)
	stalled.MakeChunks.Stall.WaitingOnPath = "/srv/node/cache/prepared/3/chunks-picture/a/00007.m4s"
	deliver(t, bench.exchanges, 1, stalled)
	answer := testutil.Receive(t, resolved.answers, "job 7's answer")
	if len(disks.roots) != 0 {
		t.Errorf("roots made unavailable: %q; want none, the source's disk answered", disks.roots)
	}
	if _, isParked := jobqueue.ParkingOf(answer); isParked || answer == nil ||
		answer.Error() != "the node said stalled" {
		t.Errorf("a stall outside the root was answered %v; want the kind's answer, parked nowhere", answer)
	}
}
