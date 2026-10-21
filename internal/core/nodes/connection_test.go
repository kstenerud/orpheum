// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
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
	scan := nodeproto.Command{JobID: 3, Work: nodeproto.Work{
		Type: nodeproto.CommandList, List: &nodeproto.List{RootPath: "/media", Path: "."},
	}}
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
	if commands, _ := harness.pollThroughHold(t, reply.NodeID, 1); len(commands.Commands) != 0 {
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

// A hello brings back the work parked on the node and on any node (D231 §6: a node's hello is its
// probe), and the work waiting for room on its slots and devices, of which it may say more than
// before (D240 §5).
func TestHello_BringsBackTheWorkParkedOnTheNodeAndOnAnyNode(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	reply := harness.hello(t, nodeproto.Hello{Name: "local", Devices: nodeproto.Devices{{Key: "zfs:tank"}}})
	node := reply.NodeID
	want := []jobs.Destination{
		jobs.NodeDestination(node), jobs.DestinationAnyNode,
		jobs.SlotDestination(node, "encode"), jobs.SlotDestination(node, "remux"),
		jobs.SlotDestination(node, "extract"), jobs.SlotDestination(node, "read_through"),
		jobs.SlotDestination(node, "artwork"),
		jobs.ReaderDestination(node, "zfs:tank"),
	}
	if unparked := harness.destinations.all(); !slices.Equal(unparked, want) {
		t.Errorf("a hello unparked %q, want %q", unparked, want)
	}
}

func TestStatus_ShowingRoomBringsBackThePreparesParkedOnThePreparedDirectory(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	sayStatus := func(diskUsedPercent int, availableMiB int64) {
		t.Helper()
		usage := nodeproto.PreparedUsage{
			DiskUsedPercent: diskUsedPercent, DiskUsedLimitPercent: 90,
			DiskAvailableBytes: availableMiB << 20, DiskMinFreeBytes: 100 << 20,
		}
		harness.postStatus(t, reply.NodeID, nodeproto.Status{PreparedUsage: &usage})
	}
	prepared := jobs.PreparedDirectoryDestination(reply.NodeID)
	sayStatus(95, 500)
	if slices.Contains(harness.destinations.all(), prepared) {
		t.Error("a status past the disk's limit unparked the prepared directory")
	}
	sayStatus(80, 50)
	if slices.Contains(harness.destinations.all(), prepared) {
		t.Error("a status below the disk's floor of free space unparked the prepared directory")
	}
	sayStatus(80, 500)
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

// A node that says hello in another version of the node protocol is refused, shown as needing
// updating, and its work waits for it with that reason; its hello in this core's version brings it
// back (D233 §9, S89).
func TestHello_ANodeOnAnotherProtocolIsRefusedAndItsWorkWaitsUntilItIsUpdated(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	ctx := t.Context()
	settingstest.Set(t, harness.settings, settings.ShouldScanOnNodeHello, "false")
	connected := harness.hello(t, nodeproto.Hello{Name: "local"})
	older := nodeproto.Hello{Name: "local", ProtocolVersion: nodeproto.ProtocolVersion + 1}
	encoded, err := json.Marshal(older)
	if err != nil {
		t.Fatal(err)
	}
	refused := harness.post(t, nodeproto.PathHello, 0, bytes.NewReader(encoded))
	if refused.status != http.StatusConflict || !bytes.Contains(refused.body, []byte("update it")) {
		t.Fatalf("a hello in another protocol answered %d %q, want 409 saying to update the node",
			refused.status, refused.body)
	}
	if connection, _ := harness.service.NodeConnection(ctx, connected.NodeID); connection != NeedsUpdate {
		t.Errorf("the refused node is %s, want %s", connection, NeedsUpdate)
	}
	scan := nodeproto.Command{JobID: 3, Work: nodeproto.Work{
		Type: nodeproto.CommandList, List: &nodeproto.List{RootPath: "/media", Path: "."},
	}}
	parking, isParked := jobqueue.ParkingOf(harness.service.Send(ctx, connected.NodeID, scan))
	if !isParked || parking.Destination != jobs.NodeDestination(connected.NodeID) ||
		parking.Reason != jobs.ReasonNeedsUpdate || !parking.AwaitsSignal {
		t.Errorf("work for it parks %+v (%v), want it waiting on the node, which needs updating", parking, isParked)
	}
	harness.hello(t, nodeproto.Hello{Name: "local"})
	if connection, _ := harness.service.NodeConnection(ctx, connected.NodeID); connection != Connected {
		t.Errorf("after a hello in this core's protocol the node is %s, want connected", connection)
	}
}

// A node's status is reconciled with what it was given (D234 §6), judging none of the attempts whose
// command it had not taken when it made the status: those are on their way.
func TestStatus_IsReconciledWithWhatTheNodeWasGivenButForCommandsOnTheirWay(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	sink := &recordingResultSink{}
	harness.service.SetResultSink(sink)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	for _, token := range []string{"taken", "on its way"} {
		harness.service.Enqueue(reply.NodeID, nodeproto.Command{
			JobID: 7, AttemptToken: token, Work: nodeproto.Work{Type: nodeproto.CommandPrepare},
		})
	}
	held := []nodeproto.HeldAttempt{{JobID: 7, AttemptToken: "taken", State: nodeproto.HeldRunning}}
	harness.postStatus(t, reply.NodeID, nodeproto.Status{Held: held, TakenCommandSeq: 1})
	if len(sink.reconciled) != 1 || len(sink.reconciled[0].held) != 1 ||
		sink.reconciled[0].held[0].AttemptToken != "taken" ||
		!sink.reconciled[0].onTheirWay["on its way"] || sink.reconciled[0].onTheirWay["taken"] {
		t.Errorf("reconciled %+v; want what the node holds, with only the command it had not taken on its way",
			sink.reconciled)
	}
}

// An attempt given to a node whose command is not queued yet, its job's assignment still being
// recorded, is on its way to a status, which cannot have taken it; for unsentGrace at most, after which
// one never queued is judged as any other (the M2 acceptance under check-pr's load, 2026-10-08).
func TestStatus_JudgesNoAttemptWhoseCommandIsNotQueuedYet(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	sink := &recordingResultSink{}
	harness.service.SetResultSink(sink)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	harness.service.WillSend(reply.NodeID, "being assigned")
	harness.postStatus(t, reply.NodeID, nodeproto.Status{})
	harness.clock.Advance(unsentGrace)
	harness.postStatus(t, reply.NodeID, nodeproto.Status{})
	if len(sink.reconciled) != 2 || !sink.reconciled[0].onTheirWay["being assigned"] ||
		sink.reconciled[1].onTheirWay["being assigned"] {
		t.Errorf("reconciled %+v; want the attempt on its way, then judged once unsentGrace has passed",
			sink.reconciled)
	}
}

// An attempt's commands the node has not taken are withdrawn when its job ends, its work and its
// cancel, and never sent (S27); withdrawing says whether the work was still queued.
func TestWithdraw_TakesAnEndedAttemptsCommandsOutBeforeTheNodeTakesThem(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	work := func(token string) nodeproto.Command {
		return nodeproto.Command{JobID: 7, AttemptToken: token, Work: nodeproto.Work{Type: nodeproto.CommandPrepare}}
	}
	harness.service.Enqueue(reply.NodeID, work("ended"))
	harness.service.Enqueue(reply.NodeID, work("kept"))
	if !harness.service.Withdraw(reply.NodeID, "ended") {
		t.Error("withdrawing an attempt whose work was queued said it was not")
	}
	if harness.service.Withdraw(reply.NodeID, "never sent") {
		t.Error("withdrawing an attempt with nothing queued said its work was")
	}
	commands, _ := harness.pollThroughHold(t, reply.NodeID, 0)
	if len(commands.Commands) != 1 || commands.Commands[0].AttemptToken != "kept" {
		t.Errorf("the node took %+v, want only the attempt that did not end", commands.Commands)
	}
}

// Work an answer carried to the node, which it has not acknowledged yet, and a change of its
// priority are not work never sent: withdrawing the attempt says the node may hold it, so the node
// is told to stop it, not left holding work no job wants (staging, 2026-10-16).
func TestWithdraw_WorkCarriedToTheNodeIsNotWithdrawnAsNeverSent(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	harness.service.Enqueue(reply.NodeID, nodeproto.Command{
		JobID: 7, AttemptToken: "carried", Work: nodeproto.Work{Type: nodeproto.CommandReadThrough},
	})
	if commands, _ := harness.pollThroughHold(t, reply.NodeID, 0); len(commands.Commands) != 1 {
		t.Fatalf("the node took %+v, want the work", commands.Commands)
	}
	harness.service.Enqueue(reply.NodeID, ReprioritizeCommand(7, "carried", 5))
	if harness.service.Withdraw(reply.NodeID, "carried") {
		t.Error("withdrawing work an answer carried to the node said it was never sent")
	}
}

// fakeRenditionIndex keeps the renditions named in keeps, or fails when err is set.
type fakeRenditionIndex struct {
	keeps []string
	err   error
}

func (index fakeRenditionIndex) AdoptRenditions(
	context.Context, int64, []nodeproto.RenditionManifest,
) ([]string, error) {
	return index.keeps, index.err
}

// The reply to a hello names the renditions it listed that the index keeps; the node evicts the
// others (D238 §2). With no index, or one that cannot be reconciled, it keeps them all: the node
// evicts nothing for the core's failure.
func TestHello_RepliesWithTheRenditionsTheIndexKeepsAndKeepsAllWhenItCannotSay(t *testing.T) {
	t.Parallel()
	listed := []nodeproto.RenditionManifest{
		{Prepare: nodeproto.Prepare{RenditionDir: "1/remux/aaaaaaaaaaaaaaaa"}},
		{Prepare: nodeproto.Prepare{RenditionDir: "2/remux/bbbbbbbbbbbbbbbb"}},
	}
	all := []string{"1/remux/aaaaaaaaaaaaaaaa", "2/remux/bbbbbbbbbbbbbbbb"}
	for _, each := range []struct {
		name  string
		index RenditionIndex
		want  []string
	}{
		{name: "no index", want: all},
		{name: "an index that keeps one", index: fakeRenditionIndex{keeps: all[1:]}, want: all[1:]},
		{name: "an index that fails", index: fakeRenditionIndex{err: errors.New("cache.db is gone")}, want: all},
	} {
		harness := newFixture(t, time.Millisecond)
		if each.index != nil {
			harness.service.SetRenditionIndex(each.index)
		}
		reply := harness.hello(t, nodeproto.Hello{Name: "local", Renditions: listed})
		if !slices.Equal(reply.KeptRenditionDirs, each.want) {
			t.Errorf("with %s the reply kept %v, want %v", each.name, reply.KeptRenditionDirs, each.want)
		}
	}
}

// The reply to a hello gives each of its devices the reader limit its kind's setting says, and a
// kind the core does not know the unknown kind's (D240 §5).
func TestHello_RepliesWithEachDevicesReaderLimitByItsKind(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	settingstest.Set(t, harness.settings, settings.SSDReaderLimit, "6")
	reply := harness.hello(t, nodeproto.Hello{Name: "local", Devices: []nodeproto.Device{
		{Key: "zfs:tank", Kind: nodeproto.DeviceSpinning},
		{Key: "disk:sda", Kind: nodeproto.DeviceSSD},
		{Key: "network:nas:/films", Kind: nodeproto.DeviceNetwork},
		{Key: "unknown:0:30", Kind: "tape"},
	}})
	want := map[string]int{"zfs:tank": 1, "disk:sda": 6, "network:nas:/films": 2, "unknown:0:30": 1}
	if !maps.Equal(reply.DeviceReaderLimits, want) {
		t.Errorf("the reply's reader limits are %v, want %v", reply.DeviceReaderLimits, want)
	}
}

// Each answer to a poll for commands carries the reader limits as the settings say then, so a change
// reaches the node without a new hello (D240 §5).
func TestCommands_EachAnswerCarriesTheReaderLimitsTheSettingsSayNow(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	reply := harness.hello(t, nodeproto.Hello{Name: "local", Devices: []nodeproto.Device{
		{Key: "zfs:tank", Kind: nodeproto.DeviceSpinning},
	}})
	settingstest.Set(t, harness.settings, settings.SpinningDiskReaderLimit, "2")
	commands, status := harness.pollThroughHold(t, reply.NodeID, 0)
	if status != http.StatusOK || commands.DeviceReaderLimits["zfs:tank"] != 2 {
		t.Errorf("the poll answered %d with limits %v; want zfs:tank's raised to 2", status,
			commands.DeviceReaderLimits)
	}
}

// The reply to a hello and each answer to a poll say how often the node posts its status: three
// times within node_silent_after_seconds as it is then, so the node's cadence follows the setting it
// is judged silent by (cut-a §4; the constants sweep, 2026-10-21).
func TestHelloAndCommands_SayTheStatusCadenceTheSilenceSettingGives(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	settingstest.Set(t, harness.settings, settings.NodeSilentAfterSeconds, "60")
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	if reply.Tuning.StatusEverySeconds != 20 {
		t.Errorf("the hello's reply says every %d s; want 20, a third of the 60 s silence",
			reply.Tuning.StatusEverySeconds)
	}
	settingstest.Set(t, harness.settings, settings.NodeSilentAfterSeconds, "9")
	if commands, status := harness.pollThroughHold(t, reply.NodeID, 0); status != http.StatusOK ||
		commands.Tuning.StatusEverySeconds != 3 {
		t.Errorf("the poll answered %d saying every %d s; want 3, a third of the 9 s silence now", status,
			commands.Tuning.StatusEverySeconds)
	}
}

// A node posts its status at its own starting cadence until a core says one, so that cadence is the
// one the silence setting's starting value gives (the constants sweep, 2026-10-21).
func TestStatusEveryFor_ANodesCadenceBeforeACoreSaysIsTheSilenceSettingsStart(t *testing.T) {
	t.Parallel()
	if every := StatusEveryFor(settings.StartingDuration(settings.NodeSilentAfterSeconds)); every !=
		nodeproto.StartingStatusEverySeconds {
		t.Errorf("a node posts every %d s before a core says; node_silent_after_seconds' start gives %d s",
			nodeproto.StartingStatusEverySeconds, every)
	}
}

// Each answer to a poll carries how strongly each chunk store is kept as the players stand then; with
// nothing to say them, or when they cannot be read, none, which leaves the node its last (D309 §8).
func TestCommands_EachAnswerCarriesHowStronglyEachChunkStoreIsKept(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	if commands, _ := harness.pollThroughHold(t, reply.NodeID, 0); commands.StoreTiers != nil {
		t.Errorf("with nothing to say them, the tiers are %v; want none", commands.StoreTiers)
	}
	said := map[string]nodeproto.KeepTier{"1/chunks-q400k360p/g": nodeproto.KeepInUse}
	harness.service.SetStoreTiers(func(context.Context) (map[string]nodeproto.KeepTier, error) { return said, nil })
	if commands, _ := harness.pollThroughHold(t, reply.NodeID, 0); !maps.Equal(commands.StoreTiers, said) {
		t.Errorf("the tiers = %v; want %v", commands.StoreTiers, said)
	}
	harness.service.SetStoreTiers(func(context.Context) (map[string]nodeproto.KeepTier, error) {
		return nil, errors.New("the settings cannot be read")
	})
	if commands, _ := harness.pollThroughHold(t, reply.NodeID, 0); commands.StoreTiers != nil {
		t.Errorf("when the tiers cannot be read, they are %v; want none", commands.StoreTiers)
	}
}

// What a node's machine can do is its config's, said in its hello: the core knows each node's slots
// as its last hello said, which it holds the node's work to (D240 §5), and how fast each reads.
func TestHello_TellsTheCoreEachNodesSlotsAndHowFastItReads(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	local := harness.hello(t, nodeproto.Hello{Name: "local", Capacity: nodeproto.Capacity{
		EncodeSlotCount: 1, RemuxSlotCount: 2, SlowestReadBytesPerSecond: 20 << 20,
	}})
	attic := harness.hello(t, nodeproto.Hello{Name: "attic", Capacity: nodeproto.Capacity{
		EncodeSlotCount: 2, RemuxSlotCount: 4, SlowestReadBytesPerSecond: 5 << 20,
	}})
	harness.hello(t, nodeproto.Hello{Name: "local", Capacity: nodeproto.Capacity{
		EncodeSlotCount: 1, RemuxSlotCount: 3, SlowestReadBytesPerSecond: 40 << 20,
	}})
	for _, node := range []struct {
		id               int64
		encodes, remuxes int
	}{{local.NodeID, 1, 3}, {attic.NodeID, 2, 4}} {
		resources, isConnected, err := harness.service.Resources(t.Context(), node.id)
		if err != nil || !isConnected || resources.Capacity.EncodeSlotCount != node.encodes ||
			resources.Capacity.RemuxSlotCount != node.remuxes {
			t.Errorf("node %d's resources are %+v (%v, %v), want %d encodes and %d remuxes, as its last hello said",
				node.id, resources, isConnected, err, node.encodes, node.remuxes)
		}
	}
	if rate := harness.service.SlowestReadBytesPerSecond(local.NodeID); rate != 40<<20 {
		t.Errorf("local reads at %d bytes a second, want its last hello's 40 MiB", rate)
	}
	if rate := harness.service.SlowestReadBytesPerSecond(99); rate != 0 {
		t.Errorf("a node that has not said hello reads at %d, want 0: not known", rate)
	}
}

// A device's reader limit an admin set wins over its kind's setting, in the hello's reply, each poll's
// answer and the device's state; cleared, the device takes its kind's again; a node no live node is
// named is refused (D240 §5).
func TestSetReaderLimit_ADevicesSetLimitWinsOverItsKindsUntilCleared(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	devices := []nodeproto.Device{
		{Key: "disk:sdc", Kind: nodeproto.DeviceUnknown}, {Key: "zfs:tank", Kind: nodeproto.DeviceSpinning},
	}
	reply := harness.hello(t, nodeproto.Hello{Name: "local", Devices: devices})
	four := 4
	if err := harness.service.SetReaderLimit(t.Context(), "local", "disk:sdc", &four); err != nil {
		t.Fatal(err)
	}
	if commands, _ := harness.pollThroughHold(t, reply.NodeID, 0); commands.DeviceReaderLimits["disk:sdc"] != 4 ||
		commands.DeviceReaderLimits["zfs:tank"] != 1 {
		t.Errorf("the poll's limits are %v, want sdc's set 4 and tank's kind's 1", commands.DeviceReaderLimits)
	}
	states, err := harness.service.DeviceStates(t.Context(), reply.NodeID)
	if err != nil || len(states) != 2 || states[0].ReaderLimit != 4 || !states[0].IsReaderLimitSet ||
		states[1].ReaderLimit != 1 || states[1].IsReaderLimitSet {
		t.Errorf("the device states are %+v (%v), want sdc at its set 4 and tank at its kind's 1", states, err)
	}
	if err := harness.service.SetReaderLimit(t.Context(), "local", "disk:sdc", nil); err != nil {
		t.Fatal(err)
	}
	again := harness.hello(t, nodeproto.Hello{Name: "local", Devices: devices})
	if again.DeviceReaderLimits["disk:sdc"] != 1 {
		t.Errorf("once cleared, sdc's limit is %d, want its kind's 1", again.DeviceReaderLimits["disk:sdc"])
	}
	if err := harness.service.SetReaderLimit(t.Context(), "attic", "disk:sdc", &four); !errors.Is(err, ErrNoNode) {
		t.Errorf("setting a limit on attic, no node, answered %v; want ErrNoNode", err)
	}
}
