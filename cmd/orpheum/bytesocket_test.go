// SPDX-License-Identifier: GPL-3.0-or-later

package main

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
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/unixsocket"
	"github.com/kstenerud/orpheum/internal/store/storetest"
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
func (*takenResults) DeliverProgress(context.Context, int64, nodeproto.Progress)              {}
func (*takenResults) Interrupt(context.Context, int64)                                        {}
func (*takenResults) Square(context.Context, int64, []nodeproto.HeldAttempt, map[string]bool) {}

// The core's byte socket client is how its node channel fetches a result the node holds, too large
// to post (D234 §5): wired so, a post naming one hands on the whole result from the node's socket.
func TestOpenByteSocket_LetsTheNodeChannelTakeAResultTheNodeHolds(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	service := nodes.NewService(nodes.ServiceParts{
		Database: database, Clock: clock.System{}, Settings: settingstest.Seeded(t, database),
		Destinations: ignoringReports{}, Logger: slog.New(slog.DiscardHandler),
		PollHold: time.Second,
	})
	taken := &takenResults{}
	service.SetResultSink(taken)
	whole, err := json.Marshal(nodeproto.Result{
		JobID: 4, AttemptToken: "a4", Work: &nodeproto.Work{Type: nodeproto.CommandMeasureLoudness},
		Loudness: &nodeproto.LoudnessResult{Failure: "too large to post, in a test's terms"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var config Config
	config.Server.SocketDir = filepath.Join(t.TempDir(), "run")
	listener, err := unixsocket.Listen(config.Server.SocketDir, nodeSocketName)
	if err != nil {
		t.Fatal(err)
	}
	nodeSocket := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(whole) //nolint:errcheck // the test reads what arrives
	}), ReadHeaderTimeout: time.Second}
	go func() { _ = nodeSocket.Serve(listener) }() //nolint:errcheck // closed at cleanup
	t.Cleanup(func() { _ = nodeSocket.Close() })   //nolint:errcheck // as above
	openByteSocket(service, config)
	channel := httptest.NewServer(service.Handler())
	t.Cleanup(channel.Close)
	post := func(path string, nodeID int64, body any) (int, []byte) {
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
	_, hello := post(nodeproto.PathHello, 0, nodeproto.Hello{Name: "local", ProtocolVersion: nodeproto.ProtocolVersion})
	if err := json.Unmarshal(hello, &reply); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(whole)
	status, _ := post(nodeproto.PathResult, reply.NodeID, nodeproto.Result{
		JobID: 4, AttemptToken: "a4", Work: &nodeproto.Work{Type: nodeproto.CommandMeasureLoudness},
		Held: &nodeproto.HeldResult{SizeBytes: int64(len(whole)), Sha256: hex.EncodeToString(sum[:])},
	})
	if status != http.StatusOK || len(taken.results) != 1 || taken.results[0].Loudness == nil {
		t.Errorf("a post naming a held result: %d, handed on %+v; want the whole result", status, taken.results)
	}
}
