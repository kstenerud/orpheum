// SPDX-License-Identifier: AGPL-3.0-or-later

package preferences

import (
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// D537: a person's own sound offset for a file overrides every person's for them alone, and once
// forgotten every person's holds again; every person's at 0 puts the file back in step.
func TestSoundOffset_APersonsOwnOverridesEveryonesForThemUntilForgotten(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	service := NewService(database, clocktest.NewFake(time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)))
	alice := storetest.InsertUser(t, database, "alice", "admin")
	bob := storetest.InsertUser(t, database, "bob", "user")
	const fileID = 6
	storetest.ExecRaw(t, database.Path(), `INSERT INTO file (id) VALUES (6)`)
	write := func(change func(queries *sqlcgen.Queries) error) {
		t.Helper()
		if err := database.Write(t.Context(), change); err != nil {
			t.Fatal(err)
		}
	}
	// expectOffset checks what a person plays the file with: every person's, their own (nil for
	// none), and so the one it plays with, their own over every person's.
	expectOffset := func(userID, wantEveryones int64, wantOwn *int64, why string) {
		t.Helper()
		prefs, err := service.ForFile(t.Context(), database.Read(), userID, fileID, Tracks{})
		if err != nil {
			t.Fatal(err)
		}
		got := prefs.SoundOffset
		wantMs := wantEveryones
		if wantOwn != nil {
			wantMs = *wantOwn
		}
		isOwnRight := (got.OwnMs == nil) == (wantOwn == nil) && (wantOwn == nil || *got.OwnMs == *wantOwn)
		if got.EveryonesMs != wantEveryones || !isOwnRight || got.Ms() != wantMs {
			t.Errorf("%s: plays with %+v; want every person's %d, own %v, playing %d", why, got, wantEveryones,
				wantOwn, wantMs)
		}
	}
	own := func(ms int64) *int64 { return &ms }
	expectOffset(alice, 0, nil, "a file no one set")
	write(func(queries *sqlcgen.Queries) error {
		return service.KeepEveryonesSoundOffset(t.Context(), queries, fileID, -1200)
	})
	write(func(queries *sqlcgen.Queries) error {
		return service.KeepSoundOffset(t.Context(), queries, bob, fileID, -900)
	})
	expectOffset(alice, -1200, nil, "alice, with none of her own")
	expectOffset(bob, -1200, own(-900), "bob, his own over every person's")
	write(func(queries *sqlcgen.Queries) error {
		return service.ForgetSoundOffset(t.Context(), queries, bob, fileID)
	})
	expectOffset(bob, -1200, nil, "bob, his own forgotten")
	write(func(queries *sqlcgen.Queries) error {
		return service.KeepSoundOffset(t.Context(), queries, bob, fileID, 0)
	})
	expectOffset(bob, -1200, own(0), "bob, his own set to 0, a choice of his own")
	write(func(queries *sqlcgen.Queries) error {
		return service.KeepEveryonesSoundOffset(t.Context(), queries, fileID, 0)
	})
	if left := storetest.QueryRaw(t, database.Path(), `SELECT count(*) FROM file_sound_offset`); !slices.Equal(
		left, []string{"0"}) {
		t.Errorf("every person's set to 0 keeps %v rows; want none: a file with none plays in step", left)
	}
}

// D537, D363 §2: a file confirmed to be a copy of another gives it every person's sound offset where it
// has none, and keeps the other's where it has one; the copy's own goes.
func TestMoveChoicesToCopysFile_MovesEveryonesSoundOffsetWhereTheFileHasNone(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	service := NewService(database, clocktest.NewFake(time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)))
	storetest.ExecRaw(t, database.Path(), `INSERT INTO file (id) VALUES (1), (2), (3), (4)`)
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		for _, set := range [][2]int64{{2, 300}, {3, -500}, {4, 700}} {
			if err := service.KeepEveryonesSoundOffset(t.Context(), queries, set[0], set[1]); err != nil {
				return err
			}
		}
		if err := MoveChoicesToCopysFile(t.Context(), queries, 2, 1); err != nil {
			return err
		}
		return MoveChoicesToCopysFile(t.Context(), queries, 3, 4)
	})
	if err != nil {
		t.Fatal(err)
	}
	got := storetest.QueryRaw(t, database.Path(),
		`SELECT group_concat(file_id || ':' || offset_ms, ' ') FROM file_sound_offset ORDER BY file_id`)
	if want := []string{"1:300 4:700"}; !slices.Equal(got, want) {
		t.Errorf("after the copies' folds, the offsets are %v; want %v", got, want)
	}
}
