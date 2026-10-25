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
	"github.com/kstenerud/orpheum/internal/store"
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
		Register: register, Jobs: record, Nodes: room, Waiting: room, Reads: &readsAsked{},
		Playing:  func() []int64 { return []int64{200} },
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
		Register: register, Jobs: record, Nodes: room, Waiting: room, Reads: &readsAsked{},
		Playing:  func() []int64 { return nil },
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

// seriesOfSix is a database with series 20, whose season 1 is episodes 21 and 22 and season 2 is
// 23 to 26, each with one file, its id ten times the episode's, 1000 ms long, and user 1.
func seriesOfSix(t *testing.T) *store.Database {
	t.Helper()
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
		`INSERT INTO file_probe (file_id, probe_json, duration_ms) VALUES (210, '{}', 1000), (220, '{}', 1000),
		 (230, '{}', 1000), (240, '{}', 1000), (250, '{}', 1000), (260, '{}', 1000)`,
		`INSERT INTO user (id, name, role_id, created_at) VALUES (1, 'owner', 1, 1)`)
	return database
}

// A look at a series' season raises the series and that season's episodes from its person's place
// in it, not the series' next ones; a session that moves to another season releases the episodes
// it left and keeps the series raised (the owner, 2026-10-24).
func TestRaiser_ALookAtASeasonRaisesItsEpisodes(t *testing.T) {
	t.Parallel()
	database := seriesOfSix(t)
	// Season 2's first episode finished: the series' Next Up is 24.
	storetest.ExecRaw(t, database.Path(), `INSERT INTO play_position (user_id, file_id, position_ms, position_read_at,
		 report_seq, updated_at) VALUES (1, 230, 1000, 5, 1, 1)`)
	now := clocktest.NewFake(time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC))
	register := NewRegister(now)
	record := &raising{}
	room := &waitingRoom{}
	raiser := mustRaiser(t, Parts{
		Register: register, Jobs: record, Nodes: room, Waiting: room, Reads: &readsAsked{},
		Playing:  func() []int64 { return nil },
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
		describe(jobs.Interested{ItemIDs: []int64{20, 24}, FileIDs: []int64{240}}, jobs.PriorityLookedAt, false),
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
	if gone := describe(jobs.Interested{ItemIDs: []int64{24}, FileIDs: []int64{240}}, 0,
		false); !slices.Equal(released, []string{gone}) {
		t.Errorf("released %q; want the series' Next Up, and the series still looked at", released)
	}
}

// readsAsked records the titles whose reads were asked for, in order.
type readsAsked struct{ titleIDs []int64 }

func (reads *readsAsked) ReadWhenViewed(_ context.Context, titleID int64) error {
	reads.titleIDs = append(reads.titleIDs, titleID)
	return nil
}

// A look at a series' own page raises the series and its person's Next Up alone, as that episode's
// own page would, and asks for its read: the episode partway through; else the one after the last
// finished; else the first; and none past the last (the owner, 2026-10-25).
func TestRaiser_ALookAtASeriesRaisesItsNextUpAlone(t *testing.T) {
	t.Parallel()
	for _, each := range []struct {
		name      string
		positions string
		want      jobs.Interested
	}{
		{"nothing watched: the first", "", jobs.Interested{ItemIDs: []int64{20, 21}, FileIDs: []int64{210}}},
		{
			"partway: it, though a later one is finished", "(1, 220, 500, 9, 1, 1), (1, 240, 1000, 3, 1, 1)",
			jobs.Interested{ItemIDs: []int64{20, 22}, FileIDs: []int64{220}},
		},
		{
			"finished: the one after", "(1, 210, 1000, 3, 1, 1), (1, 220, 1000, 4, 1, 1)",
			jobs.Interested{ItemIDs: []int64{20, 23}, FileIDs: []int64{230}},
		},
		{"the last finished: the series alone", "(1, 260, 1000, 3, 1, 1)", jobs.Interested{ItemIDs: []int64{20}}},
	} {
		database := seriesOfSix(t)
		if each.positions != "" {
			storetest.ExecRaw(t, database.Path(), `INSERT INTO play_position (user_id, file_id, position_ms,
				 position_read_at, report_seq, updated_at) VALUES `+each.positions)
		}
		now := clocktest.NewFake(time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC))
		register := NewRegister(now)
		record := &raising{}
		room := &waitingRoom{}
		reads := &readsAsked{}
		raiser := mustRaiser(t, Parts{
			Register: register, Jobs: record, Nodes: room, Waiting: room, Reads: reads,
			Playing: func() []int64 { return nil }, Database: database, Settings: settingstest.Seeded(t, database),
			Clock: now, Logger: slog.New(slog.DiscardHandler),
		})
		register.Look("only", 1, 20, NoSeason)
		if err := raiser.Raise(t.Context()); err != nil {
			t.Fatal(err)
		}
		if want := describe(each.want, jobs.PriorityLookedAt, false); !slices.Equal(record.raised, []string{want}) {
			t.Errorf("%s: raised %q; want %q", each.name, record.raised, want)
		}
		if want := each.want.ItemIDs[1:]; !slices.Equal(reads.titleIDs, want) {
			t.Errorf("%s: asked for the reads of %v; want %v, the Next Up's", each.name, reads.titleIDs, want)
		}
	}
}

