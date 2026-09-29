// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"testing"

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
	service := &Service{database: database}
	named, err := service.bytesFileOf(t.Context(), located{
		subject: "file 1", libraryID: libraryID, nodeID: nodeID, reportedPathBytes: []byte("/Film.mkv"),
		payloadHash: []byte{0x0c},
	})
	if err != nil || named.RootPath != "/" || string(named.Path) != "Film.mkv" || named.PayloadHash != "0c" {
		t.Errorf("a file under / is named %+v, %v; want root / and path Film.mkv", named, err)
	}
}
