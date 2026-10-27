// SPDX-License-Identifier: AGPL-3.0-or-later

package interest

import (
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// A list's tiles in view are a session's look of their own, beside the page it shows: said again as
// the list scrolls, the same look since it began; left, or lapsed, gone (D362 §4).
func TestRegister_ASessionsTilesInViewAreALookBesideItsPage(t *testing.T) {
	t.Parallel()
	now := clocktest.NewFake(time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC))
	register := NewRegister(now)
	register.Look("one", 1, 20, NoSeason)
	register.SeeTiles("one", 1, []int64{21, 22})
	began := now.Now()
	now.Advance(time.Second)
	register.SeeTiles("one", 1, []int64{22, 23})
	looks := register.Looks(time.Minute)
	tilesAt := slices.IndexFunc(looks, func(look Look) bool { return look.Tiles != nil })
	if len(looks) != 2 || tilesAt < 0 {
		t.Fatalf("looks %+v; want the page and its tiles", looks)
	}
	if tiles := looks[tilesAt]; !slices.Equal(tiles.Tiles, []int64{22, 23}) || !tiles.Since.Equal(began) {
		t.Errorf("the tiles' look %+v; want 22 and 23, scrolled to, since the list was first shown", tiles)
	}
	register.LeaveTiles("one")
	if looks = register.Looks(time.Minute); len(looks) != 1 || looks[0].ItemID != 20 {
		t.Errorf("looks %+v after the list was left; want the page alone", looks)
	}
	register.SeeTiles("two", 2, []int64{30})
	now.Advance(2 * time.Minute)
	if looks = register.Looks(time.Minute); len(looks) != 0 {
		t.Errorf("looks %+v after a minute unsaid; want none", looks)
	}
}

// The tiles in view raise the work of the items they show, as a page looked at raises its own, and
// ask for no file's read; tiles scrolled away from are released (D362 §4, D324).
func TestRaiser_TilesInViewRaiseTheirItemsWork(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	now := clocktest.NewFake(time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC))
	register := NewRegister(now)
	record := &raising{}
	room := &waitingRoom{}
	reads := &readsAsked{}
	raiser := mustRaiser(t, Parts{
		Register: register, Jobs: record, Nodes: room, Waiting: room, Reads: reads,
		Playing:  func() []int64 { return nil },
		Database: database, Settings: settingstest.Seeded(t, database), Clock: now,
		Logger: slog.New(slog.DiscardHandler),
	})
	register.SeeTiles("one", 1, []int64{31, 32})
	if err := raiser.Raise(t.Context()); err != nil {
		t.Fatal(err)
	}
	want := describe(jobs.Interested{ItemIDs: []int64{31, 32}}, jobs.PriorityLookedAt, false)
	if !slices.Equal(record.raised, []string{want}) || len(reads.titleIDs) != 0 {
		t.Errorf("raised %q, asked to read %v; want the tiles' items at a look's priority, no read", record.raised,
			reads.titleIDs)
	}
	register.SeeTiles("one", 1, []int64{32, 33})
	if err := raiser.Raise(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(record.released) != 1 || !slices.Equal(record.released[0].ItemIDs, []int64{31}) {
		t.Errorf("released %+v; want item 31, scrolled away from", record.released)
	}
}
