// SPDX-License-Identifier: GPL-3.0-or-later

package identify_test

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/bounded"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// A stored provider answer is read back under a bound, as it was fetched: one that expands past
// it is refused rather than read whole.
func TestPass_AStoredAnswerThatExpandsPastItsBoundIsRefused(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	lib.provider.Add(heat)
	lib.scan(videoRecord("Metropolis {tmdb-9019}.mkv"))
	lib.matchFileTitle("Metropolis {tmdb-9019}.mkv")
	huge := heat
	huge.Overview = strings.Repeat("x", 8<<20)
	body, err := json.Marshal(huge) //nolint:musttag // as the fake provider stores it
	if err != nil {
		t.Fatal(err)
	}
	compressed, err := bounded.Gzip(body)
	if err != nil {
		t.Fatal(err)
	}
	storetest.ExecRaw(t, lib.database.Path(),
		"UPDATE provider_response SET body_gzip = X'"+hex.EncodeToString(compressed)+"'")
	_, err = lib.pass.IdentifyLibrary(t.Context(), lib.libraryID)
	if !errors.Is(err, bounded.ErrTooLarge) {
		t.Errorf("a pass over a stored answer of %d bytes: %v; want it refused as too large", len(body), err)
	}
	if file := lib.titlesByPath()["Metropolis {tmdb-9019}.mkv"]; titleName(file) != heat.Name {
		t.Errorf("the title is %q, want what the pass before stored left as it was", titleName(file))
	}
}
