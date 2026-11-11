// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
)

// homeRows are Home's rows for a person: Continue Watching's works by id, and the newest films and
// other videos, as GET /items answers each row of Home.
func (harness testHarness) homeRows(t *testing.T, token string) string {
	t.Helper()
	watching := []string{}
	for _, row := range harness.continueWatching(t, token) {
		watching = append(watching, fmt.Sprint(row.ID))
	}
	films, _ := harness.titleNames(t, "/items?kind=film&sort=added&limit=10&home=true", token)
	return fmt.Sprintf("watching %v, films %v", watching, films)
}

// leaveOut keeps the libraries the person leaves out of the place preference names.
func (harness testHarness) leaveOut(t *testing.T, token, preference string, libraryIDs ...int64) {
	t.Helper()
	if libraryIDs == nil {
		libraryIDs = []int64{}
	}
	put := harness.do(t, http.MethodPut, "/me/preferences", token, map[string]any{preference: libraryIDs})
	if put.status != http.StatusNoContent {
		t.Fatalf("PUT /me/preferences %s = %d %s", preference, put.status, put.body)
	}
}

// Each person chooses which libraries fill their Home: a library left out adds nothing to any of
// its rows and stays in Films and Series; an item two libraries reach is on Home while either is
// left in; the choice is apart from Films and Series' (D366 §5).
func TestHome_ALibraryLeftOutOfHomeAddsNothingToItsRows(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	harness.shelveOtherVideos(t)
	harness.setPositionReadAt(t, 1, 60_000, 100)  // Metropolis, of library 1
	harness.setPositionReadAt(t, 40, 60_000, 200) // Beach day, of library 2
	if got, want := harness.homeRows(t, admin), "watching [40 10], films [Metropolis]"; got != want {
		t.Errorf("Home with every library on it = %s; want %s", got, want)
	}

	harness.leaveOut(t, admin, "leftOutOfHomeLibraryIds", 1)
	if mine := harness.do(t, http.MethodGet, "/me/preferences", admin, nil); !strings.Contains(mine.body,
		`"leftOutOfHomeLibraryIds":[1]`) || !strings.Contains(mine.body, `"leftOutLibraryIds":[]`) {
		t.Errorf("GET /me/preferences = %s; want library 1 left out of Home alone", mine.body)
	}
	if got, want := harness.homeRows(t, admin), "watching [40], films []"; got != want {
		t.Errorf("Home with library 1 left out = %s; want %s", got, want)
	}
	names, _ := harness.titleNames(t, "/items?kind=film&sort=name&limit=5", admin)
	if fmt.Sprint(names) != "[Metropolis]" {
		t.Errorf("Films with library 1 left out of Home = %v; want Metropolis, still there", names)
	}

	// A second library over Metropolis's folder reaches Metropolis too, and is left in.
	librarytest.LibraryOn(t, harness.database, 1,
		librarytest.Library{Name: "More films", Kind: "movie", MetadataSource: "nfo", Root: "/m"})
	if got, want := harness.homeRows(t, admin), "watching [40 10], films [Metropolis]"; got != want {
		t.Errorf("Home with Metropolis reached by a library left in = %s; want %s", got, want)
	}
	harness.leaveOut(t, admin, "leftOutOfHomeLibraryIds", 1, 3)
	if got, want := harness.homeRows(t, admin), "watching [40], films []"; got != want {
		t.Errorf("Home with both of Metropolis's libraries left out = %s; want %s", got, want)
	}

	// Left out of Films and Series alone, a library is still on Home.
	harness.leaveOut(t, admin, "leftOutOfHomeLibraryIds")
	harness.leaveOut(t, admin, "leftOutLibraryIds", 1, 3)
	if got, want := harness.homeRows(t, admin), "watching [40 10], films [Metropolis]"; got != want {
		t.Errorf("Home with Metropolis's libraries left out of Films alone = %s; want %s", got, want)
	}
	wantInvalidAt(t, harness.do(t, http.MethodGet, "/items?kind=film&sort=added&limit=5&home=true&library=1",
		admin, nil), http.StatusUnprocessableEntity, "query.home")
}
