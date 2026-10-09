// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Refusals an admin's request can meet.
var (
	// ErrNoLibrary is a library that does not exist or was removed.
	ErrNoLibrary = errors.New("no such library")
	// ErrNodeNotConnected is a node that has not said hello since the server started, so its
	// roots are not known.
	ErrNodeNotConnected = errors.New("the node has not connected since the server started")
	// ErrNotANodeRoot is a root the node did not configure (D107).
	ErrNotANodeRoot = errors.New("not one of the node's configured roots")
	// ErrNoSuchRoot is a root the library does not cover.
	ErrNoSuchRoot = errors.New("the library has no such root")
	// ErrNothingWithheld is a root whose removals are not waiting for the admin.
	ErrNothingWithheld = errors.New("no removals wait for the admin at that root")
	// ErrMountNotMissing is a path that is not a mount boundary known to be missing.
	ErrMountNotMissing = errors.New("not a disk known to be missing from a mount point in the library")
)

// Nodes is what the admin service asks of the node channel: the roots a connected node named, by
// name (D207), and removing a node an admin retired.
type Nodes interface {
	NodeConnections
	ConnectedNodeRoots(ctx context.Context, name string) (int64, []string, bool)
	RemoveNode(ctx context.Context, nodeID int64) error
	// RootWatches are whether a node watches each of its roots, as its last status said (D272 §9).
	RootWatches(nodeID int64) map[string]nodeproto.RootWatch
}

// ScanRequests are how an admin asks for a root's scan, and sees those in progress: Scans (D240).
type ScanRequests interface {
	Request(ctx context.Context, nodeID int64, root string, priority int64) error
	InProgress(ctx context.Context) (map[ScanRoot]ScanProgress, error)
}

// Admin is what an admin does with libraries: create one, list them, ask for a scan, and see
// what the scanner saw (D163, D184), and retire a root, a node or a library (D231 §7).
type Admin struct {
	database   *store.Database
	clock      clock.Clock
	nodes      Nodes
	scans      ScanRequests
	retirement Retirement
}

// NewAdmin returns the admin service; scans is how it asks for a scan, and retirement what a retire
// asks of other packages.
func NewAdmin(
	database *store.Database, clockSource clock.Clock, nodes Nodes, scans ScanRequests, retirement Retirement,
) *Admin {
	return &Admin{database: database, clock: clockSource, nodes: nodes, scans: scans, retirement: retirement}
}

// NodeRoots is one enrolled node, and the roots its last hello named when it is connected: what
// a new library's roots are chosen from (D107; audit-2026-09-25, proposal 5).
type NodeRoots struct {
	ID          int64
	Name        string
	IsConnected bool
	Roots       []string
}

// NodeRoots is every enrolled node, by name, with the roots of those connected.
func (admin *Admin) NodeRoots(ctx context.Context) ([]NodeRoots, error) {
	enrolled, err := admin.database.Read().ListLiveNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the nodes: %w", err)
	}
	listed := make([]NodeRoots, 0, len(enrolled))
	for _, node := range enrolled {
		_, roots, connected := admin.nodes.ConnectedNodeRoots(ctx, node.Name)
		if roots == nil {
			roots = []string{}
		}
		listed = append(listed, NodeRoots{ID: node.ID, Name: node.Name, IsConnected: connected, Roots: roots})
	}
	return listed, nil
}

// CreateRequest is a library an admin asks for: the three questions answered (D184), and the roots of
// one node, by the node's name.
type CreateRequest struct {
	NewLibrary
	NodeName string
}

// Create adds a library over roots the node configured, then asks the node to scan them.
func (admin *Admin) Create(ctx context.Context, request CreateRequest) (int64, error) {
	nodeID, roots, connected := admin.nodes.ConnectedNodeRoots(ctx, request.NodeName)
	if !connected {
		return 0, fmt.Errorf("node %q: %w", request.NodeName, ErrNodeNotConnected)
	}
	for _, root := range request.Roots {
		if !slices.Contains(roots, root) {
			return 0, fmt.Errorf("%s on node %q: %w; it names %v", root, request.NodeName, ErrNotANodeRoot, roots)
		}
	}
	library := request.NewLibrary
	library.NodeID = nodeID
	id, err := Create(ctx, admin.database, admin.clock, library)
	if err != nil {
		return 0, err
	}
	for _, root := range library.Roots {
		if err := admin.scans.Request(ctx, nodeID, root, jobs.PriorityPersonsRequest); err != nil {
			return id, err
		}
	}
	return id, nil
}

// Summary is a library with its roots' states and its files counted by filing.
type Summary struct {
	sqlcgen.ListLibrariesRow
	Roots             []RootSummary
	FileCountByFiling map[string]int64
}

// RootSummary is a root's last scan, the one in progress, if one is, and the mount points inside it
// whose disk a scan found missing: each is what ForgetMount takes, when the admin says that disk is
// gone (D208).
type RootSummary struct {
	sqlcgen.ListLibraryRootStatesRow
	Scan              *ScanProgress
	MissingMountPaths []string
	// Watch is whether the root's node watches it for changes, absent until the node has said
	// (D272 §9).
	Watch *nodeproto.RootWatch
}

