// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// The node, as runNode assembles it, says in each status what it and its FFmpeg runs use of its
// machine, which the core records each minute (internal/core/resourceuse).
func TestRunNode_SaysInItsStatusWhatItUsesOfItsMachine(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stdout: "ffprobe version test"})
	config.Node.FFmpegDir = fake.Directory
	statuses := statusesToFakeCore(t, config.Node.CoreSocket)
	captured := newCapturedLog()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- runNodeWith(ctx, config, captured.logger, fakeFFmpegSeams) }()
	defer func() {
		cancel()
		if err := testutil.Receive(t, done, "the node to stop"); err != nil {
			t.Errorf("runNode returned %v", err)
		}
	}()
	status := testutil.Receive(t, statuses, "the node's first status")
	if use := status.SelfUse; use == nil || use.Scope == "" {
		t.Errorf("the node's status says it uses %+v of its machine; want its reading\n%s", use, captured)
	}
}

// statusesToFakeCore serves a core on socket that takes the node's hello, holds its poll for
// commands open, and passes on each status it is sent.
func statusesToFakeCore(t *testing.T, socket string) <-chan nodeproto.Status {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	statuses := make(chan nodeproto.Status, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+nodeproto.PathHello, func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(nodeproto.HelloReply{NodeID: 1}) //nolint:errcheck // the node says if not
	})
	mux.HandleFunc("GET "+nodeproto.PathCommands, func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	})
	mux.HandleFunc("POST "+nodeproto.PathStatus, func(writer http.ResponseWriter, request *http.Request) {
		var status nodeproto.Status
		if err := json.NewDecoder(request.Body).Decode(&status); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		select {
		case statuses <- status:
		default:
		}
		_, _ = writer.Write([]byte("{}")) //nolint:errcheck // the node says if not
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: serveWaitLimit}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	t.Cleanup(func() {
		_ = server.Close() //nolint:errcheck // a test's server, closed at its end
		if err := <-served; !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("the fake core stopped with %v", err)
		}
	})
	return statuses
}
