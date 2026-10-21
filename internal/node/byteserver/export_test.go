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
