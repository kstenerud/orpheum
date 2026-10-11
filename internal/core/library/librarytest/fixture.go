// SPDX-License-Identifier: AGPL-3.0-or-later

package librarytest

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Library is a library a test makes: what it is called, what it holds, where its metadata comes
// from, and its one root on its node, or none when Root is empty.
type Library struct {
	Name           string
	Kind           string
	MetadataSource string
	Root           string
}

// Node enrols a node by name and returns its id.
func Node(t *testing.T, database *store.Database, name string) int64 {
	t.Helper()
	var nodeID int64
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		var err error
		nodeID, err = queries.InsertNode(t.Context(), sqlcgen.InsertNodeParams{Name: name, EnrolledAt: 1})
		return err
	})
	if err != nil {
		t.Fatalf("node %s: %v", name, err)
	}
	return nodeID
}

// LibraryOn makes library, with manual change detection, over its root on the node, and returns
// its id. The built-in user role is given it, as step 0033 gave that role every library a
// household had (D243 §4); a test of what a role does not reach takes it back.
func LibraryOn(t *testing.T, database *store.Database, nodeID int64, library Library) int64 {
	t.Helper()
	var libraryID int64
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		var err error
		libraryID, err = queries.InsertLibrary(t.Context(), sqlcgen.InsertLibraryParams{
			Name: library.Name, Kind: library.Kind, MetadataSource: library.MetadataSource,
			ChangeDetection: "manual", CreatedAt: 1,
		})
		if err != nil {
			return err
		}
		userRole, err := queries.GetRoleByName(t.Context(), "user")
		if err != nil {
			return err
		}
		err = queries.InsertRoleLibrary(t.Context(), sqlcgen.InsertRoleLibraryParams{
			RoleID: userRole.ID, LibraryID: libraryID,
		})
		if err != nil || library.Root == "" {
			return err
		}
		return queries.InsertLibraryRoot(t.Context(), sqlcgen.InsertLibraryRootParams{
			LibraryID: libraryID, NodeID: nodeID, Path: library.Root,
		})
	})
	if err != nil {
		t.Fatalf("library %s: %v", library.Name, err)
	}
	return libraryID
}

// NodeWithLibrary enrols a node named "local" and makes library over one root on it, and returns
// the node's and the library's ids.
func NodeWithLibrary(t *testing.T, database *store.Database, library Library) (nodeID, libraryID int64) {
	t.Helper()
	nodeID = Node(t, database, "local")
	return nodeID, LibraryOn(t, database, nodeID, library)
}

// Scan is one scan's records as the ingest takes them: begin, the records, and complete unless the
// scan was cut off.
func Scan(begin nodeproto.ScanBegin, records []nodeproto.ScanRecord, isComplete bool) []nodeproto.ScanRecord {
	all := append([]nodeproto.ScanRecord{{Type: nodeproto.RecordScanBegin, ScanBegin: &begin}}, records...)
	if isComplete {
		complete := &nodeproto.ScanComplete{RootPath: begin.RootPath}
		all = append(all, nodeproto.ScanRecord{Type: nodeproto.RecordScanComplete, ScanComplete: complete})
	}
	return all
}
