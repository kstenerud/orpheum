// SPDX-License-Identifier: AGPL-3.0-or-later

package problemreport_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/problemreport"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// keeperStart is when the keeper's tests make their reports.
var keeperStart = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

// fakeNodes answers for the nodes: each node's lines, its error, and how it was last heard.
type fakeNodes struct {
	mutex       sync.Mutex
	lines       map[int64][]slogx.Line
	failures    map[int64]error
	connections map[int64]nodes.Connection
	asked       []int64
}

func (fake *fakeNodes) RecentLog(_ context.Context, nodeID int64) ([]slogx.Line, error) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.asked = append(fake.asked, nodeID)
	if err := fake.failures[nodeID]; err != nil {
		return nil, err
	}
	return fake.lines[nodeID], nil
}

func (fake *fakeNodes) NodeConnection(_ context.Context, nodeID int64) (nodes.Connection, time.Time) {
	connection, found := fake.connections[nodeID]
	if !found {
		return nodes.AwaitingHello, time.Time{}
	}
	return connection, keeperStart.Add(-4 * time.Hour)
}

// keeperFixture is a keeper over a migrated database, the core's recent log and fake nodes.
type keeperFixture struct {
	keeper   *problemreport.Keeper
	database *store.Database
	clock    *clocktest.Fake
	nodes    *fakeNodes
	userID   int64
}

func newKeeperFixture(t *testing.T) *keeperFixture {
	t.Helper()
	database := storetest.OpenMigrated(t)
	clock := clocktest.NewFake(keeperStart)
	recent := slogx.NewRecent(slogx.ProcessRecentLimits())
	coreLogger, err := slogx.NewKeeping(&strings.Builder{}, slogx.FormatJSON, slog.LevelInfo, recent)
	if err != nil {
		t.Fatal(err)
	}
	record := slog.NewRecord(keeperStart.Add(-time.Minute), slog.LevelDebug, "the core's detail", 0)
	if err = coreLogger.Handler().Handle(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	fake := &fakeNodes{
		lines: map[int64][]slogx.Line{}, failures: map[int64]error{}, connections: map[int64]nodes.Connection{},
	}
	keeper := problemreport.NewKeeper(problemreport.Parts{
		Database: database, Clock: clock, Version: "v0.9.0", Recent: recent, NodeLogs: fake, Connections: fake,
		House: func() string { return "hillhouse" }, Logger: slog.New(slog.DiscardHandler),
	})
	return &keeperFixture{
		keeper: keeper, database: database, clock: clock, nodes: fake,
		userID: storetest.InsertUser(t, database, "danielle", "user"),
	}
}

// addNode enrols a node named name.
func (fixture *keeperFixture) addNode(t *testing.T, name string) int64 {
	t.Helper()
	var nodeID int64
	err := fixture.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		var inserting error
		nodeID, inserting = queries.InsertNode(t.Context(), sqlcgen.InsertNodeParams{
			Name: name, EnrolledAt: keeperStart.UnixMilli(),
		})
		return inserting
	})
	if err != nil {
		t.Fatal(err)
	}
	return nodeID
}

// report makes a report of words.
func (fixture *keeperFixture) report(t *testing.T, words string) int64 {
	t.Helper()
	reportID, err := fixture.keeper.Make(t.Context(), problemreport.Request{
		UserID: fixture.userID, DeviceName: "lounge telly", Words: words, Page: "/watch/12",
		Device: []problemreport.Fact{{Name: "Browser", Value: "Firefox 158"}},
		Playback: &problemreport.PlaybackRequest{
			PlaybackID: 3, ItemID: 12, Position: 90 * time.Second, Quality: "1080p", State: "stalled",
		},
	})
	if err != nil {
		t.Fatalf("Make: %v", err)
	}
	return reportID
}

// download is the report in form.
func (fixture *keeperFixture) download(t *testing.T, reportID int64, form problemreport.Form) string {
	t.Helper()
	rendered, err := fixture.keeper.Download(t.Context(), reportID, problemreport.Options{Form: form})
	if err != nil {
		t.Fatalf("Download %s: %v", form, err)
	}
	return rendered.Whole
}

// A report holds the core's recent log, a debug line among it, each connected node's, and names
// every node that gave none absent, with why and when it was last heard (D524 §3).
func TestMake_KeepsTheCoresAndEachNodesLogAndNamesANodeThatGaveNoneAbsent(t *testing.T) {
	t.Parallel()
	fixture := newKeeperFixture(t)
	answering := fixture.addNode(t, "garage")
	failing := fixture.addNode(t, "attic")
	fixture.addNode(t, "cellar") // never heard since the core started
	fixture.nodes.connections[answering] = nodes.Connected
	fixture.nodes.connections[failing] = nodes.Connected
	fixture.nodes.lines[answering] = []slogx.Line{{
		Time: keeperStart.Add(-time.Second), Level: slog.LevelDebug, Message: "the node's detail",
		Fields: []slogx.Field{{Key: "fileId", Value: slog.Int64Value(77)}},
	}}
	fixture.nodes.failures[failing] = errors.New("context deadline exceeded")

	full := fixture.download(t, fixture.report(t, "it stalled"), problemreport.FormFull)
	for _, want := range []string{
		"the core's detail", "the node's detail", "77", "context deadline exceeded", "last heard 4h0m0s ago",
		"not heard since the core started", "it stalled", "stalled", "1080p", "Firefox 158",
	} {
		if !strings.Contains(full, want) {
			t.Errorf("the full report does not say %q:\n%s", want, full)
		}
	}
	if len(fixture.nodes.asked) != 2 {
		t.Errorf("asked nodes %v for their logs; want the two connected ones alone", fixture.nodes.asked)
	}
}

