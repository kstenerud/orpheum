// SPDX-License-Identifier: AGPL-3.0-or-later

package byteserver

import (
	"io/fs"
	"os"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// OpeningWith has the server open each file with open, as a test's disk does.
func (server *Server) OpeningWith(
	open func(roots map[string]*confine.MediaRoot, named nodeproto.BytesFile) (*os.File, fs.FileInfo, error),
) *Server {
	server.openFile = open
	return server
}

// ProbingWith has the server read a place's facts with facts, as a test's disk does.
func (server *Server) ProbingWith(facts func(root *confine.MediaRoot, relative string) (confine.Facts, error)) *Server {
	server.placeFacts = facts
	return server
}

// ProbeAsks are how many asks share the probe of root running; 0 when none runs.
func (server *Server) ProbeAsks(root string) int {
	server.probes.mutex.Lock()
	defer server.probes.mutex.Unlock()
	if call, isRunning := server.probes.running[root]; isRunning {
		return call.asks
	}
	return 0
}

// ListingFoldersWith has the server list a folder's folders with list, as a test's disk does.
func (server *Server) ListingFoldersWith(
	list func(root *confine.MediaRoot, relative string) ([]string, error),
) *Server {
	server.subfolders = list
	return server
}
