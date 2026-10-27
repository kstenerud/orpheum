// SPDX-License-Identifier: AGPL-3.0-or-later

package byteserver

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"sync"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// sharedReads runs at most one read of each place at a time, every ask meanwhile sharing its answer:
// a read of a share that is gone waits in the kernel, and each one more left waiting is one more
// thread held, however soon it is asked again (D327 §2). The probes and the folder listings
// (D359 §2) each have their own.
type sharedReads[Answer any] struct {
	mutex   sync.Mutex
	running map[string]*sharedRead[Answer] // guarded by mutex: by place
}

// sharedRead is one read running, its answer ready once done is closed.
type sharedRead[Answer any] struct {
	done   chan struct{}
	answer Answer
	// asks are how many asks share it; guarded by the sharedReads' mutex.
	asks int
}

// join is the read of place running, or one started by read when none is.
func (running *sharedReads[Answer]) join(place string, read func() Answer) *sharedRead[Answer] {
	running.mutex.Lock()
	defer running.mutex.Unlock()
	if call, isRunning := running.running[place]; isRunning {
		call.asks++
		return call
	}
	if running.running == nil {
		running.running = map[string]*sharedRead[Answer]{}
	}
	call := &sharedRead[Answer]{done: make(chan struct{}), asks: 1}
	running.running[place] = call
	go func() {
		call.answer = read()
		running.mutex.Lock()
		delete(running.running, place)
		running.mutex.Unlock()
		close(call.done)
	}()
	return call
}

// answerShared waits for a shared read's answer and writes it as JSON, unless the ask leaves first,
// leaving the read running for the next ask to share.
func answerShared[Answer any](writer http.ResponseWriter, request *http.Request, call *sharedRead[Answer]) {
	select {
	case <-call.done:
	case <-request.Context().Done():
		return
	}
	encoded, err := json.Marshal(call.answer)
	if err != nil {
		http.Error(writer, "the answer could not be written", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	// The node's own JSON, on its private socket, to the core: no page renders it.
	_, _ = writer.Write(encoded) //nolint:errcheck // a core that hung up asks again
}

// serveProbe answers nodeproto.PathBytesProbe: whether a place, one of the node's roots or a
// directory inside one, can be read now, without a slot, a reader or a queue, its facts measured and
// nothing under it opened (D327 §1, D328 §6). An ask that leaves before the probe ends leaves it
// running, for the next ask to share.
func (server *Server) serveProbe(writer http.ResponseWriter, request *http.Request) {
	path := request.URL.Query().Get("path")
	root, relative, isPlace := server.placeOf(path)
	if !isPlace {
		http.Error(writer, "the place is not one of this node's roots or inside one", http.StatusNotFound)
		return
	}
	ctx := context.WithoutCancel(request.Context())
	call := server.probes.join(path, func() nodeproto.PlaceProbe { return server.probe(ctx, path, root, relative) })
	answerShared(writer, request, call)
}

// placeOf is the root that holds path, and path relative to it; false when no root does, or path is
// not clean (confine.Holding).
func (server *Server) placeOf(path string) (*confine.MediaRoot, string, bool) {
	return confine.Holding(server.roots, path)
}

// probe measures a place's facts through the disk gate, keyed by its root: at once not answering
// when the root holds a reading that has not returned, and otherwise within the window a file's open
// has. The facts end in a statfs, which a network share answers only from its server, where a stat
// may answer from the attribute cache for a minute after the server is gone
// (design/research/nfs-client-recovery-2026-10-21.md): the statfs is what makes a readable answer
// mean the share is there.
func (server *Server) probe(
	ctx context.Context, path string, root *confine.MediaRoot, relative string,
) nodeproto.PlaceProbe {
	answer := nodeproto.PlaceProbe{Path: path}
	var facts confine.Facts
	var err error
	if server.disks == nil {
		facts, err = server.placeFacts(root, relative)
	} else {
		operation := diskwatch.Operation{Root: root.Path(), Path: path, Window: server.openWindow}
		facts, err = diskwatch.Run(ctx, server.disks, operation,
			func() (confine.Facts, error) { return server.placeFacts(root, relative) }, func(confine.Facts) {})
	}
	switch {
	case err == nil:
		answer.State, answer.Device, answer.IsMountPoint = nodeproto.ProbeReadable, facts.FilesystemDevice,
			facts.IsMountPoint
		return answer
	case errors.Is(err, diskwatch.ErrStalled) || diskwatch.IsDiskFailure(err):
		answer.State = nodeproto.ProbeNotAnswering
		if server.disks != nil {
			if stuck, isStuck := server.disks.IsStuck(root.Path(), path); isStuck {
				answer.StuckSinceMs, answer.WaitingOnPath = stuck.Since.UnixMilli(), stuck.Path
			}
		}
	case errors.Is(err, fs.ErrNotExist):
		answer.State = nodeproto.ProbeMissing
	default:
		answer.State = nodeproto.ProbeFailed
	}
	answer.Failure = err.Error()
	return answer
}