// List is every live library reach reaches, by name (D243 §4).
func (admin *Admin) List(ctx context.Context, reach auth.Reach) ([]Summary, error) {
	queries := admin.database.Read()
	libraries, err := queries.ListLibraries(ctx)
	if err != nil {
		return nil, fmt.Errorf("list libraries: %w", err)
	}
	scanning, err := admin.scans.InProgress(ctx)
	if err != nil {
		return nil, err
	}
	summaries := make([]Summary, 0, len(libraries))
	for _, library := range libraries {
		if !reach.Reaches(&library.ID) {
			continue
		}
		roots, err := rootSummaries(ctx, queries, library.ID)
		if err != nil {
			return nil, err
		}
		for index, root := range roots {
			if progress, isScanning := scanning[ScanRoot{NodeID: root.NodeID, RootPath: root.RootPath}]; isScanning {
				roots[index].Scan = &progress
			}
			if watch, isSaid := admin.nodes.RootWatches(root.NodeID)[root.RootPath]; isSaid {
				roots[index].Watch = &watch
			}
		}
		counts, err := queries.CountLibraryFilings(ctx, library.ID)
		if err != nil {
			return nil, fmt.Errorf("count library %d's files: %w", library.ID, err)
		}
		filings := map[string]int64{}
		for _, count := range counts {
			filings[count.Filing] = count.Files
		}
		summaries = append(summaries, Summary{ListLibrariesRow: library, Roots: roots, FileCountByFiling: filings})
	}
	return summaries, nil
}

// rootSummaries are a library's roots, each with its missing mounts.
func rootSummaries(ctx context.Context, queries *sqlcgen.Queries, libraryID int64) ([]RootSummary, error) {
	roots, err := queries.ListLibraryRootStates(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("list library %d's roots: %w", libraryID, err)
	}
	summaries := make([]RootSummary, 0, len(roots))
	for _, root := range roots {
		mounts, err := queries.ListRootMounts(ctx, sqlcgen.ListRootMountsParams{
			NodeID: root.NodeID, RootPath: root.RootPath,
		})
		if err != nil {
			return nil, fmt.Errorf("list the mounts under %s: %w", root.RootPath, err)
		}
		missing := []string{}
		for _, mount := range mounts {
			if mount.IsPresent == 0 {
				missing = append(missing, mount.MountPath)
			}
		}
		summaries = append(summaries, RootSummary{ListLibraryRootStatesRow: root, MissingMountPaths: missing})
	}
	return summaries, nil
}

// Scan asks each root's node to scan it, and reports how many roots were asked. A root whose
// node is not connected is skipped: it scans when the node says hello, where should_scan_on_node_hello
// says so.
func (admin *Admin) Scan(ctx context.Context, libraryID int64) (int, error) {
	roots, err := admin.libraryRoots(ctx, libraryID)
	if err != nil {
		return 0, err
	}
	asked := 0
	for _, root := range roots {
		isAsked, err := admin.rescan(ctx, root)
		if err != nil {
			return asked, err
		}
		if isAsked {
			asked++
		}
	}
	return asked, nil
}

// FilesQuery pages through what the scanner saw (D163).
type FilesQuery struct {
	LibraryID *int64
	Filing    *string
	Cursor    string // the page before's NextCursor; empty for the first
	Limit     int64
}

// FilesPage is one page of live files, by id, and the cursor of the next when it is full.
type FilesPage struct {
	Files      []sqlcgen.ListFilesPageRow
	NextCursor string
}

// Files is one page of live files, by id, behind the same opaque cursor as the browse lists.
func (admin *Admin) Files(ctx context.Context, query FilesQuery) (FilesPage, error) {
	key, afterID, err := decodeCursor(query.Cursor)
	if err != nil || len(key) != 0 || afterID < 0 {
		return FilesPage{}, ErrBadCursor
	}
	rows, err := admin.database.Read().ListFilesPage(ctx, sqlcgen.ListFilesPageParams{
		AfterID: afterID, LibraryID: query.LibraryID, Filing: query.Filing, MaxRows: query.Limit,
	})
	if err != nil {
		return FilesPage{}, fmt.Errorf("list files: %w", err)
	}
	page := FilesPage{Files: rows}
	if int64(len(rows)) == query.Limit && len(rows) > 0 {
		page.NextCursor = encodeCursor(nil, rows[len(rows)-1].ID)
	}
	return page, nil
}

