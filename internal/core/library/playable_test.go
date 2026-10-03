// SPDX-License-Identifier: GPL-3.0-or-later

package library_test

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// A file is named on its node's byte socket by the root it is under, even a root of "/", whose
// path is its own separator.
func TestBytesFileOf_NamesAFileUnderARootOfSlash(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	nodeID, libraryID := librarytest.NodeWithLibrary(t, database,
		librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "nfo", Root: "/"})
	named, err := library.BytesFileOf(t.Context(), database.Read(), library.Located{
		Subject: "file 1", LibraryID: libraryID, NodeID: nodeID, ReportedPathRaw: []byte("/Film.mkv"),
		PayloadHash: []byte{0x0c},
	})
	if err != nil || named.RootPath != "/" || string(named.Path) != "Film.mkv" || named.PayloadHash != "0c" {
		t.Errorf("a file under / is named %+v, %v; want root / and path Film.mkv", named, err)
	}
}
