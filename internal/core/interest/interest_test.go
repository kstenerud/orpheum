// SPDX-License-Identifier: AGPL-3.0-or-later

package interest

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// A session looks at one page at a time; a look lapses when not said again, and the latest begun
// is first (D324 §1, §2).
func TestRegister_KeepsEachSessionsPageTheLatestFirstUntilItLapses(t *testing.T) {
	t.Parallel()
	now := clocktest.NewFake(time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC))
	register := NewRegister(now)
	register.Look("first", 1, 10, NoSeason)
	now.Advance(time.Second)
	register.Look("second", 1, 20, NoSeason)
	now.Advance(time.Second)
	register.Look("first", 1, 30, NoSeason) // moved on from 10
	looks := register.Looks(time.Minute)
	if len(looks) != 2 || looks[0].ItemID != 30 || looks[1].ItemID != 20 {
		t.Fatalf("looks %+v; want 30, begun last, then 20", looks)
	}
	register.Leave("second", 99, NoSeason) // a page it is not on
	now.Advance(30 * time.Second)
	register.Look("first", 1, 30, NoSeason)
	now.Advance(40 * time.Second)
	if looks = register.Looks(time.Minute); len(looks) != 1 || looks[0].ItemID != 30 {
		t.Errorf("looks %+v; want 30 alone, said lately, and 20 lapsed", looks)
	}
	register.Leave("first", 30, NoSeason)
	if looks = register.Looks(time.Minute); len(looks) != 0 {
		t.Errorf("looks %+v after leaving; want none", looks)
	}
}

// A series' season's page is a look of its own: a session that moves to another season begins a
// new look, and leaving a season it has moved on from leaves the one it is on (the owner,
// 2026-10-24).
func TestRegister_ASeasonsPageIsALookOfItsOwn(t *testing.T) {
	t.Parallel()
	now := clocktest.NewFake(time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC))
	register := NewRegister(now)
	register.Look("only", 1, 20, NoSeason)
	now.Advance(time.Second)
	register.Look("only", 1, 20, 1)
	looks := register.Looks(time.Minute)
	if len(looks) != 1 || looks[0].ItemID != 20 || looks[0].Season != 1 || !looks[0].Since.Equal(now.Now()) {
		t.Fatalf("looks %+v; want series 20's season 1, begun now", looks)
	}
	now.Advance(time.Second)
	register.Look("only", 1, 20, 2)
	register.Leave("only", 20, 1) // the season moved on from, said after
	if looks = register.Looks(time.Minute); len(looks) != 1 || looks[0].Season != 2 {
		t.Fatalf("looks %+v; want season 2, still shown", looks)
	}
	register.Leave("only", 20, NoSeason)
	if looks = register.Looks(time.Minute); len(looks) != 1 {
		t.Errorf("looks %+v after leaving the series' own page; want season 2 still", looks)
	}
	register.Leave("only", 20, 2)
	if looks = register.Looks(time.Minute); len(looks) != 0 {
		t.Errorf("looks %+v after leaving season 2; want none", looks)
	}
}

// raising records each raise and release.
type raising struct {
	raised   []string
	released []jobs.Interested
	// answers are what each raise at a priority answers: the jobs it raised.
	answers map[int64][]jobs.Raised
}

func (record *raising) Raise(
	_ context.Context, interested jobs.Interested, priority int64, until time.Time,
) ([]jobs.Raised, error) {
	if len(interested.ItemIDs)+len(interested.FileIDs) > 0 {
		record.raised = append(record.raised, describe(interested, priority, until.IsZero()))
	}
	return record.answers[priority], nil
}

// mustRaiser is the raiser of parts, which name every one.
func mustRaiser(t *testing.T, parts Parts) *Raiser {
	t.Helper()
	raiser, err := NewRaiser(parts)
	if err != nil {
		t.Fatal(err)
	}
	return raiser
}

// A raiser built without one of its parts is refused, rather than failing at the raise that
// reaches it.
func TestNewRaiser_RefusesPartsThatLeaveOneOut(t *testing.T) {
	t.Parallel()
	if _, err := NewRaiser(Parts{Register: NewRegister(clocktest.NewFake(time.Time{}))}); !errors.Is(err,
		ErrMissingPart) {
		t.Errorf("NewRaiser of a register alone = %v; want ErrMissingPart", err)
	}
}

// waitingRoom records the destinations brought back, and the nodes told a new priority.
type waitingRoom struct {
	unparked      []jobs.Destination
	reprioritized []int64
}

func (room *waitingRoom) Unpark(_ context.Context, destination jobs.Destination) {
	room.unparked = append(room.unparked, destination)
}

