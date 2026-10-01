// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// silentAfter is node_silent_after_seconds' starting value, which the tests' settings keep.
const silentAfter = 30 * time.Second

func TestNodeConnection_IsConnectedWhileItPollsOrWasHeardLatelyAndSilentOtherwise(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, 200*time.Millisecond)
	ctx := t.Context()
	if connection, _ := harness.service.NodeConnection(ctx, 1); connection != AwaitingHello {
		t.Errorf("a node that has not said hello is %s, want %s", connection, AwaitingHello)
	}
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	heardAt := harness.clock.Now()
	if connection, lastHeard := harness.service.NodeConnection(ctx, reply.NodeID); connection != Connected ||
		!lastHeard.Equal(heardAt) {
		t.Errorf("just after its hello: %s, last heard %v; want connected, heard then", connection, lastHeard)
	}
	harness.clock.Advance(silentAfter)
	if connection, lastHeard := harness.service.NodeConnection(ctx, reply.NodeID); connection != Silent ||
		!lastHeard.Equal(heardAt) || harness.service.IsConnected(ctx, reply.NodeID) {
		t.Errorf("silent for node_silent_after_seconds: %s, last heard %v; want silent, heard at its hello",
			connection, lastHeard)
	}
	if _, found := harness.service.ConnectedNodeID(ctx); found {
		t.Error("a silent node was offered as the connected one")
	}
	polled := make(chan struct{})
	go func() {
		defer close(polled)
		harness.poll(t, reply.NodeID, 0) // held for the fixture's poll hold
	}()
	// The poll is a real request: wait for it to open, as the clock the service reads stands still.
	deadline := time.Now().Add(5 * time.Second) //nolint:forbidigo // a real request, not the fake clock
	for !harness.service.IsConnected(ctx, reply.NodeID) {
		if time.Now().After(deadline) { //nolint:forbidigo // as above
			t.Fatal("the long-poll never opened")
		}
		time.Sleep(time.Millisecond)
	}
	harness.clock.Advance(silentAfter)
	if !harness.service.IsConnected(ctx, reply.NodeID) {
		t.Error("a node with a long-poll open is not connected")
	}
	testutil.Receive(t, polled, "the long-poll to end")
	harness.clock.Advance(silentAfter)
	if harness.service.IsConnected(ctx, reply.NodeID) {
		t.Error("the node is still connected node_silent_after_seconds after its poll ended")
	}
}

func TestSend_ACommandToANodeNotConnectedParksItsJobAndSendsNothing(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	ctx := t.Context()
	scan := nodeproto.Command{Type: nodeproto.CommandScan, Scan: &nodeproto.Scan{RootPath: "/media"}}
	err := harness.service.Send(ctx, 7, scan)
	parking, isParked := jobqueue.ParkingOf(err)
	if !isParked || parking.Destination != jobs.NodeDestination(7) || !parking.AwaitsSignal ||
		parking.Reason != "not connected" || !errors.Is(err, ErrNodeSilent) {
		t.Errorf("a command to a node that never said hello: %v (%+v); want its job parked on it until its hello",
			err, parking)
	}
	settingstest.Set(t, harness.settings, settings.ShouldScanOnNodeHello, "false")
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	heardAt := harness.clock.Now()
	if err := harness.service.Send(ctx, reply.NodeID, scan); err != nil {
		t.Fatalf("a command to a connected node: %v", err)
	}
	if commands, status := harness.poll(t, reply.NodeID, 0); status != http.StatusOK || len(commands.Commands) != 1 {
		t.Fatalf("the connected node was sent %+v (%d), want the one command", commands, status)
	}
	harness.clock.Advance(silentAfter + time.Second)
	parking, isParked = jobqueue.ParkingOf(harness.service.Send(ctx, reply.NodeID, scan))
	if !isParked || parking.Destination != jobs.NodeDestination(reply.NodeID) || !parking.Since.Equal(heardAt) {
		t.Errorf("a command to a silent node parks %+v (%v); want it parked since the node was last heard",
			parking, isParked)
	}
	if commands, _ := harness.poll(t, reply.NodeID, 1); len(commands.Commands) != 0 {
		t.Errorf("a silent node was queued %+v; a parked job sends nothing", commands.Commands)
	}
}

