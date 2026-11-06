// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/unixsocket"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// markedRoots notes each root made unavailable after its disk did not answer.
type markedRoots struct {
	since time.Time
	roots []string
}

func (marked *markedRoots) MarkStalled(_ context.Context, _ int64, rootPath, _ string) (string, time.Time, error) {
	marked.roots = append(marked.roots, rootPath)
	return rootPath, marked.since, nil
}

// A node that answers a read 504 naming a root, its disk not answering, has the root made
// unavailable, and the read is a StalledRootError saying since when (D233 §10, D241).
func TestByteSocketClient_ARootANodeSaysDoesNotAnswerIsMadeUnavailable(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	socketDir := filepath.Join(testutil.SocketDirectory(t), "run")
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

// heldResultReader is a node's byte socket holding results, by token.
type heldResultReader struct {
	held map[string]nodeproto.Result
}

func (reader heldResultReader) ReadHeldResult(
	_ context.Context, _ int64, posted nodeproto.Result,
) (nodeproto.Result, error) {
	result, found := reader.held[posted.AttemptToken]
	if !found {
		return nodeproto.Result{}, errors.New("the node answered 404 Not Found")
	}
	return result, nil
}

// A post naming a result the node holds, too large to post, is taken as the whole result it
// fetches: the sink records that; a held result it cannot fetch leaves the post unacknowledged.
func TestResult_AHeldResultIsFetchedWholeBeforeItIsTaken(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	sink := &recordingResultSink{}
	harness.service.SetResultSink(sink)
	whole := nodeproto.Result{JobID: 4, AttemptToken: "a4", Artwork: &nodeproto.ArtworkBatchResult{
		Images: []nodeproto.ArtworkResult{{SourceWidth: 4000}},
	}}
	harness.service.SetHeldResultReader(heldResultReader{held: map[string]nodeproto.Result{"a4": whole}})
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	post := func(token string) int {
		encoded, err := json.Marshal(nodeproto.Result{
			JobID: 4, AttemptToken: token, Held: &nodeproto.HeldResult{SizeBytes: 5 << 20, Sha256: "ab"},
		})
		if err != nil {
			t.Fatal(err)
		}
		return harness.post(t, nodeproto.PathResult, reply.NodeID, bytes.NewReader(encoded)).status
	}
	if status := post("a5"); status != http.StatusInternalServerError || len(sink.results) != 0 {
		t.Errorf("a held result the node could not serve: %d, sink %+v; want 500 and nothing taken", status,
			sink.results)
	}
	if status := post("a4"); status != http.StatusOK || len(sink.results) != 1 || sink.results[0].Artwork == nil ||
		sink.results[0].Artwork.Images[0].SourceWidth != 4000 {
		t.Errorf("a held result: %d, sink %+v; want the whole result taken", status, sink.results)
	}
}

// A held result's bytes are taken only at the size and hash its post named, for the same job and
// attempt.
func TestTakeHeld_TakesOnlyTheBytesThePostNamedForItsAttempt(t *testing.T) {
	t.Parallel()
	encode := func(result nodeproto.Result) []byte {
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		return encoded
	}
	named := func(encoded []byte) nodeproto.Result {
		sum := sha256.Sum256(encoded)
		return nodeproto.Result{JobID: 4, AttemptToken: "a4", Held: &nodeproto.HeldResult{
			SizeBytes: int64(len(encoded)), Sha256: hex.EncodeToString(sum[:]),
		}}
	}
	own := encode(nodeproto.Result{JobID: 4, AttemptToken: "a4", ReadThrough: &nodeproto.ReadThroughResult{}})
	if taken, err := takeHeld(named(own), own); err != nil || taken.ReadThrough == nil {
		t.Errorf("its own bytes: %+v, %v; want the result", taken, err)
	}
	if _, err := takeHeld(named(own), append(own, ' ')); !errors.Is(err, ErrMalformedResult) {
		t.Errorf("bytes of another size: %v, want ErrMalformedResult", err)
	}
	altered := bytes.Replace(own, []byte(`"a4"`), []byte(`"b4"`), 1)
	if _, err := takeHeld(named(own), altered); !errors.Is(err, ErrMalformedResult) {
		t.Errorf("bytes of another hash: %v, want ErrMalformedResult", err)
	}
	if _, err := takeHeld(named(altered), altered); !errors.Is(err, ErrMalformedResult) {
		t.Errorf("another attempt's result: %v, want ErrMalformedResult", err)
	}
}

// The core fetches a held result from the node's byte socket by its attempt's token, and takes it
// as the bytes its post named.
func TestReadHeldResult_FetchesItFromTheNodesByteSocket(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	socketDir := filepath.Join(testutil.SocketDirectory(t), "run")
	// The core reads from the socket the node's hello names (D242 §7).
	reply := harness.hello(t, nodeproto.Hello{Name: "local", ByteSocketPath: filepath.Join(socketDir, "node.sock")})
	encoded, err := json.Marshal(nodeproto.Result{
		JobID: 4, AttemptToken: "a4", ReadThrough: &nodeproto.ReadThroughResult{},
	})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := unixsocket.Listen(socketDir, "node.sock")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+nodeproto.PathBytesResult+"a4", func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(encoded) //nolint:errcheck // the test reads what arrives
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(listener) }() //nolint:errcheck // closed at cleanup
	t.Cleanup(func() { _ = server.Close() })   //nolint:errcheck // as above
	sum := sha256.Sum256(encoded)
	posted := nodeproto.Result{JobID: 4, AttemptToken: "a4", Held: &nodeproto.HeldResult{
		SizeBytes: int64(len(encoded)), Sha256: hex.EncodeToString(sum[:]),
	}}
	client := NewByteSocketClient(harness.service, nil)
	result, err := client.ReadHeldResult(t.Context(), reply.NodeID, posted)
	if err != nil || result.ReadThrough == nil || result.JobID != 4 {
		t.Errorf("the held result = %+v, %v; want job 4's measurement", result, err)
	}
	posted.AttemptToken = "a5"
	if _, err := client.ReadHeldResult(t.Context(), reply.NodeID, posted); err == nil {
		t.Error("a result the node does not hold was taken")
	}
}

