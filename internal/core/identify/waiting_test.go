// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/querytally"
)

// What waits for a person's match is counted by one statement, whatever the library holds, never
// by fetching its titles (D303, DL6; the access audit's A7).
func TestWaitingForAPerson_IsOneCountingStatement(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	lib.scan(videoRecord("Metropolis (1995)/Metropolis (1995).mkv"), videoRecord("Wing (1979)/Wing (1979).mkv"))
	ctx, tally := querytally.New(t.Context())
	if _, err := lib.chooser.WaitingForAPerson(ctx); err != nil {
		t.Fatal(err)
	}
	if got := tally.String(); got != "CountWaitingForAPerson×1" {
		t.Errorf("counting what waits ran %q; want the one counting statement", got)
	}
}
