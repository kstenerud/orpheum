// SPDX-License-Identifier: GPL-3.0-or-later

package librarytest

import (
	"fmt"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Library is the library NodeWithLibrary makes: what it is called, what it holds, where its
// metadata comes from, and its one root on the node.
type Library struct {
	Name           string
	Kind           string
	MetadataSource string
	Root           string
}

// NodeWithLibrary enrols a node named "local" and makes library over one root on it, with
// manual change detection, and returns the node's and the library's ids.
func NodeWithLibrary(t *testing.T, database *store.Database, library Library) (nodeID, libraryID int64) {
	t.Helper()
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		var err error
		nodeID, err = queries.InsertNode(t.Context(), sqlcgen.InsertNodeParams{Name: "local", EnrolledAt: 1})
		if err != nil {
			return fmt.Errorf("node: %w", err)
		}
		libraryID, err = queries.InsertLibrary(t.Context(), sqlcgen.InsertLibraryParams{
			Name: library.Name, Kind: library.Kind, MetadataSource: library.MetadataSource,
			ChangeDetection: "manual", CreatedAt: 1,
		})
		if err != nil {
			return fmt.Errorf("library: %w", err)
		}
		return queries.InsertLibraryRoot(t.Context(), sqlcgen.InsertLibraryRootParams{
			LibraryID: libraryID, NodeID: nodeID, Path: library.Root,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	return nodeID, libraryID
}

// Reports numbers what one node reports, as the node does: every record carries the next
// ReportSeq. The zero value is a node that has reported nothing.
type Reports struct {
	lastSeq int64
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
		reports.lastSeq++
		all[index].ReportSeq = reports.lastSeq
	}
	return all
}