func (room *waitingRoom) Reprioritize(jobID, _ int64) {
	room.reprioritized = append(room.reprioritized, jobID)
}

func (record *raising) Release(_ context.Context, interested jobs.Interested) error {
	record.released = append(record.released, interested)
	return nil
}

// describe is a raise in words a test compares.
func describe(interested jobs.Interested, priority int64, isForGood bool) string {
	return slog.Group("", "items", interested.ItemIDs, "files", interested.FileIDs, "priority", priority,
		"forGood", isForGood).String()
}

// What is played is raised at a play's priority, what is looked at below it the latest highest, a
// film with its files; a look that ended is released, and what anyone watched is raised for good
// to the shown-interest rung (D324 §2, §3).
func TestRaiser_RaisesThePlayedTheLookedAtAndTheWatched(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	librarytest.NodeWithLibrary(t, database, librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "tmdb"})
	storetest.ExecRaw(t, database.Path(),
		`INSERT INTO item (id, kind, library_id, added_at) VALUES (10, 'title', 1, 1), (20, 'title', 1, 1)`,
		`INSERT INTO title (id, kind, identification_state) VALUES (10, 'movie', 'identified'),
		 (20, 'movie', 'identified')`,
		`INSERT INTO version (id, title_id, created_at) VALUES (1, 10, 1), (2, 20, 1)`,
		`INSERT INTO file (id, library_id, node_id, path, size_bytes, payload_hash, filing, version_id, first_seen_at,
		 last_seen_at) VALUES (100, 1, 1, '/m/a.mkv', 1, X'01', 'listed', 1, 1, 1),
		 (200, 1, 1, '/m/b.mkv', 1, X'02', 'listed', 2, 1, 1)`,
		`INSERT INTO user (id, name, role_id, created_at) VALUES (1, 'owner', 1, 1)`,
		`INSERT INTO play_position (user_id, file_id, position_ms, report_seq, updated_at) VALUES (1, 200, 1000, 1, 1)`)
	now := clocktest.NewFake(time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC))
	register := NewRegister(now)
	record := &raising{}
	room := &waitingRoom{}
	raiser := mustRaiser(t, Parts{
		Register: register, Jobs: record, Nodes: room, Waiting: room, Playing: func() []int64 { return []int64{200} },
		Database: database, Settings: settingstest.Seeded(t, database), Clock: now,
		Logger: slog.New(slog.DiscardHandler),
	})
	register.Look("first", 1, 20, NoSeason)
	now.Advance(time.Second)
	register.Look("second", 1, 10, NoSeason)
	if err := raiser.Raise(t.Context()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		describe(jobs.Interested{FileIDs: []int64{200}}, jobs.PriorityPlayNow, false),
		describe(jobs.Interested{ItemIDs: []int64{10}, FileIDs: []int64{100}}, jobs.PriorityLookedAt, false),
		describe(jobs.Interested{ItemIDs: []int64{20}, FileIDs: []int64{200}}, jobs.PriorityLookedAt-1, false),
	}
	if !slices.Equal(record.raised, want) {
		t.Errorf("raised\n  %q\nwant\n  %q", record.raised, want)
	}
	register.Leave("first", 20, NoSeason)
	if err := raiser.Raise(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(record.released) != 1 || !slices.Equal(record.released[0].ItemIDs, []int64{20}) {
		t.Errorf("released %+v; want film 20, left", record.released)
	}
	record.raised = nil
	if err := raiser.RaiseWatched(t.Context()); err != nil {
		t.Fatal(err)
	}
	if watched := describe(jobs.Interested{ItemIDs: []int64{20}, FileIDs: []int64{200}}, jobs.PriorityShownInterest,
		true); !slices.Equal(record.raised, []string{watched}) {
		t.Errorf("raised %q; want the watched film for good, at the shown-interest rung", record.raised)
	}
}

