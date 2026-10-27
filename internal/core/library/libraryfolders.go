// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/pathtree"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// A library's folders (D359): one of a node's configured roots, or any folder beneath one, chosen by
// an admin browsing the node's folders. A folder narrows a configured root and never widens it
// (D107): the core judges the path's text, the node that it is a real folder.

// FolderLister asks a node for the folders in a folder, on its byte socket, outside every queue:
// nodes.ByteSocketClient (D359 §2).
type FolderLister interface {
	ListFolders(ctx context.Context, nodeID int64, path string) (nodeproto.FolderListing, error)
}

// folderListingWait bounds how long an admin's page waits on a node's listing of a folder: a folder
// on a disk that answers lists in milliseconds, and one asleep spins up within it, so one not
// answered by then is a share that has gone away, answered "not responding" while the node's listing
// goes on for the next ask to share (D359 §2).
const folderListingWait = 5 * time.Second

// ListFoldersThrough has the admin service list a node's folders through lister, which is made after
// it: the byte socket's client.
func (admin *Admin) ListFoldersThrough(lister FolderLister) { admin.folderLister = lister }

// UseLogger has the admin service log to logger: an admin's folder listings and the library folders
// it refuses (D359 §5). Without, it logs nothing.
func (admin *Admin) UseLogger(logger *slog.Logger) { admin.logger = logger }

// log is the admin service's logger, or one that discards.
func (admin *Admin) log() *slog.Logger {
	if admin.logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return admin.logger
}

// Folders are the folders in a folder on the named node, for an admin choosing a library folder: one
// of the node's configured roots or a folder inside one, as its node lists them, links and hidden
// folders left out (D359 §2). A folder the node did not list within folderListingWait is not
// answering, the page never left waiting on a share that has gone away.
func (admin *Admin) Folders(ctx context.Context, nodeName, path string) (nodeproto.FolderListing, error) {
	nodeID, err := admin.libraryFolderOn(ctx, nodeName, path)
	if err != nil {
		return nodeproto.FolderListing{}, err
	}
	startedAt := admin.clock.Now()
	listing := nodeproto.FolderListing{Path: path, State: nodeproto.ProbeNotAnswering, Failure: "nothing lists folders"}
	if admin.folderLister != nil {
		listCtx, cancel := context.WithTimeout(ctx, cmp.Or(admin.folderListingWait, folderListingWait))
		listing, err = admin.folderLister.ListFolders(listCtx, nodeID, path)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return nodeproto.FolderListing{}, ctx.Err()
			}
			listing = nodeproto.FolderListing{Path: path, State: nodeproto.ProbeNotAnswering, Failure: err.Error()}
		}
	}
	admin.log().DebugContext(ctx, "listed the folders in a folder on a node", "nodeName", nodeName,
		"nodeId", nodeID, "path", path, "state", listing.State, "folderCount", len(listing.Folders),
		"tookMs", admin.clock.Now().Sub(startedAt).Milliseconds(), "failure", listing.Failure)
	return listing, nil
}

// AddRoot adds a folder to a library: one of a node's configured roots or a folder inside one, which no
// library covers yet, nor any folder above or below it, and asks for its scan (D359 §3). The node is
// any connected one, the library's own or another.
func (admin *Admin) AddRoot(ctx context.Context, libraryID int64, nodeName, path string) error {
	if _, err := admin.libraryRoots(ctx, libraryID); err != nil {
		return err
	}
	nodeID, err := admin.libraryFolderOn(ctx, nodeName, path)
	if err != nil {
		return err
	}
	if coveredErr := admin.refuseCovered(ctx, nodeID, nodeName, path, ""); coveredErr != nil {
		return coveredErr
	}
	err = admin.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.InsertLibraryRoot(ctx, sqlcgen.InsertLibraryRootParams{
			LibraryID: libraryID, NodeID: nodeID, Path: path,
		})
	})
	if err != nil {
		return fmt.Errorf("add %s on node %q to library %d: %w", path, nodeName, libraryID, err)
	}
	admin.log().InfoContext(ctx, "a folder was added to a library", "libraryId", libraryID, "nodeName", nodeName,
		"path", path)
	return admin.scans.Request(ctx, nodeID, path, jobs.PriorityPersonsRequest)
}

// libraryFolderOn is the id of the connected node named, once path is a library folder on it: one of
// the roots its hello named, or a folder inside one by a clean path, no "..", "." or doubled or
// trailing separator in it (D107, D359 §1). Whether it is a real folder, reached without a link, is
// the node's to say, at each listing. A refusal is logged at warn.
func (admin *Admin) libraryFolderOn(ctx context.Context, nodeName, path string) (int64, error) {
	nodeID, roots, connected := admin.nodes.ConnectedNodeRoots(ctx, nodeName)
	if !connected {
		return 0, fmt.Errorf("node %q: %w", nodeName, ErrNodeNotConnected)
	}
	if _, isHeld := pathtree.DeepestHolding(path, roots); !isHeld {
		admin.log().WarnContext(ctx, "refused a library folder that no configured root of its node holds",
			"nodeName", nodeName, "path", path, "rootPaths", roots)
		return 0, fmt.Errorf("%s on node %q: %w; it names %v", path, nodeName, ErrNotANodeRoot, roots)
	}
	return nodeID, nil
}

// refuseCovered refuses path on the node when a live library's folder there is at it, above it or
// below it (ErrRootCovered): a file is one library's, read by one scan. The folder a repoint moves away
// from, except, does not count; empty for none.
func (admin *Admin) refuseCovered(ctx context.Context, nodeID int64, nodeName, path, except string) error {
	roots, err := admin.database.Read().ListLiveRootsOfNode(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("read node %q's library folders: %w", nodeName, err)
	}
	for _, root := range roots {
		if root.RootPath == except {
			continue
		}
		if pathtree.IsAtOrUnder(path, root.RootPath) || pathtree.IsAtOrUnder(root.RootPath, path) {
			admin.log().WarnContext(ctx, "refused a library folder another covers", "nodeName", nodeName,
				"path", path, "coveredBy", root.RootPath, "libraryId", root.LibraryID)
			return fmt.Errorf("%s on node %q: %w: library %d has %s", path, nodeName, ErrRootCovered,
				root.LibraryID, root.RootPath)
		}
	}
	return nil
}

// refuseOverlapping refuses folders, a new library's, when one is at, above or below another of them.
func refuseOverlapping(nodeName string, folders []string) error {
	for index, folder := range folders {
		for _, other := range folders[index+1:] {
			if pathtree.IsAtOrUnder(folder, other) || pathtree.IsAtOrUnder(other, folder) {
				return fmt.Errorf("%s and %s on node %q: %w", folder, other, nodeName, ErrRootCovered)
			}
		}
	}
	return nil
}

// watchOf is the watch of the configured root holding a library folder, as its node's status said,
// and whether it said one (D272 §9, D359 §4).
func watchOf(watches map[string]nodeproto.RootWatch, folder string) (nodeproto.RootWatch, bool) {
	roots := make([]string, 0, len(watches))
	for root := range watches {
		roots = append(roots, root)
	}
	root, isHeld := pathtree.DeepestHolding(folder, roots)
	if !isHeld {
		return nodeproto.RootWatch{}, false
	}
	return watches[root], true
}
