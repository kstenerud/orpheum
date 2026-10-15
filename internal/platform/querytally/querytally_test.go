// SPDX-License-Identifier: AGPL-3.0-or-later

package querytally_test

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/querytally"
)

func TestTally_CountsByQueryNameAndSummarises(t *testing.T) {
	t.Parallel()
	if querytally.Of(t.Context()) != nil {
		t.Error("a context with no tally has one")
	}
	ctx, tally := querytally.New(t.Context())
	if querytally.Of(ctx) != tally {
		t.Fatal("the context does not carry its tally")
	}
	tally.Count("-- name: GetLiveGrant :one\nSELECT 1")
	tally.Count("-- name: ListTitles :many\nSELECT 1")
	tally.Count("-- name: ListTitles :many\nSELECT 1")
	tally.Count("SELECT 1")
	tally.CountWrite()
	if got, want := tally.String(), "ListTitles×2 GetLiveGrant×1 unnamed×1"; got != want {
		t.Errorf("summary %q; want %q", got, want)
	}
	if tally.Statements() != 4 || tally.Writes() != 1 {
		t.Errorf("%d statements and %d writes; want 4 and 1", tally.Statements(), tally.Writes())
	}
}