// The core asks a node for the folders in a folder on its byte socket, outside every queue, and takes
// its answer; a refusal is an error (D359 §2).
func TestListFolders_AsksTheNodesByteSocket(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	socketDir := filepath.Join(testutil.SocketDirectory(t), "run")
	reply := harness.hello(t, nodeproto.Hello{Name: "local", ByteSocketPath: filepath.Join(socketDir, "node.sock")})
	listener, err := unixsocket.Listen(socketDir, "node.sock")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+nodeproto.PathBytesFolders, func(writer http.ResponseWriter, request *http.Request) {
		path := request.URL.Query().Get("path")
		if path != "/media/video" {
			http.Error(writer, "not a root's", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(writer).Encode(nodeproto.FolderListing{ //nolint:errcheck // the test reads what arrives
			Path: path, State: nodeproto.ProbeReadable, Folders: []string{"movies", "shows"},
		})
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(listener) }() //nolint:errcheck // closed at cleanup
	t.Cleanup(func() { _ = server.Close() })   //nolint:errcheck // as above
	client := NewByteSocketClient(harness.service, nil)
	listing, err := client.ListFolders(t.Context(), reply.NodeID, "/media/video")
	if err != nil || listing.State != nodeproto.ProbeReadable ||
		!slices.Equal(listing.Folders, []string{"movies", "shows"}) {
		t.Errorf("the listing = %+v, %v; want movies and shows", listing, err)
	}
	if _, err := client.ListFolders(t.Context(), reply.NodeID, "/etc"); err == nil {
		t.Error("a listing the node refused was taken")
	}
}

// The core probes a node's root on its byte socket, outside every queue, and takes its answer; a
// node that does not answer in time leaves the probe failed, not waited on (D327).
func TestProbeRoot_AsksTheNodesByteSocket(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	socketDir := filepath.Join(testutil.SocketDirectory(t), "run")
	reply := harness.hello(t, nodeproto.Hello{Name: "local", ByteSocketPath: filepath.Join(socketDir, "node.sock")})
	listener, err := unixsocket.Listen(socketDir, "node.sock")
	if err != nil {
		t.Fatal(err)
	}
	hold := make(chan struct{})
	t.Cleanup(func() { close(hold) })
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+nodeproto.PathBytesProbe, func(writer http.ResponseWriter, request *http.Request) {
		root := request.URL.Query().Get("path")
		if root == "/media/away" {
			select { // its disk does not answer
			case <-hold:
			case <-request.Context().Done():
			}
		}
		_ = json.NewEncoder(writer).Encode(nodeproto.PlaceProbe{ //nolint:errcheck // the test reads what arrives
			Path: root, State: nodeproto.ProbeReadable, Device: 41, IsMountPoint: true,
		})
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(listener) }() //nolint:errcheck // closed at cleanup
	t.Cleanup(func() { _ = server.Close() })   //nolint:errcheck // as above
	client := NewByteSocketClient(harness.service, nil)
	probe, err := client.ProbePlace(t.Context(), reply.NodeID, "/media/films")
	if err != nil || probe.State != nodeproto.ProbeReadable || probe.Path != "/media/films" || probe.Device != 41 ||
		!probe.IsMountPoint {
		t.Errorf("the probe = %+v, %v; want /media/films readable on device 41, a mount point", probe, err)
	}
	bounded, cancel := context.WithCancel(t.Context())
	cancel() // the caller's wait is over
	if _, err := client.ProbePlace(bounded, reply.NodeID, "/media/away"); err == nil {
		t.Error("a probe the caller stopped waiting on answered; want it failed")
	}
}
