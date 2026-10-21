// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/unixsocket"
)

// markedRoots notes each root made unavailable after its disk did not answer.
type markedRoots struct {
	since time.Time
	roots []string
}

func (marked *markedRoots) MarkRootStalled(_ context.Context, _ int64, rootPath string) (time.Time, error) {
	marked.roots = append(marked.roots, rootPath)
	return marked.since, nil
}

// A node that answers a read 504 naming a root, its disk not answering, has the root made
// unavailable, and the read is a StalledRootError saying since when (D233 §10, D241).
func TestByteSocketClient_ARootANodeSaysDoesNotAnswerIsMadeUnavailable(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	socketDir := filepath.Join(t.TempDir(), "run")
	reply := harness.hello(t, nodeproto.Hello{Name: "local", ByteSocketPath: filepath.Join(socketDir, "node.sock")})
	listener, err := unixsocket.Listen(socketDir, "node.sock")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set(nodeproto.HeaderStalledRoot, "/media/films")
		writer.WriteHeader(http.StatusGatewayTimeout)
	}), ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(listener) }() //nolint:errcheck // closed at cleanup
	t.Cleanup(func() { _ = server.Close() })   //nolint:errcheck // as above
	marked := &markedRoots{since: time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)}
	client := NewByteSocketClient(harness.service, marked)
	response, err := client.Do(t.Context(), reply.NodeID, http.MethodGet, nodeproto.PathBytesFile, nil)
	if response != nil {
		_ = response.Body.Close() //nolint:errcheck // read-only
	}
	stalled, isStalled := errors.AsType[*StalledRootError](err)
	if !isStalled || stalled.RootPath != "/media/films" || !stalled.Since.Equal(marked.since) ||
		!errors.Is(err, ErrStalledOnDisk) {
		t.Errorf("the read = %v; want /media/films stalled since %v", err, marked.since)
	}
	if len(marked.roots) != 1 || marked.roots[0] != "/media/films" {
		t.Errorf("the roots made unavailable = %v; want /media/films", marked.roots)
	}
}
