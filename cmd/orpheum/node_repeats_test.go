// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// The node, as runNode assembles it, fingerprints sound: a findRepeats its core sends is taken as
// work, not refused as a node without a fingerprinter refuses it (D101, D115).
func TestRunNode_TakesASeasonsComparisonOfItsSound(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stdout: "ffprobe version test"})
	config.Node.FFmpegDir = fake.Directory
	results := resultsFromFakeCore(t, config.Node.CoreSocket, nodeproto.Command{
		CommandSeq: 1, JobID: 1, AttemptToken: "repeats",
		Work: nodeproto.Work{Type: nodeproto.CommandFindRepeats, FindRepeats: &nodeproto.FindRepeats{
			OpeningMs: 600_000, ClosingMs: 360_000,
		}},
	})
	captured := newCapturedLog()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- runNodeWith(ctx, config, processLog{logger: captured.logger}, fakeFFmpegSeams) }()
	defer func() {
		cancel()
		if err := testutil.Receive(t, done, "the node to stop"); err != nil {
			t.Errorf("runNode returned %v", err)
		}
	}()
	result := testutil.Receive(t, results, "the node's answer to the comparison")
	if result.Refusal != "" || result.Repeats == nil || result.Repeats.Refusal != "" {
		t.Errorf("the node answered a season's comparison with %+v (repeats %+v); want it taken\n%s",
			result, result.Repeats, captured)
	}
}

// resultsFromFakeCore serves a core on socket that takes the node's hello, sends it command at its
// first poll and holds every later one open, and passes on each result it is posted.
func resultsFromFakeCore(t *testing.T, socket string, command nodeproto.Command) <-chan nodeproto.Result {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan nodeproto.Result, 1)
	var isSent atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+nodeproto.PathHello, func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(nodeproto.HelloReply{NodeID: 1}) //nolint:errcheck // the node says if not
	})
	mux.HandleFunc("GET "+nodeproto.PathCommands, func(writer http.ResponseWriter, request *http.Request) {
		if !isSent.Swap(true) {
			//nolint:errcheck // the node says if not
			_ = json.NewEncoder(writer).Encode(nodeproto.Commands{Commands: []nodeproto.Command{command}})
			return
		}
		select {
		case <-request.Context().Done():
		case <-t.Context().Done():
		}
	})
	mux.HandleFunc("POST "+nodeproto.PathResult, func(writer http.ResponseWriter, request *http.Request) {
		var result nodeproto.Result
		if err := json.NewDecoder(request.Body).Decode(&result); err == nil {
			select {
			case results <- result:
			default:
			}
		}
		_, _ = writer.Write([]byte("{}")) //nolint:errcheck // the node posts again if not
	})
	mux.HandleFunc("/", func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("{}")) //nolint:errcheck // a status or progress the test does not read
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: serveWaitLimit}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	t.Cleanup(func() {
		_ = server.Close() //nolint:errcheck // a test's server, closed at its end
		if err := testutil.Receive(t, served, "the fake core to stop"); !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("the fake core stopped with %v", err)
		}
	})
	return results
}
