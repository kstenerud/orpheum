// SPDX-License-Identifier: GPL-3.0-or-later

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
// its id.
func LibraryOn(t *testing.T, database *store.Database, nodeID int64, library Library) int64 {
	t.Helper()
	var libraryID int64
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		var err error
		libraryID, err = queries.InsertLibrary(t.Context(), sqlcgen.InsertLibraryParams{
			Name: library.Name, Kind: library.Kind, MetadataSource: library.MetadataSource,
			ChangeDetection: "manual", CreatedAt: 1,
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

// Reports numbers what one node reports, as the node does: every record carries the next
// ReportSeq. The zero value is a node that has reported nothing.
type Reports struct {
	lastReportSeq int64
}

// Scan is one scan as the node sends it: begin, the records, and complete unless the scan was
// cut off, each numbered after the last record these Reports numbered.
func (reports *Reports) Scan(
	begin nodeproto.ScanBegin, records []nodeproto.Record, isComplete bool,
) []nodeproto.Record {
	all := append([]nodeproto.Record{{Type: nodeproto.RecordScanBegin, ScanBegin: &begin}}, records...)
	if isComplete {
		complete := &nodeproto.ScanComplete{RootPath: begin.RootPath}
		all = append(all, nodeproto.Record{Type: nodeproto.RecordScanComplete, ScanComplete: complete})
	}
	for index := range all {
		reports.lastReportSeq++
		all[index].ReportSeq = reports.lastReportSeq
	}
	return all
}
