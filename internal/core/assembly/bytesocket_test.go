// SPDX-License-Identifier: AGPL-3.0-or-later

package assembly

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/unixsocket"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// ignoringReports takes a node's parked destinations, and does nothing with them.
type ignoringReports struct{}

func (ignoringReports) Unpark(context.Context, jobs.Destination) {}

// takenResults keeps the results the node channel hands on.
type takenResults struct{ results []nodeproto.Result }

func (taken *takenResults) Deliver(_ context.Context, _ int64, result nodeproto.Result) error {
	taken.results = append(taken.results, result)
	return nil
}

func (*takenResults) DeliverProgress(context.Context, int64, nodeproto.Progress) {}

func (*takenResults) Interrupt(context.Context, int64, string) {}

func (*takenResults) Reconcile(context.Context, int64, []nodeproto.HeldAttempt, map[string]bool) {}

// benchSocketName is the bench node's byte socket, in its run directory.
const benchSocketName = "node.sock"

// byteSocketBench is a node channel whose node said hello, with its byte socket answered by
// answer, and how to post to the channel as that node.
type byteSocketBench struct {
	service *nodes.Service
	nodeID  int64
	post    func(path string, body any) int
}

func newByteSocketBench(t *testing.T, answer http.HandlerFunc) byteSocketBench {
	t.Helper()
	database := storetest.OpenMigrated(t)
	service := nodes.NewService(nodes.ServiceParts{
		Database: database, Clock: clock.System{}, Settings: settingstest.Seeded(t, database),
		Destinations: ignoringReports{}, Logger: slog.New(slog.DiscardHandler),
		PollHold: time.Second,
	})
	runDir := filepath.Join(testutil.SocketDirectory(t), "run")
	listener, err := unixsocket.Listen(runDir, benchSocketName)
	if err != nil {
		t.Fatal(err)
	}
	nodeSocket := &http.Server{Handler: answer, ReadHeaderTimeout: time.Second}
	go func() { _ = nodeSocket.Serve(listener) }() //nolint:errcheck // closed at cleanup
	t.Cleanup(func() { _ = nodeSocket.Close() })   //nolint:errcheck // as above
	channel := httptest.NewServer(service.Handler())
	t.Cleanup(channel.Close)
	bench := byteSocketBench{service: service}
	postAs := func(path string, nodeID int64, body any) (int, []byte) {
		t.Helper()
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, channel.URL+path,
			bytes.NewReader(encoded))
		if err != nil {
			t.Fatal(err)
		}
		if nodeID != 0 {
			request.Header.Set(nodes.NodeIDHeader, strconv.FormatInt(nodeID, 10))
		}
		response, err := channel.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }() //nolint:errcheck // read-only
		answer, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, answer
	}
	var reply nodeproto.HelloReply
	_, hello := postAs(nodeproto.PathHello, 0, nodeproto.Hello{
		Name: "local", ProtocolVersion: nodeproto.ProtocolVersion,
		ByteSocketPath: filepath.Join(runDir, benchSocketName),
	})
	if err := json.Unmarshal(hello, &reply); err != nil {
		t.Fatal(err)
	}
	bench.nodeID = reply.NodeID
	bench.post = func(path string, body any) int {
		t.Helper()
		status, _ := postAs(path, reply.NodeID, body)
		return status
	}
	return bench
}

// The core's byte socket client is how its node channel fetches a result the node holds, too large
// to post (D234 §5): wired so, a post naming one hands on the whole result from the node's socket.
func TestOpenByteSocket_LetsTheNodeChannelTakeAResultTheNodeHolds(t *testing.T) {
	t.Parallel()
	whole, err := json.Marshal(nodeproto.Result{
		JobID: 4, AttemptToken: "a4", Work: &nodeproto.Work{Type: nodeproto.CommandReadThrough},
		ReadThrough: &nodeproto.ReadThroughResult{Failure: "too large to post, in a test's terms"},
	})
	if err != nil {
		t.Fatal(err)
	}
	bench := newByteSocketBench(t, func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(whole) //nolint:errcheck // the test reads what arrives
	})
	taken := &takenResults{}
	bench.service.SetResultSink(taken)
	openByteSocket(bench.service, nil)
	sum := sha256.Sum256(whole)
	status := bench.post(nodeproto.PathResult, nodeproto.Result{
		JobID: 4, AttemptToken: "a4", Work: &nodeproto.Work{Type: nodeproto.CommandReadThrough},
		Held: &nodeproto.HeldResult{SizeBytes: int64(len(whole)), Sha256: hex.EncodeToString(sum[:])},
	})
	if status != http.StatusOK || len(taken.results) != 1 || taken.results[0].ReadThrough == nil {
		t.Errorf("a post naming a held result: %d, handed on %+v; want the whole result", status, taken.results)
	}
}

// The core's byte socket client is how the scans probe a root, outside every queue (D327): wired so,
// a probe asks the node's socket and takes its answer.
func TestOpenByteSocket_LetsTheScansProbeARoot(t *testing.T) {
	t.Parallel()
	bench := newByteSocketBench(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != nodeproto.PathBytesProbe {
			http.NotFound(writer, request)
			return
		}
		_ = json.NewEncoder(writer).Encode(nodeproto.PlaceProbe{ //nolint:errcheck // the test reads what arrives
			Path: request.URL.Query().Get("path"), State: nodeproto.ProbeNotAnswering,
		})
	})
	scans := library.NewScans(library.ScansParts{Logger: slog.New(slog.DiscardHandler)})
	openByteSocket(bench.service, scans)
	probe, isBack, err := scans.ProbeAndBringBack(t.Context(), bench.nodeID, "/media/films")
	if err != nil || isBack || probe.State != nodeproto.ProbeNotAnswering || probe.Path != "/media/films" {
		t.Errorf("a probe through the wired socket: %+v, %t, %v; want the node's answer, not answering", probe,
			isBack, err)
	}
}
