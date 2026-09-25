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
func NewAdmin(database *store.Database, now clock.Clock, nodes Nodes) *Admin {
	return &Admin{database: database, clock: now, nodes: nodes}
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
	queries := admin.database.Read()
	if _, err := store.One(queries.GetLibrary(ctx, libraryID)); errors.Is(err, store.ErrNotFound) {
		return 0, fmt.Errorf("library %d: %w", libraryID, ErrNoLibrary)
	} else if err != nil {
		return 0, fmt.Errorf("read library %d: %w", libraryID, err)
	}
	roots, err := queries.ListLibraryRootStates(ctx, libraryID)
	if err != nil {
		return 0, fmt.Errorf("list library %d's roots: %w", libraryID, err)
	}
	asked := 0
	for _, root := range roots {
		nodeID, _, connected := admin.nodes.Roots(root.NodeName)
		if !connected || nodeID != root.NodeID {
			continue
		}
		scan := nodeproto.Command{Type: nodeproto.CommandScan, Scan: &nodeproto.Scan{Root: root.Path}}
		admin.nodes.Enqueue(nodeID, scan)
		asked++
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