// The shareable download hides every name the database holds that says whose the report is, found
// in the person's own words too: a person's, a device's, a node's, a library's, the house's
// (D524 §4).
func TestDownload_TheShareableFormHidesTheNamesTheDatabaseHolds(t *testing.T) {
	t.Parallel()
	fixture := newKeeperFixture(t)
	fixture.addNode(t, "garagebox")
	storetest.ExecRaw(t, fixture.database.Path(),
		`INSERT INTO library (name, kind, metadata_source, change_detection, check_every_hours, is_writable, created_at)
		 VALUES ('grandmas shelf', 'movie', 'tmdb', 'manual', 6, 0, 1)`)
	words := "danielle on the lounge telly: grandmas shelf from garagebox stalls at hillhouse"
	reportID := fixture.report(t, words)

	shareable := fixture.download(t, reportID, problemreport.FormShareable)
	for _, name := range []string{"danielle", "lounge telly", "grandmas shelf", "garagebox", "hillhouse"} {
		if strings.Contains(shareable, name) {
			t.Errorf("the shareable report names %q:\n%s", name, shareable)
		}
	}
	if full := fixture.download(t, reportID, problemreport.FormFull); !strings.Contains(full, words) {
		t.Errorf("the full report does not hold the words as said:\n%s", full)
	}
}

// One person makes no more than PerPersonPerHour reports in an hour, and may again an hour on.
func TestMake_RefusesAPersonPastTheirReportsInAnHour(t *testing.T) {
	t.Parallel()
	fixture := newKeeperFixture(t)
	for range problemreport.PerPersonPerHour {
		fixture.report(t, "again")
	}
	request := problemreport.Request{UserID: fixture.userID, Words: "once more"}
	if _, err := fixture.keeper.Make(t.Context(), request); !errors.Is(err, problemreport.ErrTooMany) {
		t.Errorf("one report past the hour's = %v; want ErrTooMany", err)
	}
	fixture.clock.Advance(time.Hour)
	if _, err := fixture.keeper.Make(t.Context(), request); err != nil {
		t.Errorf("a report an hour on = %v; want it made", err)
	}
}

// A report is listed, newest first, until it is 60 days old; then it is listed no longer, cannot
// be downloaded, and Expire deletes it (D524 §4).
func TestList_AReportGoesAfterSixtyDays(t *testing.T) {
	t.Parallel()
	fixture := newKeeperFixture(t)
	old := fixture.report(t, "the old one")
	fixture.clock.Advance(24 * time.Hour)
	newer := fixture.report(t, "the newer one")
	listed, err := fixture.keeper.List(t.Context())
	if err != nil || len(listed) != 2 || listed[0].ID != newer || listed[0].UserName != "danielle" ||
		listed[0].Words != "the newer one" || listed[0].Version != "v0.9.0" {
		t.Fatalf("List = %+v, %v; want the newer report first, then the old", listed, err)
	}

	fixture.clock.Advance(problemreport.KeptFor - 24*time.Hour + time.Millisecond)
	listed, err = fixture.keeper.List(t.Context())
	if err != nil || len(listed) != 1 || listed[0].ID != newer {
		t.Errorf("List 60 days on = %+v, %v; want the newer report alone", listed, err)
	}
	_, err = fixture.keeper.Download(t.Context(), old, problemreport.Options{Form: problemreport.FormFull})
	if !errors.Is(err, problemreport.ErrNoReport) {
		t.Errorf("Download of the old report = %v; want ErrNoReport", err)
	}
	if err = fixture.keeper.Expire(t.Context()); err != nil {
		t.Fatal(err)
	}
	if rows := storetest.QueryRaw(t, fixture.database.Path(), "SELECT id FROM problem_report"); len(rows) != 1 {
		t.Errorf("after Expire the table holds %v; want the newer report alone", rows)
	}
}

// Delete deletes one report at once, and says when there was none to delete.
func TestDelete_TakesOneReportAtOnce(t *testing.T) {
	t.Parallel()
	fixture := newKeeperFixture(t)
	kept, deleting := fixture.report(t, "kept"), fixture.report(t, "deleted")
	if deleted, err := fixture.keeper.Delete(t.Context(), deleting); err != nil || !deleted {
		t.Errorf("Delete = %v, %v; want it deleted", deleted, err)
	}
	if deleted, err := fixture.keeper.Delete(t.Context(), deleting); err != nil || deleted {
		t.Errorf("Delete again = %v, %v; want none deleted", deleted, err)
	}
	if listed, err := fixture.keeper.List(t.Context()); err != nil || len(listed) != 1 || listed[0].ID != kept {
		t.Errorf("List after Delete = %+v, %v; want the kept report alone", listed, err)
	}
}