// A look that goes on gives back the work that left it, as a series' episode its person moved past
// does: here a film's file, removed since the last pass (the owner's staging, 2026-10-21, where an
// episode out of the window kept the looked-at priority until its interest lapsed).
func TestRaiser_ALookThatGoesOnReleasesWhatLeftIt(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	librarytest.NodeWithLibrary(t, database, librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "tmdb"})
	storetest.ExecRaw(t, database.Path(),
		`INSERT INTO item (id, kind, library_id, added_at) VALUES (10, 'title', 1, 1)`,
		`INSERT INTO title (id, kind, identification_state) VALUES (10, 'movie', 'identified')`,
		`INSERT INTO version (id, title_id, created_at) VALUES (1, 10, 1)`,
		`INSERT INTO file (id, library_id, node_id, path, size_bytes, payload_hash, filing, version_id, first_seen_at,
		 last_seen_at) VALUES (100, 1, 1, '/m/a.mkv', 1, X'01', 'listed', 1, 1, 1),
		 (101, 1, 1, '/m/a2.mkv', 1, X'02', 'listed', 1, 1, 1)`)
	now := clocktest.NewFake(time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC))
	register := NewRegister(now)
	record := &raising{}
	room := &waitingRoom{}
	raiser := mustRaiser(t, Parts{
		Register: register, Jobs: record, Nodes: room, Waiting: room, Playing: func() []int64 { return nil },
		Database: database, Settings: settingstest.Seeded(t, database), Clock: now,
		Logger: slog.New(slog.DiscardHandler),
	})
	register.Look("only", 1, 10, NoSeason)
	if err := raiser.Raise(t.Context()); err != nil {
		t.Fatal(err)
	}
	storetest.ExecRaw(t, database.Path(), `UPDATE file SET removed_at = 2 WHERE id = 101`)
	if err := raiser.Raise(t.Context()); err != nil {
		t.Fatal(err)
	}
	var released []string
	for _, interested := range record.released {
		released = append(released, describe(interested, 0, false))
	}
	if want := describe(jobs.Interested{FileIDs: []int64{101}}, 0, false); !slices.Equal(released, []string{want}) {
		t.Errorf("released %q; want the removed file alone, the film still looked at", released)
	}
}

// A look at a series' season raises the series and that season's episodes from its person's place
// in it, not the series' next ones; a session that moves to another season releases the episodes
// it left and keeps the series raised (the owner, 2026-10-24).
func TestRaiser_ALookAtASeasonRaisesItsEpisodes(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	librarytest.NodeWithLibrary(t, database, librarytest.Library{Name: "Shows", Kind: "shows", MetadataSource: "tmdb"})
	storetest.ExecRaw(t, database.Path(),
		`INSERT INTO item (id, kind, library_id, added_at) VALUES (20, 'collection', 1, 1), (21, 'title', 1, 1),
		 (22, 'title', 1, 1), (23, 'title', 1, 1), (24, 'title', 1, 1), (25, 'title', 1, 1), (26, 'title', 1, 1)`,
		`INSERT INTO title (id, kind, identification_state) VALUES (21, 'episode', 'identified'),
		 (22, 'episode', 'identified'), (23, 'episode', 'identified'), (24, 'episode', 'identified'),
		 (25, 'episode', 'identified'), (26, 'episode', 'identified')`,
		`INSERT INTO collection (id, kind, membership, created_by, identification_state)
		 VALUES (20, 'series', 'explicit', 'scanner', 'identified')`,
		`INSERT INTO ordering (id, collection_id, name) VALUES (1, 20, 'aired')`,
		`INSERT INTO position (ordering_id, title_id, group_number, number_in_group, sort_order) VALUES
		 (1, 21, 1, 1, 10001), (1, 22, 1, 2, 10002), (1, 23, 2, 1, 20001), (1, 24, 2, 2, 20002),
		 (1, 25, 2, 3, 20003), (1, 26, 2, 4, 20004)`,
		`INSERT INTO version (id, title_id, created_at) VALUES (1, 21, 1), (2, 22, 1), (3, 23, 1), (4, 24, 1),
		 (5, 25, 1), (6, 26, 1)`,
		`INSERT INTO file (id, library_id, node_id, path, size_bytes, payload_hash, filing, version_id, first_seen_at,
		 last_seen_at) VALUES (210, 1, 1, '/s/1.mkv', 1, X'01', 'listed', 1, 1, 1),
		 (220, 1, 1, '/s/2.mkv', 1, X'02', 'listed', 2, 1, 1), (230, 1, 1, '/s/3.mkv', 1, X'03', 'listed', 3, 1, 1),
		 (240, 1, 1, '/s/4.mkv', 1, X'04', 'listed', 4, 1, 1), (250, 1, 1, '/s/5.mkv', 1, X'05', 'listed', 5, 1, 1),
		 (260, 1, 1, '/s/6.mkv', 1, X'06', 'listed', 6, 1, 1)`,
		`INSERT INTO file_probe (file_id, probe_json, duration_ms) VALUES (230, '{}', 1000)`,
		`INSERT INTO user (id, name, role_id, created_at) VALUES (1, 'owner', 1, 1)`,
		// Season 2's first episode finished: the series' next are 24 to 26.
		`INSERT INTO play_position (user_id, file_id, position_ms, position_read_at, report_seq, updated_at)
		 VALUES (1, 230, 1000, 5, 1, 1)`)
	now := clocktest.NewFake(time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC))
	register := NewRegister(now)
	record := &raising{}
	room := &waitingRoom{}
	raiser := mustRaiser(t, Parts{
		Register: register, Jobs: record, Nodes: room, Waiting: room, Playing: func() []int64 { return nil },
		Database: database, Settings: settingstest.Seeded(t, database), Clock: now,
		Logger: slog.New(slog.DiscardHandler),
	})
	register.Look("only", 1, 20, NoSeason)
	if err := raiser.Raise(t.Context()); err != nil {
		t.Fatal(err)
	}
	register.Look("only", 1, 20, 1)
	if err := raiser.Raise(t.Context()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		describe(jobs.Interested{ItemIDs: []int64{20, 24, 25, 26}, FileIDs: []int64{240, 250, 260}},
			jobs.PriorityLookedAt, false),
		describe(jobs.Interested{ItemIDs: []int64{20, 21, 22}, FileIDs: []int64{210, 220}}, jobs.PriorityLookedAt,
			false),
	}
	if !slices.Equal(record.raised, want) {
		t.Errorf("raised\n  %q\nwant\n  %q", record.raised, want)
	}
	var released []string
	for _, interested := range record.released {
		released = append(released, describe(interested, 0, false))
	}
	if gone := describe(jobs.Interested{ItemIDs: []int64{24, 25, 26}, FileIDs: []int64{240, 250, 260}}, 0,
		false); !slices.Equal(released, []string{gone}) {
		t.Errorf("released %q; want the series' next episodes, and the series still looked at", released)
	}
}