// A look at a series' own page raises the work of each season's still it shows, as a title's page
// raises its own artwork, and a look at a season's the work of every one of its episodes, whose
// stills it shows; their files only as before (the owner, Fire TV, 2026-10-25: a series' season
// stills showed blank while their sizes waited their turn).
func TestRaiser_ALookAtASeriesRaisesTheWorkOfTheStillsItShows(t *testing.T) {
	t.Parallel()
	database := seriesOfSix(t)
	// Season 1's still is its second episode's, its first having none; season 2's is its first's.
	storetest.ExecRaw(t, database.Path(),
		`INSERT INTO artwork (item_id, kind, source, source_ref, source_sha256) VALUES
		 (22, 'still', 'tmdb', '/b.jpg', X'22'), (23, 'still', 'tmdb', '/c.jpg', X'23'),
		 (24, 'still', 'tmdb', '/d.jpg', X'24')`)
	now := clocktest.NewFake(time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC))
	register := NewRegister(now)
	record := &raising{}
	room := &waitingRoom{}
	raiser := mustRaiser(t, Parts{
		Register: register, Jobs: record, Nodes: room, Waiting: room, Reads: &readsAsked{},
		Playing: func() []int64 { return nil }, Database: database, Settings: settingstest.Seeded(t, database),
		Clock: now, Logger: slog.New(slog.DiscardHandler),
	})
	register.Look("only", 1, 20, NoSeason)
	if err := raiser.Raise(t.Context()); err != nil {
		t.Fatal(err)
	}
	register.Look("only", 1, 20, 2)
	if err := raiser.Raise(t.Context()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		// The series, its Next Up 21 with its file, and the episodes whose stills stand for its seasons.
		describe(jobs.Interested{ItemIDs: []int64{20, 21, 22, 23}, FileIDs: []int64{210}}, jobs.PriorityLookedAt,
			false),
		// The series, season 2's three ahead with their files, and its fourth, whose still it shows.
		describe(jobs.Interested{ItemIDs: []int64{20, 23, 24, 25, 26}, FileIDs: []int64{230, 240, 250}},
			jobs.PriorityLookedAt, false),
	}
	if !slices.Equal(record.raised, want) {
		t.Errorf("raised\n  %q\nwant\n  %q", record.raised, want)
	}
}