func TestUnanswered_ParksOnANodeThatWentAwayAndIsTheJobsOwnFailureOtherwise(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	ctx := t.Context()
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	quiet := errors.New("node said nothing about job 3 for 5m0s")
	if err := harness.service.Unanswered(ctx, reply.NodeID, quiet); !errors.Is(err, quiet) {
		t.Errorf("a connected node that said nothing of a job: %v, want the job's own failure", err)
	}
	if _, isParked := jobqueue.ParkingOf(harness.service.Unanswered(ctx, reply.NodeID, quiet)); isParked {
		t.Error("a connected node's silence about one job parked the node")
	}
	harness.clock.Advance(silentAfter)
	parking, isParked := jobqueue.ParkingOf(harness.service.Unanswered(ctx, reply.NodeID, quiet))
	if !isParked || parking.Destination != jobs.NodeDestination(reply.NodeID) {
		t.Errorf("a node gone silent: parks %+v (%v), want the job parked on the node", parking, isParked)
	}
}

func TestHello_BringsBackTheWorkParkedOnTheNodeAndOnAnyNode(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	want := []jobs.Destination{jobs.NodeDestination(reply.NodeID), jobs.DestinationAnyNode}
	if unparked := harness.destinations.all(); !slices.Equal(unparked, want) {
		t.Errorf("a hello unparked %q, want %q (D231 §6: a node's hello is its probe)", unparked, want)
	}
}

func TestStatus_ShowingRoomBringsBackThePreparesParkedOnThePreparedDirectory(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	report := func(seq int64, diskUsedPercent int) {
		t.Helper()
		usage := nodeproto.PreparedUsage{DiskUsedPercent: diskUsedPercent, DiskUsedLimitPercent: 90}
		status := []nodeproto.Record{{
			ReportSeq: seq, Type: nodeproto.RecordStatus,
			Status: &nodeproto.Status{Activity: "idle", PreparedUsage: &usage},
		}}
		if response := harness.post(t, nodeproto.PathReport, reply.NodeID, jsonl(t, status)); response.status !=
			http.StatusNoContent {
			t.Fatalf("the report answered %d %s", response.status, response.body)
		}
	}
	prepared := jobs.PreparedDirectoryDestination(reply.NodeID)
	report(1, 95)
	if slices.Contains(harness.destinations.all(), prepared) {
		t.Error("a status past the disk's limit unparked the prepared directory")
	}
	report(2, 80)
	if !slices.Contains(harness.destinations.all(), prepared) {
		t.Error("a status with room did not unpark the prepared directory (D231 §5)")
	}
}

func TestRemoveNode_ARetiredNodeIsToldToSayHelloAgainAndItsNameEnrolsANewNode(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, 0)
	ctx := t.Context()
	retired := harness.hello(t, nodeproto.Hello{Name: "local"}).NodeID
	if err := harness.service.RemoveNode(ctx, retired); err != nil {
		t.Fatal(err)
	}
	if connection, _ := harness.service.NodeConnection(ctx, retired); connection != AwaitingHello ||
		harness.service.IsConnected(ctx, retired) {
		t.Errorf("a retired node is %s, want %s and not connected", connection, AwaitingHello)
	}
	if _, status := harness.poll(t, retired, 0); status != http.StatusConflict {
		t.Errorf("a retired node's poll answered %d, want %d: say hello first", status, http.StatusConflict)
	}
	if enrolled := harness.hello(t, nodeproto.Hello{Name: "local"}).NodeID; enrolled == retired {
		t.Errorf("its name said hello again as node %d, the retired one; want a new node", enrolled)
	}
}