// Work of a person's tier that waits for room on a node (a slot, a disk's readers, or its last reader
// kept for faster work) is brought back, once for each room, at each pass, so its claim bumps the
// lowest work holding it; work given to a node is told the priority a raise gave it; work waiting on
// anything else, or raised only to the shown-interest rung, waits on (D324 §4, D255 §4).
func TestRaiser_BringsBackRaisedWorkWaitingForRoomOnANode(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	now := clocktest.NewFake(time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC))
	reader, slot := jobs.ReaderDestination(1, "disk:sda"), jobs.SlotDestination(1, "read_through")
	lastReader := jobs.LastReaderDestination(1, "disk:sda")
	record := &raising{answers: map[int64][]jobs.Raised{
		jobs.PriorityPlayNow: {
			{ID: 1, Priority: jobs.PriorityPlayNow, Destination: reader},
			{ID: 2, Priority: jobs.PriorityPlayNow, Destination: reader},
			{ID: 3, Priority: jobs.PriorityPlayNow, Destination: slot},
			{ID: 4, Priority: jobs.PriorityPlayNow, Destination: jobs.DestinationTMDBAPI},
			{ID: 5, Priority: jobs.PriorityPlayNow, IsAssigned: true, Destination: slot, HasRisen: true},
			{ID: 7, Priority: jobs.PriorityPlayNow, IsAssigned: true}, // its node told before
			{ID: 8, Priority: jobs.PriorityPlayNow, Destination: lastReader},
		},
		jobs.PriorityShownInterest: {{ID: 6, Priority: jobs.PriorityShownInterest, Destination: reader}},
	}}
	room := &waitingRoom{}
	raiser := mustRaiser(t, Parts{
		Register: NewRegister(now), Jobs: record, Nodes: room, Waiting: room,
		Playing: func() []int64 { return []int64{100} }, Database: database,
		Settings: settingstest.Seeded(t, database), Clock: now, Logger: slog.New(slog.DiscardHandler),
	})
	if err := raiser.Raise(t.Context()); err != nil {
		t.Fatal(err)
	}
	if want := []jobs.Destination{reader, slot, lastReader}; !slices.Equal(room.unparked, want) {
		t.Errorf("brought back %q; want %q, each once", room.unparked, want)
	}
	if !slices.Equal(room.reprioritized, []int64{5}) {
		t.Errorf("told the nodes of %v; want job 5, the one given to a node", room.reprioritized)
	}
	// Background work parks the room again as often as it is brought back: each pass brings it back.
	room.unparked = nil
	if err := raiser.Raise(t.Context()); err != nil {
		t.Fatal(err)
	}
	if want := []jobs.Destination{reader, slot, lastReader}; !slices.Equal(room.unparked, want) {
		t.Errorf("the next pass brought back %q; want %q again", room.unparked, want)
	}
	room.unparked = nil
	if err := raiser.RaiseWatched(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(room.unparked) != 0 {
		t.Errorf("a raise to the shown-interest rung brought back %q; want nothing", room.unparked)
	}
}
