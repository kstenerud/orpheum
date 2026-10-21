// SPDX-License-Identifier: AGPL-3.0-or-later

package byteserver

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// probes runs at most one probe of each root at a time, every ask meanwhile sharing its answer: a
// probe of a share that is gone waits in the kernel, and each one more left waiting is one more
// thread held, however soon it is asked again (D327 §2).
type probes struct {
	mutex   sync.Mutex
	running map[string]*probeCall // guarded by mutex: by root
}

// probeCall is one probe running, its answer ready once done is closed.
type probeCall struct {
	done   chan struct{}
	answer nodeproto.PlaceProbe
	// asks are how many asks share it; guarded by the probes' mutex.
	asks int
}

// join is the probe of root running, or one started by probe when none is.
func (running *probes) join(root string, probe func() nodeproto.PlaceProbe) *probeCall {
	running.mutex.Lock()
	defer running.mutex.Unlock()
	if call, isRunning := running.running[root]; isRunning {
		call.asks++
		return call
	}
	if running.running == nil {
		running.running = map[string]*probeCall{}
	}
	call := &probeCall{done: make(chan struct{}), asks: 1}
	running.running[root] = call
	go func() {
		call.answer = probe()
		running.mutex.Lock()
		delete(running.running, root)
		running.mutex.Unlock()
		close(call.done)
	}()
	return call
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
	select {
	case <-call.done:
	case <-request.Context().Done():
		return
	}
	encoded, err := json.Marshal(call.answer)
	if err != nil {
		http.Error(writer, "the probe's answer could not be written", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	// The node's own JSON, on its private socket, to the core: no page renders it.
	_, _ = writer.Write(encoded) //nolint:errcheck // a core that hung up asks again
}

// placeOf is the root that holds path, and path relative to it; false when no root does.
func (server *Server) placeOf(path string) (*confine.MediaRoot, string, bool) {
	if root, isRoot := server.roots[path]; isRoot {
		return root, ".", true
	}
	if !filepath.IsAbs(path) {
		return nil, "", false
	}
	for _, root := range server.roots {
		if relative, isInside := root.RelativeIfInside(filepath.Clean(path)); isInside {
			return root, relative, true
		}
	}
	return nil, "", false
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
			if stuck, isStuck := server.disks.IsStuck(root.Path()); isStuck {
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