// A look at a series follows the ordering its person watches it in (D159; episode-orderings.md
// §7 Q4): its Next Up there, a season its group, and an episode the ordering leaves out never raised.
func TestRaiser_ALookAtASeriesFollowsTheOrderingItsPersonWatchesItIn(t *testing.T) {
	t.Parallel()
	database := seriesOfSix(t)
	// A DVD order: disc 1 is 23 then 21, disc 2 is 26; 22, 24 and 25 are not in it. 23 is finished.
	storetest.ExecRaw(t, database.Path(),
		`INSERT INTO ordering (id, collection_id, name, provider) VALUES (2, 20, 'tmdb:dvd', 'tmdb')`,
		`INSERT INTO position (ordering_id, title_id, group_number, number_in_group, sort_order) VALUES
		 (2, 23, 1, 1, 10001), (2, 21, 1, 2, 10002), (2, 26, 2, 1, 20001)`,
		`INSERT INTO preference (user_id, scope, scope_id, axis, value_json, set_at)
		 VALUES (1, 'work', 20, 'episode_ordering', '"tmdb:dvd"', 1)`,
		`INSERT INTO play_position (user_id, file_id, position_ms, position_read_at, report_seq, updated_at)
		 VALUES (1, 230, 1000, 5, 1, 1)`)
	now := clocktest.NewFake(time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC))
	register := NewRegister(now)
	record := &raising{}
	room := &waitingRoom{}
	raiser := mustRaiser(t, Parts{
		Register: register, Jobs: record, Nodes: room, Waiting: room, Reads: &readsAsked{},
		Playing: func() []int64 { return nil }, Database: database, Settings: settingstest.Seeded(t, database),
		Clock: now, Logger: slog.New(slog.DiscardHandler),
	})
	register.Look("only", 1, 20, NoSeason)
	if err := raiser.Raise(t.Context()); err != nil {
		t.Fatal(err)
	}
	register.Look("only", 1, 20, 2)
	if err := raiser.Raise(t.Context()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		describe(jobs.Interested{ItemIDs: []int64{20, 21}, FileIDs: []int64{210}}, jobs.PriorityLookedAt, false),
		describe(jobs.Interested{ItemIDs: []int64{20, 26}, FileIDs: []int64{260}}, jobs.PriorityLookedAt, false),
	}
	if !slices.Equal(record.raised, want) {
		t.Errorf("raised\n  %q\nwant\n  %q; want the DVD order's Next Up, 21 after 23, then its disc 2", record.raised,
			want)
	}
}

// A look at a series' season asks for the read of every one of its episodes, in their order, once
// for the look: said again it asks for none, and a look begun anew asks again (the owner,
// 2026-10-25: a season's page quick-scans its episodes).
func TestRaiser_ALookAtASeasonAsksForTheReadOfEachOfItsEpisodesOnce(t *testing.T) {
	t.Parallel()
	database := seriesOfSix(t)
	now := clocktest.NewFake(time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC))
	register := NewRegister(now)
	record := &raising{}
	room := &waitingRoom{}
	reads := &readsAsked{}
	raiser := mustRaiser(t, Parts{
		Register: register, Jobs: record, Nodes: room, Waiting: room, Reads: reads,
		Playing: func() []int64 { return nil }, Database: database, Settings: settingstest.Seeded(t, database),
		Clock: now, Logger: slog.New(slog.DiscardHandler),
	})
	register.Look("only", 1, 20, 2)
	if err := raiser.Raise(t.Context()); err != nil {
		t.Fatal(err)
	}
	if want := []int64{23, 24, 25, 26}; !slices.Equal(reads.titleIDs, want) {
		t.Fatalf("asked for the reads of %v; want season 2's episodes, %v", reads.titleIDs, want)
	}
	now.Advance(time.Second)
	register.Look("only", 1, 20, 2)
	if err := raiser.Raise(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(reads.titleIDs) != 4 {
		t.Errorf("asked for the reads of %v; want none more for the look said again", reads.titleIDs)
	}
	register.Leave("only", 20, 2)
	if err := raiser.Raise(t.Context()); err != nil {
		t.Fatal(err)
	}
	now.Advance(time.Second)
	register.Look("only", 1, 20, 2)
	if err := raiser.Raise(t.Context()); err != nil {
		t.Fatal(err)
	}
	if want := []int64{23, 24, 25, 26, 23, 24, 25, 26}; !slices.Equal(reads.titleIDs, want) {
		t.Errorf("asked for the reads of %v; want season 2's again, for the look begun anew", reads.titleIDs)
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
		Register: NewRegister(now), Jobs: record, Nodes: room, Waiting: room, Reads: &readsAsked{},
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
