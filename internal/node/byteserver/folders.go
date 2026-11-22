// SPDX-License-Identifier: AGPL-3.0-or-later

package byteserver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	pathpkg "path"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/videofile"
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
		answer.VideoCounts = server.countFolders(ctx, path, root, relative, folders)
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
			"folderCount", len(answer.Folders), "countedCount", len(answer.VideoCounts),
			"tookMs", server.clock.Now().Sub(startedAt).Milliseconds(),
			"failure", answer.Failure)
	}
	return answer
}

// folderCountBudget is how long a listing's counts of its folders' videos may take, all of them
// together: an admin choosing a folder waits on the listing, so a folder not counted within it is
// left uncounted rather than holding the listing up (D539 §7).
const folderCountBudget = 300 * time.Millisecond

// countFolders counts the videos in each of folders, the folders in the folder at relative inside
// root, in their order, within folderCountBudget by the server's countClock: each through the disk
// gate, so a share that has gone away stops the counting, never the listing. A folder whose count
// failed, or did not end within the budget, has none, nor has any folder after the budget ran out:
// no count is ever a part of one. Nil when none was counted.
func (server *Server) countFolders(
	ctx context.Context, path string, root *confine.MediaRoot, relative string, folders []string,
) map[string]int64 {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	budget := server.countClock.NewTimer(folderCountBudget)
	defer budget.Stop()
	go func() {
		select {
		case <-budget.C():
			cancel()
		case <-ctx.Done():
		}
	}()
	var counts map[string]int64
	for _, folder := range folders {
		folderRelative := pathpkg.Join(relative, folder)
		count := func() (int64, error) { return server.countVideos(ctx, root, folderRelative) }
		var videoCount int64
		var err error
		if server.disks == nil {
			videoCount, err = count()
		} else {
			operation := diskwatch.Operation{
				Root: root.Path(), Path: pathpkg.Join(path, folder), Window: server.openWindow,
			}
			videoCount, err = diskwatch.Run(ctx, server.disks, operation, count, func(int64) {})
		}
		if ctx.Err() != nil {
			break
		}
		if err == nil {
			if counts == nil {
				counts = make(map[string]int64, len(folders))
			}
			counts[folder] = videoCount
		}
	}
	return counts
}

// countVideosIn counts the videos in the folder at relative inside root and in every folder below it,
// as a scan of it would read them (videofile): a file a scan reads as a video, in no folder a scan
// skips. A link is not followed, to a file or a folder, so a count may be short of a scan's, which
// follows one that stays inside the roots, but never counts a video twice. It stops with ctx's error
// once ctx ends.
func countVideosIn(ctx context.Context, root *confine.MediaRoot, relative string) (int64, error) {
	var count int64
	var countBelow func(below string) error
	countBelow = func(below string) error {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("count the videos in %s: %w", relative, err)
		}
		entries, err := root.ReadDir(pathpkg.Join(relative, below))
		if err != nil {
			return err
		}
		for _, entry := range entries {
			name := pathpkg.Join(below, entry.Name())
			switch {
			case entry.IsDir() && !videofile.IsSkippedFolder(name):
				if err := countBelow(name); err != nil {
					return err
				}
			case entry.Type().IsRegular() && videofile.IsVideo(entry.Name()):
				count++
			}
		}
		return nil
	}
	if err := countBelow("."); err != nil {
		return 0, err
	}
	return count, nil
}
