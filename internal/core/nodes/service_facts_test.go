// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/host"
)

// Each status is taken as it arrives: the node is heard, and its last status kept (D234 §6).
func TestStatus_TheLastIsKeptAndTheNodeHeard(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	for _, sentAt := range []int64{7, 8} {
		harness.postStatus(t, reply.NodeID, nodeproto.Status{SentAt: sentAt})
	}
	if facts, _ := harness.service.Facts("local"); facts.Status == nil || facts.Status.SentAt != 8 {
		t.Errorf("the status kept in memory is %+v, want the last one posted", facts.Status)
	}
	if heard, ever := harness.service.LastHeard("local"); !ever || !heard.Equal(harness.clock.Now()) {
		t.Errorf("LastHeard = %v, %v", heard, ever)
	}
}

func TestStatus_ABodyThatIsNotAStatusIsRefused(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	response := harness.post(t, nodeproto.PathStatus, reply.NodeID, strings.NewReader(`{"sentAt":"soon"}`))
	if response.status != http.StatusBadRequest || !strings.Contains(string(response.body), "a status is JSON") {
		t.Errorf("a status that is not one: %d %q, want a 400 saying why", response.status, response.body)
	}
}

func TestConnectedNodeRoots_AreWhatTheNodesLastHelloNamed(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	if _, _, connected := harness.service.ConnectedNodeRoots(t.Context(), "local"); connected {
		t.Error("a node that never said hello has roots")
	}
	harness.hello(t, nodeproto.Hello{Name: "local", RootPaths: []string{"/media/films", "/media/shows"}})
	reply := harness.hello(t, nodeproto.Hello{Name: "local", RootPaths: []string{"/media/films"}})
	nodeID, roots, connected := harness.service.ConnectedNodeRoots(t.Context(), "local")
	if !connected || nodeID != reply.NodeID || !slices.Equal(roots, []string{"/media/films"}) {
		t.Errorf("Roots = %d, %v, %v; want the last hello's", nodeID, roots, connected)
	}
}

// The answer to a result is the node's acknowledgement (D234 §5): a result the sink could not record
// is refused, and the node sends it again; one it cannot take as it is, as the node's mistake.
func TestResult_IsAcknowledgedOnlyOnceTheSinkRecordedIt(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	sink := &recordingResultSink{}
	harness.service.SetResultSink(sink)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	post := func() int {
		result := nodeproto.Result{JobID: 4, AttemptToken: "a4", ReadThrough: &nodeproto.ReadThroughResult{}}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		return harness.post(t, nodeproto.PathResult, reply.NodeID, bytes.NewReader(encoded)).status
	}
	sink.refusal = errors.New("cache.db is busy")
	if status := post(); status != http.StatusInternalServerError {
		t.Errorf("a result the sink could not record: %d, want 500", status)
	}
	sink.refusal = ErrMalformedResult
	if status := post(); status != http.StatusBadRequest {
		t.Errorf("a result the sink cannot take as it is: %d, want 400", status)
	}
	sink.refusal = nil
	if status := post(); status != http.StatusOK || len(sink.results) != 1 {
		t.Errorf("a result the sink recorded: %d, sink %+v; want 200 and the result", status, sink.results)
	}
}

// recordingResultSink records the results and progress it is handed, and the nodes it is told
// started again.
type recordingResultSink struct {
	results  []nodeproto.Result
	from     []int64
	progress []nodeproto.Progress
	// interrupted are the calls to interrupt the work of a node's other starts, in order.
	interrupted []interruption
	// refusal, when set, is Deliver's error: a result the sink could not record.
	refusal error
	// reconciled are the calls to reconcile what a node holds, in order.
	reconciled []reconciliation
}

func (sink *recordingResultSink) Reconcile(
	_ context.Context, _ int64, held []nodeproto.HeldAttempt, onTheirWay map[string]bool,
) {
	sink.reconciled = append(sink.reconciled, reconciliation{held: held, onTheirWay: onTheirWay})
}

// reconciliation is one call to reconcile what a node holds.
type reconciliation struct {
	held       []nodeproto.HeldAttempt
	onTheirWay map[string]bool
}

func (sink *recordingResultSink) DeliverProgress(_ context.Context, _ int64, progress nodeproto.Progress) {
	sink.progress = append(sink.progress, progress)
}

