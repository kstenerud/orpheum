// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/pathtree"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
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
	admin.log().DebugContext(ctx, "listed the folders in a folder on a node",
		slogx.EventKey, "foldersListed",
		"nodeName", nodeName,
		"nodeId", nodeID, "path", path, "state", listing.State, "folderCount", len(listing.Folders),
		"tookMs", admin.clock.Now().Sub(startedAt).Milliseconds(), "failure", listing.Failure)
	return listing, nil
}

// AddRoot adds a folder to a library: one of a node's configured roots or a folder inside one, and asks
// for its scan (D359 §3). The node is any connected one, the library's own or another. Another
// library's folders may be at, above or below it (D363 §1); within the library, one at or below a folder
// it has adds nothing and is refused as covered, and one above some of its folders takes their place,
// which are retired. A folder of another library's that is no scan's root any more, now that one above
// it is, has its scan state forgotten.
func (admin *Admin) AddRoot(ctx context.Context, libraryID int64, nodeName, path string) error {
	if _, err := admin.libraryRoots(ctx, libraryID); err != nil {
		return err
	}
	nodeID, err := admin.libraryFolderOn(ctx, nodeName, path)
	if err != nil {
		return err
	}
	replaced, err := admin.refuseCovered(ctx, libraryID, nodeID, nodeName, path, "")
	if err != nil {
		return err
	}
	var forgotten []jobs.Destination
	err = admin.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		nowMs := admin.clock.Now().UnixMilli()
		for _, child := range replaced {
			_, retireErr := queries.RetireLibraryRoot(ctx, sqlcgen.RetireLibraryRootParams{
				RetiredAt: &nowMs, LibraryID: libraryID, NodeID: nodeID, RootPath: child,
			})
			if retireErr != nil {
				return fmt.Errorf("retire %s, which %s takes the place of: %w", child, path, retireErr)
			}
		}
		insertErr := queries.InsertLibraryRoot(ctx, sqlcgen.InsertLibraryRootParams{
			LibraryID: libraryID, NodeID: nodeID, Path: path,
		})
		if insertErr != nil {
			return insertErr
		}
		var forgetErr error
		forgotten, forgetErr = forgetRootsNowWithin(ctx, queries, nodeID, path)
		return forgetErr
	})
	if err != nil {
		return fmt.Errorf("add %s on node %q to library %d: %w", path, nodeName, libraryID, err)
	}
	admin.log().InfoContext(ctx, "a folder was added to a library", "libraryId", libraryID, "nodeName", nodeName,
		"path", path, "replacedPaths", replaced)
	admin.watchedFoldersChanged()
	if len(forgotten) > 0 {
		// The work parked on a root that is no scan's any more waits on a destination that is no more;
		// the sweeps derive it again under the root that holds it now.
		if _, err := admin.retirement.Work.CancelRetired(ctx, jobs.RetiredWork{Destinations: forgotten}); err != nil {
			return fmt.Errorf("forget the folders now within %s: %w", path, err)
		}
	}
	return admin.scans.Request(ctx, nodeID, path, jobs.PriorityPersonsRequest)
}

// forgetRootsNowWithin forgets the scan state of each live library folder on a node below folder,
// which a scan of folder's root holds from now on (D363 §2), and returns their destinations.
func forgetRootsNowWithin(
	ctx context.Context, queries *sqlcgen.Queries, nodeID int64, folder string,
) ([]jobs.Destination, error) {
	folders, err := queries.ListLiveRootPathsOnNode(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("list node %d's library folders: %w", nodeID, err)
	}
	var destinations []jobs.Destination
	for _, inner := range folders {
		if inner == folder || !pathtree.IsAtOrUnder(inner, folder) {
			continue
		}
		mounts, err := queries.ListRootMountPaths(ctx,
			sqlcgen.ListRootMountPathsParams{NodeID: nodeID, RootPath: inner})
		if err != nil {
			return nil, fmt.Errorf("list the disks mounted inside %s: %w", inner, err)
		}
		destinations = append(destinations, jobs.RootDestination(nodeID, inner))
		for _, mount := range mounts {
			destinations = append(destinations, jobs.RootDestination(nodeID, mount))
		}
		if err := forgetRootState(ctx, queries, nodeID, inner); err != nil {
			return nil, err
		}
	}
	return destinations, nil
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
			slogx.EventKey, "folderOutsideRoots",
			"nodeName", nodeName, "path", path, "rootPaths", roots)
		return 0, fmt.Errorf("%s on node %q: %w; it names %v", path, nodeName, ErrNotANodeRoot, roots)
	}
	return nodeID, nil
}

// refuseCovered refuses path on the node for a library one of whose folders there is at it or above it
// (ErrRootCovered): within one library, a folder beneath another of its folders adds nothing (D363 §1).
// It returns the library's folders below path, whose place path takes. Another library's folders are
// no matter: libraries may overlap. The folder a repoint moves away from, except, does not count;
// empty for none. A refusal is logged at warn.
func (admin *Admin) refuseCovered(
	ctx context.Context, libraryID, nodeID int64, nodeName, path, except string,
) ([]string, error) {
	roots, err := admin.database.Read().ListLiveRootsOfNode(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("read node %q's library folders: %w", nodeName, err)
	}
	var replaced []string
	for _, root := range roots {
		if root.LibraryID != libraryID || root.RootPath == except {
			continue
		}
		if pathtree.IsAtOrUnder(path, root.RootPath) {
			admin.log().WarnContext(ctx, "refused a library folder its library covers already",
				slogx.EventKey, "folderCovered",
				"nodeName", nodeName,
				"path", path, "coveredBy", root.RootPath, "libraryId", root.LibraryID)
			return nil, fmt.Errorf("%s on node %q: %w: library %d has %s", path, nodeName, ErrRootCovered,
				root.LibraryID, root.RootPath)
		}
		if pathtree.IsAtOrUnder(root.RootPath, path) {
			replaced = append(replaced, root.RootPath)
		}
	}
	return replaced, nil
}

// outermostOfNew are a new library's folders less those beneath another of them, which a parent takes
// the place of within one library (D363 §1); each one dropped is logged.
func (admin *Admin) outermostOfNew(ctx context.Context, nodeName string, folders []string) []string {
	kept := outermostFolders(folders)
	for _, folder := range folders {
		if !slices.Contains(kept, folder) {
			admin.log().InfoContext(ctx, "a new library's folder beneath another of its folders adds nothing",
				slogx.EventKey, "folderNested",
				"nodeName", nodeName, "path", folder)
		}
	}
	return kept
}

// watchedFoldersChanged tells the nodes the library folders they watch may have changed (Nodes); with
// no node channel, as a test of the libraries alone gives, there is no one to tell.
func (admin *Admin) watchedFoldersChanged() {
	if admin.nodes != nil {
		admin.nodes.WatchedFoldersChanged()
	}
}

// watchOf is a library folder's own watch, as its node's status said, and whether it said one: the
// node watches each library folder it is given, never a configured root around it (D272 §9, D359's
// amendment of 2026-10-27).
func watchOf(watches map[string]nodeproto.RootWatch, folder string) (nodeproto.RootWatch, bool) {
	watch, isSaid := watches[folder]
	return watch, isSaid
}