// ApproveRemovals is the admin saying a root's withheld removals are real (D208 §3): its next
// complete scan removes what it does not see without waiting again. The root is scanned now if
// its node is connected; it reports whether it was.
func (admin *Admin) ApproveRemovals(ctx context.Context, libraryID int64, nodeName, path string) (bool, error) {
	root, err := admin.findRoot(ctx, libraryID, nodeName, path)
	if err != nil {
		return false, err
	}
	if root.LastScanOutcome == nil || *root.LastScanOutcome != outcomeWithheld {
		return false, fmt.Errorf("%s on %s: %w", path, nodeName, ErrNothingWithheld)
	}
	err = admin.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		_, approveErr := queries.ApproveRootRemovals(ctx, sqlcgen.ApproveRootRemovalsParams{
			NodeID: root.NodeID, RootPath: path,
		})
		return approveErr
	})
	if err != nil {
		return false, fmt.Errorf("approve %s's removals: %w", path, err)
	}
	return admin.rescan(ctx, root)
}

// ForgetMount is the admin saying a disk missing from a mount point inside a root is gone for
// good: its boundary is forgotten and the root's removals approved, so the next complete scan
// removes its files (D208).
func (admin *Admin) ForgetMount(ctx context.Context, libraryID int64, nodeName, mountPath string) (bool, error) {
	nodeID, err := admin.libraryNodeIDByName(ctx, libraryID, nodeName)
	if err != nil {
		return false, err
	}
	mount, err := store.OrNotFound(admin.database.Read().GetRootMount(ctx, sqlcgen.GetRootMountParams{
		NodeID: nodeID, MountPath: mountPath,
	}))
	if errors.Is(err, store.ErrNotFound) || (err == nil && mount.IsPresent != 0) {
		return false, fmt.Errorf("%s on %s: %w", mountPath, nodeName, ErrMountNotMissing)
	}
	if err != nil {
		return false, fmt.Errorf("read the mount %s: %w", mountPath, err)
	}
	root, err := admin.findRoot(ctx, libraryID, nodeName, mount.RootPath)
	if err != nil {
		return false, fmt.Errorf("%s on %s: %w", mountPath, nodeName, ErrMountNotMissing)
	}
	err = admin.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		forget := sqlcgen.ForgetMissingRootMountParams{NodeID: nodeID, MountPath: mountPath}
		if _, forgetErr := queries.ForgetMissingRootMount(ctx, forget); forgetErr != nil {
			return fmt.Errorf("forget the mount: %w", forgetErr)
		}
		approve := sqlcgen.ApproveRootRemovalsParams{NodeID: nodeID, RootPath: mount.RootPath}
		if _, approveErr := queries.ApproveRootRemovals(ctx, approve); approveErr != nil {
			return fmt.Errorf("approve the root's removals: %w", approveErr)
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("forget %s: %w", mountPath, err)
	}
	return admin.rescan(ctx, root)
}

// libraryRoots are a live library's roots with their states.
func (admin *Admin) libraryRoots(ctx context.Context, libraryID int64) ([]sqlcgen.ListLibraryRootStatesRow, error) {
	queries := admin.database.Read()
	if _, err := store.OrNotFound(queries.GetLibrary(ctx, libraryID)); errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("library %d: %w", libraryID, ErrNoLibrary)
	} else if err != nil {
		return nil, fmt.Errorf("read library %d: %w", libraryID, err)
	}
	roots, err := queries.ListLibraryRootStates(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("list library %d's roots: %w", libraryID, err)
	}
	return roots, nil
}

// findRoot is one of a library's roots, by its node's name and its path.
func (admin *Admin) findRoot(
	ctx context.Context, libraryID int64, nodeName, path string,
) (sqlcgen.ListLibraryRootStatesRow, error) {
	roots, err := admin.libraryRoots(ctx, libraryID)
	if err != nil {
		return sqlcgen.ListLibraryRootStatesRow{}, err
	}
	for _, root := range roots {
		if root.NodeName == nodeName && root.RootPath == path {
			return root, nil
		}
	}
	return sqlcgen.ListLibraryRootStatesRow{}, fmt.Errorf("%s on %s: %w", path, nodeName, ErrNoSuchRoot)
}

// libraryNodeIDByName is the id of the named node, which must hold one of the library's roots.
func (admin *Admin) libraryNodeIDByName(ctx context.Context, libraryID int64, nodeName string) (int64, error) {
	roots, err := admin.libraryRoots(ctx, libraryID)
	if err != nil {
		return 0, err
	}
	for _, root := range roots {
		if root.NodeName == nodeName {
			return root.NodeID, nil
		}
	}
	return 0, fmt.Errorf("node %s: %w", nodeName, ErrNoSuchRoot)
}

// rescan asks for a root's scan, if its node is connected, and reports whether it did.
func (admin *Admin) rescan(ctx context.Context, root sqlcgen.ListLibraryRootStatesRow) (bool, error) {
	nodeID, _, connected := admin.nodes.ConnectedNodeRoots(ctx, root.NodeName)
	if !connected || nodeID != root.NodeID {
		return false, nil
	}
	if err := admin.scans.Request(ctx, nodeID, root.RootPath, jobs.PriorityPersonsRequest); err != nil {
		return false, err
	}
	return true, nil
}