// A node's progress, on its own path, reaches the result sink from a node that said hello, and
// counts as hearing from it.
func TestProgress_ReachesTheResultSinkFromAKnownNodeOnly(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	sink := &recordingResultSink{}
	harness.service.SetResultSink(sink)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	progressJSON := func() io.Reader {
		encoded, err := json.Marshal(nodeproto.Progress{JobID: 4, AttemptToken: "a4", State: nodeproto.ProgressRunning})
		if err != nil {
			t.Fatal(err)
		}
		return bytes.NewReader(encoded)
	}
	if stranger := harness.post(t, nodeproto.PathProgress, reply.NodeID+1, progressJSON()); stranger.status !=
		http.StatusConflict {
		t.Errorf("progress from a node that never said hello: %d, want 409", stranger.status)
	}
	taken := harness.post(t, nodeproto.PathProgress, reply.NodeID, progressJSON())
	if taken.status != http.StatusOK || len(sink.progress) != 1 || sink.progress[0].AttemptToken != "a4" ||
		sink.progress[0].State != nodeproto.ProgressRunning {
		t.Errorf("answered %d %s; the sink has %+v, want attempt a4 running", taken.status, taken.body, sink.progress)
	}
}

// interruption is a call to interrupt the work given to a node under another start than startID.
type interruption struct {
	nodeID  int64
	startID string
}

func (sink *recordingResultSink) Interrupt(_ context.Context, nodeID int64, startID string) {
	sink.interrupted = append(sink.interrupted, interruption{nodeID: nodeID, startID: startID})
}

// Every hello, even the first since the core itself started, has the work given to any other start
// of its node interrupted, by the start it names: a node that started again lost the work it held in
// memory (D238 §3), whether or not this core heard its last start. The work its own start was given
// is left with it, as the start ids it was given under say.
func TestHello_ANodeThatStartedAgainInterruptsTheJobsWaitingOnIt(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	sink := &recordingResultSink{}
	harness.service.SetResultSink(sink)
	first := harness.hello(t, nodeproto.Hello{Name: "local", StartID: "a1"})
	harness.hello(t, nodeproto.Hello{Name: "local", StartID: "b2"})
	want := []interruption{{nodeID: first.NodeID, startID: "a1"}, {nodeID: first.NodeID, startID: "b2"}}
	if !slices.Equal(sink.interrupted, want) {
		t.Errorf("two hellos interrupted %+v, want the work of every start but each hello's own: %+v",
			sink.interrupted, want)
	}
}

func (sink *recordingResultSink) Deliver(_ context.Context, nodeID int64, result nodeproto.Result) error {
	if sink.refusal != nil {
		return sink.refusal
	}
	sink.results = append(sink.results, result)
	sink.from = append(sink.from, nodeID)
	return nil
}

func TestResult_AResultReachesTheResultSinkFromAKnownNodeOnly(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	resultJSON := func(result nodeproto.Result) io.Reader {
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		return bytes.NewReader(encoded)
	}
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	result := nodeproto.Result{JobID: 4, Artwork: &nodeproto.ArtworkBatchResult{Images: []nodeproto.ArtworkResult{{
		Images: []nodeproto.ArtImage{{SizeName: nodeproto.PortraitS, WebP: []byte("RIFF")}},
	}}}}
	if refused := harness.post(t, nodeproto.PathResult, reply.NodeID, resultJSON(result)); refused.status !=
		http.StatusServiceUnavailable {
		t.Errorf("with no result sink: %d, want 503", refused.status)
	}
	sink := &recordingResultSink{}
	harness.service.SetResultSink(sink)
	if stranger := harness.post(t, nodeproto.PathResult, reply.NodeID+1, resultJSON(result)); stranger.status !=
		http.StatusConflict {
		t.Errorf("from a node that never said hello: %d, want 409", stranger.status)
	}
	taken := harness.post(t, nodeproto.PathResult, reply.NodeID, resultJSON(result))
	if taken.status != http.StatusOK || len(sink.results) != 1 || sink.from[0] != reply.NodeID ||
		sink.results[0].JobID != 4 || sink.results[0].Artwork == nil ||
		string(sink.results[0].Artwork.Images[0].Images[0].WebP) != "RIFF" {
		t.Errorf("answered %d %s; sink has %+v", taken.status, taken.body, sink.results)
	}
	if connected, found := harness.service.ConnectedNodeID(t.Context()); !found || connected != reply.NodeID {
		t.Errorf("Connected = %d, %v; want the node", connected, found)
	}
	ctx := t.Context()
	if !harness.service.IsConnected(ctx, reply.NodeID) || harness.service.IsConnected(ctx, reply.NodeID+1) {
		t.Error("IsConnected does not tell the node that said hello from one that did not")
	}
}

