// SPDX-License-Identifier: AGPL-3.0-or-later

package byteserver

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// serveFolders answers nodeproto.PathBytesFolders: the folders in a folder, one of the node's roots or
// a folder inside one, for an admin choosing a library folder (D359 §2): no link, even to a folder,
// and no hidden folder. It reads outside every queue, through the disk gate within the window a
// file's open has, so a share that has gone away is answered not answering, never waited on; and asks
// while a listing of the folder runs share it, as probes do. A path no root holds is refused (D107).
func (server *Server) serveFolders(writer http.ResponseWriter, request *http.Request) {
	path := request.URL.Query().Get("path")
	root, relative, isPlace := server.placeOf(path)
	if !isPlace {
		if server.logger != nil {
			server.logger.WarnContext(request.Context(), "refused to list the folders of a path no root holds",
				"path", path)
		}
		http.Error(writer, "the folder is not one of this node's roots or inside one", http.StatusNotFound)
		return
	}
	ctx := context.WithoutCancel(request.Context())
	call := server.folderListings.join(path, func() nodeproto.FolderListing {
		return server.listFolders(ctx, path, root, relative)
	})
	answerShared(writer, request, call)
}

// listFolders lists the folders in the folder at path, relative inside root, through the disk gate
// keyed by the root: at once not answering when the root holds a reading that has not returned.
func (server *Server) listFolders(
	ctx context.Context, path string, root *confine.MediaRoot, relative string,
) nodeproto.FolderListing {
	var startedAt time.Time
	if server.logger != nil {
		startedAt = server.clock.Now()
	}
	answer := nodeproto.FolderListing{Path: path}
	var folders []string
	var err error
	if server.disks == nil {
		folders, err = server.subfolders(root, relative)
	} else {
		operation := diskwatch.Operation{Root: root.Path(), Path: path, Window: server.openWindow}
		folders, err = diskwatch.Run(ctx, server.disks, operation,
			func() ([]string, error) { return server.subfolders(root, relative) }, func([]string) {})
	}
	switch {
	case err == nil:
		answer.State, answer.Folders = nodeproto.ProbeReadable, folders
	case errors.Is(err, diskwatch.ErrStalled) || diskwatch.IsDiskFailure(err):
		answer.State = nodeproto.ProbeNotAnswering
	case errors.Is(err, fs.ErrNotExist):
		answer.State = nodeproto.ProbeMissing
	default:
		answer.State = nodeproto.ProbeFailed
	}
	if err != nil {
		answer.Failure = err.Error()
	}
	if server.logger != nil {
		server.logger.DebugContext(ctx, "listed the folders in a folder", "path", path, "state", answer.State,
			"folderCount", len(answer.Folders), "tookMs", server.clock.Now().Sub(startedAt).Milliseconds(),
			"failure", answer.Failure)
	}
	return answer
}
