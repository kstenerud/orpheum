// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"
	"slices"

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
// name (D207), and a command queued to it.
type Nodes interface {
	Roots(name string) (int64, []string, bool)
	Enqueue(nodeID int64, command nodeproto.Command)
}

// Admin is what an admin does with libraries: create one, list them, ask for a scan, and see
// what the scanner saw (D163, D184).
type Admin struct {
	database *store.Database
	clock    clock.Clock
	nodes    Nodes
}

// NewAdmin returns the admin service.
func NewAdmin(database *store.Database, clockSource clock.Clock, nodes Nodes) *Admin {
	return &Admin{database: database, clock: clockSource, nodes: nodes}
}

// NodeRoots is one enrolled node, and the roots its last hello named when it is connected: what
// a new library's roots are chosen from (D107; audit-2026-09-25, proposal 5).
type NodeRoots struct {
	Name      string
	Connected bool
	Roots     []string
}

// NodeRoots is every enrolled node, by name, with the roots of those connected.
func (admin *Admin) NodeRoots(ctx context.Context) ([]NodeRoots, error) {
	enrolled, err := admin.database.Read().ListLiveNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the nodes: %w", err)
	}
	listed := make([]NodeRoots, 0, len(enrolled))
	for _, node := range enrolled {
		_, roots, connected := admin.nodes.Roots(node.Name)
		if roots == nil {
			roots = []string{}
		}
		listed = append(listed, NodeRoots{Name: node.Name, Connected: connected, Roots: roots})
	}
	return listed, nil
}

// Request is a library an admin asks for: the three questions answered (D184), and the roots of
// one node, by the node's name.
type Request struct {
	NewLibrary
	Node string
}

// Create adds a library over roots the node configured, then asks the node to scan them.
func (admin *Admin) Create(ctx context.Context, request Request) (int64, error) {
	nodeID, roots, connected := admin.nodes.Roots(request.Node)
	if !connected {
		return 0, fmt.Errorf("node %q: %w", request.Node, ErrNodeNotConnected)
	}
	for _, root := range request.Roots {
		if !slices.Contains(roots, root) {
			return 0, fmt.Errorf("%s on node %q: %w; it names %v", root, request.Node, ErrNotANodeRoot, roots)
		}
	}
	library := request.NewLibrary
	library.NodeID = nodeID
	id, err := Create(ctx, admin.database, admin.clock, library)
	if err != nil {
		return 0, err
	}
	for _, root := range library.Roots {
		admin.nodes.Enqueue(nodeID, nodeproto.Command{Type: nodeproto.CommandScan, Scan: &nodeproto.Scan{Root: root}})
	}
	return id, nil
}

// Summary is a library with its roots' states and its files counted by filing.
type Summary struct {
	sqlcgen.ListLibrariesRow
	Roots   []sqlcgen.ListLibraryRootStatesRow
	Filings map[string]int64
}

// List is every live library, by name.
func (admin *Admin) List(ctx context.Context) ([]Summary, error) {
	queries := admin.database.Read()
	libraries, err := queries.ListLibraries(ctx)
	if err != nil {
		return nil, fmt.Errorf("list libraries: %w", err)
	}
	summaries := make([]Summary, 0, len(libraries))
	for _, library := range libraries {
		roots, err := queries.ListLibraryRootStates(ctx, library.ID)
		if err != nil {
			return nil, fmt.Errorf("list library %d's roots: %w", library.ID, err)
		}
		counts, err := queries.CountLibraryFilings(ctx, library.ID)
		if err != nil {
			return nil, fmt.Errorf("count library %d's files: %w", library.ID, err)
		}
		filings := map[string]int64{}
		for _, count := range counts {
			filings[count.Filing] = count.Files
		}
		summaries = append(summaries, Summary{ListLibrariesRow: library, Roots: roots, Filings: filings})
	}
	return summaries, nil
}

// Scan asks each root's node to scan it, and reports how many roots were asked. A root whose
// node is not connected is skipped: it scans when the node says hello, where scan_on_node_hello
// says so.
func (admin *Admin) Scan(ctx context.Context, libraryID int64) (int, error) {
	roots, err := admin.libraryRoots(ctx, libraryID)
	if err != nil {
		return 0, err
	}
	asked := 0
	for _, root := range roots {
		if admin.rescan(root) {
			asked++
		}
	}
	return asked, nil
}

