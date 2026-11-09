// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// An extra's type is kept in snake case, as the scanner files it, and said in camelCase, as every
// value the API sends is (the owner, 2026-11-09).
func TestItem_SaysAnExtrasTypeInCamelCase(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveDVDOrder(t) // Metropolis (10, file 1)
	librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: 31, Library: 1, Kind: "extra", AddedAt: 3})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: 8, TitleID: 31, CreatedAt: 3})
	storetest.ExecRaw(t, harness.databasePath, `INSERT INTO extra_relation
		(owner_work, extra_work, extra_type, asserted_by)
		VALUES (10, 31, 'behind_the_scenes', 'scanner')`)
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 8, Path: "/m/Metropolis (1995)/Making Of.mkv", PayloadHash: []byte{0x08}, Filing: "listed", VersionID: 8,
		LibraryID: 1, NodeID: 1, SizeBytes: 10,
	})
	got := harness.do(t, http.MethodGet, "/items/10", user, nil)
	var item api.ItemBody
	if err := json.Unmarshal([]byte(got.body), &item); err != nil || got.status != http.StatusOK {
		t.Fatalf("GET /items/10 = %d %s", got.status, got.body)
	}
	if len(item.Extras) != 1 || item.Extras[0].ExtraType != "behindTheScenes" {
		t.Errorf("Metropolis's extras = %+v; want its making-of, of the type behindTheScenes", item.Extras)
	}
}