func TestHello_KeepsWhatTheNodeSaidAboutItsConfinement(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	if _, found := harness.service.Facts("local"); found {
		t.Error("a node that never said hello has a confinement")
	}
	said := []nodeproto.Confinement{
		{Mechanism: "landlock", Reason: "not enabled at boot", Remedy: "add landlock to lsm="},
		{Mechanism: "seccomp", IsInForce: true},
	}
	harness.hello(t, nodeproto.Hello{Name: "local", Confinement: said})
	facts, found := harness.service.Facts("local")
	kept := facts.Hello.Confinement
	if !found || len(kept) != 2 || kept[0] != said[0] || kept[1] != said[1] {
		t.Errorf("the kept confinement = %+v, %v; want what the hello said (D112)", kept, found)
	}
}

func TestStatus_KeepsWhatTheNodeSaysItsPreparedDirectoryHolds(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	if facts, _ := harness.service.Facts("local"); facts.Status != nil {
		t.Error("a node that has sent no status has a prepared usage")
	}
	usage := nodeproto.PreparedUsage{UsedBytes: 10, BudgetBytes: 100, DiskUsedPercent: 50, DiskUsedLimitPercent: 95}
	harness.postStatus(t, reply.NodeID, nodeproto.Status{PreparedUsage: &usage})
	facts, _ := harness.service.Facts("local")
	if facts.Status == nil || facts.Status.PreparedUsage == nil || *facts.Status.PreparedUsage != usage {
		t.Errorf("the kept status = %+v; want the prepared usage it said (D217 §5)", facts.Status)
	}
}

func TestFacts_KeepTheHellosHardwareAndTheLastStatussDisks(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	if _, found := harness.service.Facts("local"); found {
		t.Error("a node that has not said hello has facts")
	}
	hardware := nodeproto.Hardware{
		GPUs:          []nodeproto.GPU{{DevicePath: "/dev/dri/renderD128", Driver: "i915"}},
		Trials:        []nodeproto.EncoderTrial{{Encoder: "h264_qsv"}, {Encoder: "h264_nvenc", Failure: "no device"}},
		ChosenEncoder: "h264_qsv",
	}
	reply := harness.hello(t, nodeproto.Hello{
		Name: "local", Host: host.System{Platform: "linux/amd64", Codename: "trixie"}, Hardware: hardware,
	})
	disk := nodeproto.Disk{Path: "/media", TotalBytes: 1000, AvailableBytes: 400}
	harness.postStatus(t, reply.NodeID, nodeproto.Status{Disks: []nodeproto.Disk{disk}})
	facts, found := harness.service.Facts("local")
	kept := facts.Hello.Hardware
	if !found || facts.NodeID != reply.NodeID || facts.Hello.Host.Codename != "trixie" ||
		kept.ChosenEncoder != "h264_qsv" || len(kept.Trials) != 2 || kept.GPUs[0].Driver != "i915" {
		t.Errorf("the facts' hello is %+v, %v; want the host and hardware the hello said (D225)",
			facts.Hello, found)
	}
	if facts.Status == nil || len(facts.Status.Disks) != 1 || facts.Status.Disks[0] != disk {
		t.Errorf("the facts' status is %+v; want the disks the status said (D225)", facts.Status)
	}
}

// A node encodes on a GPU when its hello chose Quick Sync or NVENC; one that chose x264, or has not
// said hello, does not (D323 §3).
func TestEncodesOnAGPU_IsTheHellosChosenEncoderOtherThanX264(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	if harness.service.EncodesOnAGPU(1) {
		t.Error("a node that has not said hello encodes on a GPU")
	}
	reply := harness.hello(t, nodeproto.Hello{Name: "local", Hardware: nodeproto.Hardware{ChosenEncoder: "h264_qsv"}})
	if !harness.service.EncodesOnAGPU(reply.NodeID) {
		t.Error("a node whose hello chose Quick Sync does not encode on a GPU")
	}
	harness.hello(t, nodeproto.Hello{Name: "local", Hardware: nodeproto.Hardware{ChosenEncoder: "x264"}})
	if harness.service.EncodesOnAGPU(reply.NodeID) {
		t.Error("a node whose hello chose x264 encodes on a GPU")
	}
}

// A hello says where its node's byte socket is, as an absolute path, or it is refused: the core
// reads every byte of the node's from there (D242 §7).
func TestHello_RefusesOneThatDoesNotSayWhereItsByteSocketIs(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	for _, socketPath := range []string{"", "run/node.sock"} {
		encoded, err := json.Marshal(nodeproto.Hello{
			Name: "local", ProtocolVersion: nodeproto.ProtocolVersion, ByteSocketPath: socketPath,
		})
		if err != nil {
			t.Fatal(err)
		}
		if response := harness.post(t, nodeproto.PathHello, 0, bytes.NewReader(encoded)); response.status !=
			http.StatusBadRequest {
			t.Errorf("a hello whose byte socket is %q answered %d; want 400", socketPath, response.status)
		}
	}
}