// FilesQuery pages through what the scanner saw (D163).
type FilesQuery struct {
	LibraryID *int64
	Filing    *string
	After     int64 // the last id of the page before; 0 for the first
	Limit     int64
}

// Files is one page of live files, by id.
func (admin *Admin) Files(ctx context.Context, query FilesQuery) ([]sqlcgen.ListFilesPageRow, error) {
	rows, err := admin.database.Read().ListFilesPage(ctx, sqlcgen.ListFilesPageParams{
		AfterID: query.After, LibraryID: query.LibraryID, Filing: query.Filing, MaxRows: query.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	return rows, nil
}

// ApproveRemovals is the admin saying a root's withheld removals are real (D208 §3): its next
// complete scan removes what it does not see without waiting again. The root is scanned now if
// its node is connected; it reports whether it was.
func (admin *Admin) ApproveRemovals(ctx context.Context, libraryID int64, node, path string) (bool, error) {
	root, err := admin.root(ctx, libraryID, node, path)
	if err != nil {
		return false, err
	}
	if root.LastScanOutcome == nil || *root.LastScanOutcome != outcomeWithheld {
		return false, fmt.Errorf("%s on %s: %w", path, node, ErrNothingWithheld)
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
	return admin.rescan(root), nil
}

// ForgetMount is the admin saying a disk missing from a mount point inside a root is gone for
// good: its boundary is forgotten and the root's removals approved, so the next complete scan
// removes its files (D208).
func (admin *Admin) ForgetMount(ctx context.Context, libraryID int64, node, mountPath string) (bool, error) {
	nodeID, err := admin.nodeOfLibrary(ctx, libraryID, node)
	if err != nil {
		return false, err
	}
	mount, err := store.One(admin.database.Read().GetRootMount(ctx, sqlcgen.GetRootMountParams{
		NodeID: nodeID, MountPath: mountPath,
	}))
	if errors.Is(err, store.ErrNotFound) || (err == nil && mount.IsPresent != 0) {
		return false, fmt.Errorf("%s on %s: %w", mountPath, node, ErrMountNotMissing)
	}
	if err != nil {
		return false, fmt.Errorf("read the mount %s: %w", mountPath, err)
	}
	root, err := admin.root(ctx, libraryID, node, mount.RootPath)
	if err != nil {
		return false, fmt.Errorf("%s on %s: %w", mountPath, node, ErrMountNotMissing)
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
	return admin.rescan(root), nil
}

// libraryRoots are a live library's roots with their states.
func (admin *Admin) libraryRoots(ctx context.Context, libraryID int64) ([]sqlcgen.ListLibraryRootStatesRow, error) {
	queries := admin.database.Read()
	if _, err := store.One(queries.GetLibrary(ctx, libraryID)); errors.Is(err, store.ErrNotFound) {
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

// root is one of a library's roots, by its node's name and its path.
func (admin *Admin) root(
	ctx context.Context, libraryID int64, node, path string,
) (sqlcgen.ListLibraryRootStatesRow, error) {
	roots, err := admin.libraryRoots(ctx, libraryID)
	if err != nil {
		return sqlcgen.ListLibraryRootStatesRow{}, err
	}
	for _, root := range roots {
		if root.NodeName == node && root.Path == path {
			return root, nil
		}
	}
	return sqlcgen.ListLibraryRootStatesRow{}, fmt.Errorf("%s on %s: %w", path, node, ErrNoSuchRoot)
}

// nodeOfLibrary is the id of the named node, which must hold one of the library's roots.
func (admin *Admin) nodeOfLibrary(ctx context.Context, libraryID int64, node string) (int64, error) {
	roots, err := admin.libraryRoots(ctx, libraryID)
	if err != nil {
		return 0, err
	}
	for _, root := range roots {
		if root.NodeName == node {
			return root.NodeID, nil
		}
	}
	return 0, fmt.Errorf("node %s: %w", node, ErrNoSuchRoot)
}

// rescan asks a root's node to scan it, if the node is connected, and reports whether it did.
func (admin *Admin) rescan(root sqlcgen.ListLibraryRootStatesRow) bool {
	nodeID, _, connected := admin.nodes.Roots(root.NodeName)
	if !connected || nodeID != root.NodeID {
		return false
	}
	admin.nodes.Enqueue(nodeID, nodeproto.Command{Type: nodeproto.CommandScan, Scan: &nodeproto.Scan{Root: root.Path}})
	return true
}
