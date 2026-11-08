// SPDX-License-Identifier: AGPL-3.0-or-later

package library_test

import (
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// A file is named on its node's byte socket by the root it is under, even a root of "/", whose
// path is its own separator.
func TestBytesFileOf_NamesAFileUnderARootOfSlash(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	nodeID, _ := librarytest.NodeWithLibrary(t, database,
		librarytest.Library{Name: "Films", Kind: "movie", MetadataSource: "nfo", Root: "/"})
	named, err := library.BytesFileOf(t.Context(), database.Read(), library.Located{
		Subject: "file 1", NodeID: nodeID, ReportedPathRaw: []byte("/Film.mkv"),
		PayloadHash: []byte{0x0c},
	})
	if err != nil || named.RootPath != "/" || string(named.Path) != "Film.mkv" || named.PayloadHash != "0c" {
		t.Errorf("a file under / is named %+v, %v; want root / and path Film.mkv", named, err)
	}
}

// A file that cannot be read says since when only when that is known: a node not heard since the
// core started has no time, which read "since 0001-01-01T00:00:00Z" (the production NAS, 2026-10-18).
func TestUnreadableError_SaysSinceWhenOnlyWhenKnown(t *testing.T) {
	t.Parallel()
	away := library.UnreadableError{Unreadable: library.Unreadable{Reason: library.NodeAway, NodeID: 1}}
	if got, want := away.Error(), "node 1's file cannot be read: node_away -"; got != want {
		t.Errorf("a node away since nobody knows when: %q; want %q", got, want)
	}
	away.Since = time.Date(2026, 10, 18, 6, 17, 0, 0, time.UTC)
	if got, want := away.Error(), "node 1's file cannot be read: node_away -, since 2026-10-18T06:17:00Z"; got != want {
		t.Errorf("a node away since a known time: %q; want %q", got, want)
	}
}
