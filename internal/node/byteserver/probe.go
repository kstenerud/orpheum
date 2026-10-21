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
	answer nodeproto.RootProbe
	// asks are how many asks share it; guarded by the probes' mutex.
	asks int
}

// join is the probe of root running, or one started by probe when none is.
func (running *probes) join(root string, probe func() nodeproto.RootProbe) *probeCall {
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

// serveProbe answers nodeproto.PathBytesProbe: whether a root can be read now, without a slot, a
// reader or a queue, its facts measured and nothing under it opened (D327 §1). An ask that leaves
// before the probe ends leaves it running, for the next ask to share.
func (server *Server) serveProbe(writer http.ResponseWriter, request *http.Request) {
	rootPath := request.URL.Query().Get("rootPath")
	root, isRoot := server.roots[rootPath]
	if !isRoot {
		http.Error(writer, "the root is not one of this node's", http.StatusNotFound)
		return
	}
	ctx := context.WithoutCancel(request.Context())
	call := server.probes.join(rootPath, func() nodeproto.RootProbe { return server.probe(ctx, rootPath, root) })
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

// probe measures root's facts through the disk gate: at once not answering when the root holds a
// reading that has not returned, and otherwise within the window a file's open has. The facts end in
// a statfs, which a network share answers only from its server, where a stat may answer from the
// attribute cache for a minute after the server is gone (design/research/nfs-client-recovery-2026-10-21.md): the
// statfs is what makes a readable answer mean the share is there.
func (server *Server) probe(ctx context.Context, rootPath string, root *confine.MediaRoot) nodeproto.RootProbe {
	answer := nodeproto.RootProbe{RootPath: rootPath}
	var facts confine.Facts
	var err error
	if server.disks == nil {
		facts, err = server.rootFacts(root)
	} else {
		operation := diskwatch.Operation{Root: rootPath, Path: rootPath, Window: server.openWindow}
		facts, err = diskwatch.Run(ctx, server.disks, operation,
			func() (confine.Facts, error) { return server.rootFacts(root) }, func(confine.Facts) {})
	}
	switch {
	case err == nil:
		answer.State, answer.Device, answer.IsMountPoint = nodeproto.ProbeReadable, facts.FilesystemDevice,
			facts.IsMountPoint
		return answer
	case errors.Is(err, diskwatch.ErrStalled) || diskwatch.IsDiskFailure(err):
		answer.State = nodeproto.ProbeNotAnswering
		if server.disks != nil {
			if stuck, isStuck := server.disks.IsStuck(rootPath); isStuck {
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
