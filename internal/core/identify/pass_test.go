// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

const root = "/media/library"

// world is a library on one node, filled by the real ingest from node records.
type world struct {
	t         *testing.T
	database  *store.Database
	ingester  *library.Ingester
	pass      *Pass
	provider  *fakeProvider
	matcher   *Matcher
	chooser   *Chooser
	settings  *settings.Service
	clock     *clocktest.Fake
	nodeID    int64
	libraryID int64
	seq       int64
}

func newWorld(t *testing.T, kind, source string) *world {
	t.Helper()
	database := storetest.Open(t)
	now := clocktest.NewFake(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))
	logger := slog.New(slog.DiscardHandler)
	provider := newFakeProvider()
	providers := Providers{"tmdb": provider}
	config := settingstest.Seeded(t, database)
	created := &world{
		t: t, database: database, clock: now, provider: provider, settings: config,
		ingester: library.NewIngester(database, now, config, logger),
		pass:     NewPass(database, now, logger, providers, config),
	}
	created.matcher = NewMatcher(database, storetest.OpenCache(t), created.pass, now, logger)
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		var err error
		created.nodeID, err = queries.InsertNode(t.Context(), sqlcgen.InsertNodeParams{Name: "local", EnrolledAt: 1})
		if err != nil {
			return fmt.Errorf("node: %w", err)
		}
		created.libraryID, err = queries.InsertLibrary(t.Context(), sqlcgen.InsertLibraryParams{
			Name: "Library", Kind: kind, MetadataSource: source, ChangeDetection: "manual", CreatedAt: 1,
		})
		if err != nil {
			return fmt.Errorf("library: %w", err)
		}
		return queries.InsertLibraryRoot(t.Context(), sqlcgen.InsertLibraryRootParams{
			LibraryID: created.libraryID, NodeID: created.nodeID, Path: root,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	created.chooser = NewChooser(created.matcher, created.pass)
	return created
}

func video(path string) nodeproto.Record {
	sum := sha256.Sum256([]byte(path))
	return nodeproto.Record{Type: nodeproto.RecordObserved, Observed: &nodeproto.Observed{
		Path: nodeproto.Path{Text: path}, SizeBytes: 1, MtimeMs: 1,
		Identity: nodeproto.Identity{
			Scheme: "payload-v1", Anchor: "whole-file",
			PayloadHash: hex.EncodeToString(sum[:]),
		},
		Probe: nodeproto.Probe{Format: nodeproto.ProbeFormat{DurationMs: 5_000}},
	}}
}

// videoMoved is video at a new path with the payload it had at from.
func videoMoved(from, to string) nodeproto.Record {
	record := video(from)
	record.Observed.Path.Text = to
	return record
}

func nfo(path, body string) nodeproto.Record {
	return nodeproto.Record{Type: nodeproto.RecordSidecar, Sidecar: &nodeproto.Sidecar{
		Path: nodeproto.Path{Text: path}, Kind: nodeproto.SidecarNFO, SizeBytes: int64(len(body)), MtimeMs: 1,
		Body: []byte(body),
	}}
}

// scan ingests a complete scan of the root, then runs the pass.
func (lib *world) scan(records ...nodeproto.Record) Report {
	lib.t.Helper()
	lib.clock.Advance(time.Minute)
	all := append([]nodeproto.Record{{Type: nodeproto.RecordScanBegin, ScanBegin: &nodeproto.ScanBegin{Root: root}}},
		records...)
	complete := nodeproto.ScanComplete{Root: root}
	all = append(all, nodeproto.Record{Type: nodeproto.RecordScanComplete, ScanComplete: &complete})
	for index := range all {
		lib.seq++
		all[index].Seq = lib.seq
	}
	if err := lib.ingester.Records(lib.t.Context(), lib.nodeID, all); err != nil {
		lib.t.Fatal(err)
	}
	report, err := lib.pass.Library(lib.t.Context(), lib.libraryID)
	if err != nil {
		lib.t.Fatal(err)
	}
	return report
}

// files is each live video's title, by path below the root.
func (lib *world) files() map[string]sqlcgen.ListFileTitlesRow {
	lib.t.Helper()
	rows, err := lib.database.Read().ListFileTitles(lib.t.Context(), lib.libraryID)
	if err != nil {
		lib.t.Fatal(err)
	}
	byPath := map[string]sqlcgen.ListFileTitlesRow{}
	for _, row := range rows {
		byPath[strings.TrimPrefix(row.Path, root+"/")] = row
	}
	return byPath
}

func name(row sqlcgen.ListFileTitlesRow) string {
	if row.Name == nil {
		return ""
	}
	return *row.Name
}

const devNFO = `<?xml version="1.0" encoding="UTF-8"?>
<movie>
  <title>Two Versions Film</title>
  <year>2019</year>
  <plot>A synthetic film.</plot>
</movie>`

func TestPass_TheDevLibrarysFilms(t *testing.T) {
	t.Parallel()
	lib := newWorld(t, "movies", "nfo")
	report := lib.scan(
		nfo("Metropolis (1995)/Metropolis (1995).nfo", `<movie><title>Metropolis</title><year>1995</year>`+
			`<uniqueid type="tmdb">9019</uniqueid><lockedfields>Name</lockedfields></movie>`),
		video("Metropolis (1995)/Metropolis (1995).mkv"),
		video("Two Versions Film (2019)/Two Versions Film (2019) - 1080p.mkv"),
		video("Two Versions Film (2019)/Two Versions Film (2019) - 720p.mkv"),
		nfo("Two Versions Film (2019)/Two Versions Film (2019).nfo", devNFO),
		video("Two Versions Film (2019)/trailers/Two Versions Film trailer.mp4"),
		video("Unidentified/untitled rip final v2.mkv"),
	)
	files := lib.files()
	checkMetropolis(t, files["Metropolis (1995)/Metropolis (1995).mkv"])
	checkVersionsAndTrailer(t, files)
	checkUnidentified(t, files["Unidentified/untitled rip final v2.mkv"])
	if report.Listed != 3 || report.Extras != 1 || report.Unidentified != 1 {
		t.Errorf("report %+v, want 3 listed, the trailer an extra, and 1 unidentified", report)
	}
}

func checkMetropolis(t *testing.T, heat sqlcgen.ListFileTitlesRow) {
	t.Helper()
	if name(heat) != "Metropolis" || heat.Year == nil || *heat.Year != 1995 || heat.Identification != identIdentified ||
		heat.Filing != filingListed || heat.FollowedSource == nil || *heat.FollowedSource != sourceNFO {
		t.Errorf("Metropolis = %+v, want identified from its NFO and listed", heat)
	}
}

func checkVersionsAndTrailer(t *testing.T, files map[string]sqlcgen.ListFileTitlesRow) {
	t.Helper()
	high := files["Two Versions Film (2019)/Two Versions Film (2019) - 1080p.mkv"]
	low := files["Two Versions Film (2019)/Two Versions Film (2019) - 720p.mkv"]
	if high.TitleID != low.TitleID || name(high) != "Two Versions Film" || high.Filing != filingListed {
		t.Errorf("the two versions are %+v and %+v, want one listed title (D155)", high, low)
	}
	trailer := files["Two Versions Film (2019)/trailers/Two Versions Film trailer.mp4"]
	if trailer.Kind != kindExtra || trailer.TitleID == high.TitleID {
		t.Errorf("the trailer is %+v, want an extra of its own (D162)", trailer)
	}
	if trailer.Filing != filingExtra || trailer.FilingReason == nil ||
		*trailer.FilingReason != "trailer of Two Versions Film (2019), by its trailers folder" {
		t.Errorf("the trailer is filed %s (%v), want an extra with its owner, type and evidence (D163)",
			trailer.Filing, ptr.ValueOrZero(trailer.FilingReason))
	}
}

func checkUnidentified(t *testing.T, rip sqlcgen.ListFileTitlesRow) {
	t.Helper()
	if rip.Filing != filingUnidentified || rip.FilingReason == nil || *rip.FilingReason != "no NFO file" {
		t.Errorf("the rip = %+v, want unidentified: no NFO in an NFO-only library", rip)
	}
}

func TestPass_ASeriesFromItsFoldersAndItsTvshowNFO(t *testing.T) {
	t.Parallel()
	lib := newWorld(t, "shows", "nfo")
	lib.scan(
		nfo("Test Show/tvshow.nfo", `<tvshow><title>Test Show</title><year>2024</year></tvshow>`),
		video("Test Show/Season 01/Test Show - S01E01.mkv"),
		video("Test Show/Season 01/Test Show - S01E02.mkv"),
	)
	files := lib.files()
	first := files["Test Show/Season 01/Test Show - S01E01.mkv"]
	second := files["Test Show/Season 01/Test Show - S01E02.mkv"]
	if first.Kind != "episode" || first.Filing != "listed" || first.TitleID == second.TitleID {
		t.Errorf("episodes %+v and %+v, want two listed episode titles", first, second)
	}
	series, err := lib.database.Read().ListLibrarySeries(t.Context(), &lib.libraryID)
	if err != nil || len(series) != 1 || series[0].Name == nil || *series[0].Name != "Test Show" {
		t.Fatalf("series %+v, %v, want Test Show", series, err)
	}
	episodes, err := lib.database.Read().ListEpisodesOfSeries(t.Context(), series[0].ID)
	if err != nil || len(episodes) != 2 || *episodes[0].NumberInGroup != 1 || *episodes[1].NumberInGroup != 2 {
		t.Errorf("aired order %+v, %v, want episodes 1 and 2", episodes, err)
	}
}

func TestPass_RunningAgainChangesNothing(t *testing.T) {
	t.Parallel()
	lib := newWorld(t, "movies", "nfo")
	records := []nodeproto.Record{
		video("Metropolis (1995)/Metropolis (1995).mkv"), nfo("Metropolis (1995)/Metropolis (1995).nfo", `<movie><title>Metropolis</title></movie>`),
		video("Metropolis (1995)/trailers/Metropolis trailer.mkv"),
	}
	lib.scan(records...)
	before := lib.files()
	lib.scan(records...)
	for path, row := range lib.files() {
		if before[path].TitleID != row.TitleID {
			t.Errorf("%s moved from title %d to %d on a second pass over the same files",
				path, before[path].TitleID, row.TitleID)
		}
	}
}

// S1, D208 §1: fixing a typo in a name re-identifies the file; a confirmed match never moves.
func TestPass_ARenamedFileIsIdentifiedAgainUnlessConfirmed(t *testing.T) {
	t.Parallel()
	lib := newWorld(t, "movies", "nfo_then_tmdb")
	lib.scan(video("Metropols (1995).mkv"), video("Nosferatu (1998).mkv"))
	typo := lib.files()["Metropols (1995).mkv"]
	ronin := lib.files()["Nosferatu (1998).mkv"]
	if err := lib.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.ConfirmTitleMatch(t.Context(), ronin.TitleID)
	}); err != nil {
		t.Fatal(err)
	}

	lib.scan(videoMoved("Metropols (1995).mkv", "Metropolis (1995).mkv"), videoMoved("Nosferatu (1998).mkv", "Nosferatu 2 (1998).mkv"))
	fixed := lib.files()["Metropolis (1995).mkv"]
	if fixed.TitleID == typo.TitleID || name(fixed) != "Metropolis" {
		t.Errorf("after the rename, the file is title %d %q, want a title called Metropolis (D208 §1)",
			fixed.TitleID, name(fixed))
	}
	if kept := lib.files()["Nosferatu 2 (1998).mkv"]; kept.TitleID != ronin.TitleID {
		t.Errorf("the confirmed file moved from title %d to %d; a person's match never moves (D172)",
			ronin.TitleID, kept.TitleID)
	}
}

// S6, D208 §2: a film moved to another library takes its title, and so its history, with it,
// found by the provider id its NFO names.
func TestPass_ATitleFollowsItsFilesToAnotherLibrary(t *testing.T) {
	t.Parallel()
	lib := newWorld(t, "movies", "nfo")
	const kids = "/media/kids"
	var kidsID int64
	err := lib.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		var err error
		kidsID, err = queries.InsertLibrary(t.Context(), sqlcgen.InsertLibraryParams{
			Name: "Kids", Kind: "movies", MetadataSource: "nfo", ChangeDetection: "manual", CreatedAt: 1,
		})
		if err != nil {
			return fmt.Errorf("library: %w", err)
		}
		return queries.InsertLibraryRoot(t.Context(), sqlcgen.InsertLibraryRootParams{
			LibraryID: kidsID, NodeID: lib.nodeID, Path: kids,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	const body = `<movie><title>The Kid</title><year>2014</year><uniqueid type="tmdb">480600</uniqueid></movie>`
	lib.scan(video("The Kid (2014)/The Kid (2014).mkv"), nfo("The Kid (2014)/The Kid (2014).nfo", body),
		video("Metropolis (1995).mkv"))
	the kid := lib.files()["The Kid (2014)/The Kid (2014).mkv"].TitleID

	// The film leaves this library...
	lib.scan(video("Metropolis (1995).mkv"))
	// ...and arrives in Kids.
	lib.clock.Advance(time.Minute)
	records := []nodeproto.Record{
		{Seq: 900, Type: nodeproto.RecordScanBegin, ScanBegin: &nodeproto.ScanBegin{Root: kids}},
		video("The Kid (2014)/The Kid (2014).mkv"), nfo("The Kid (2014)/The Kid (2014).nfo", body),
		{Seq: 903, Type: nodeproto.RecordScanComplete, ScanComplete: &nodeproto.ScanComplete{Root: kids}},
	}
	records[1].Seq, records[2].Seq = 901, 902
	if ingestErr := lib.ingester.Records(t.Context(), lib.nodeID, records); ingestErr != nil {
		t.Fatal(ingestErr)
	}
	if _, passErr := lib.pass.Library(t.Context(), kidsID); passErr != nil {
		t.Fatal(passErr)
	}
	rows, err := lib.database.Read().ListFileTitles(t.Context(), kidsID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("Kids holds %+v, %v", rows, err)
	}
	if rows[0].TitleID != the kid {
		t.Errorf("The Kid in Kids is title %d, want %d: the title follows its files (D208 §2)",
			rows[0].TitleID, the kid)
	}
}

func TestPass_NamesAreSortedAndFoundInEveryScript(t *testing.T) {
	t.Parallel()
	lib := newWorld(t, "movies", "nfo")
	lib.scan(video("Sølvi Live (2020).mkv"), video("Fièvre (2001).mkv"), video("十字路 (2001).mkv"))
	browser := library.NewBrowser(lib.database, lib.settings)
	byName := library.ListQuery{Kind: library.ListFilms, Sort: library.SortName, Limit: 10}
	listed, err := browser.List(t.Context(), byName)
	var order []string
	for _, entry := range listed.Entries {
		order = append(order, entry.Name)
	}
	if err != nil || !slices.Equal(order, []string{"Fièvre", "Sølvi Live", "十字路"}) {
		t.Errorf("films by name = %v, %v", order, err)
	}
	for query, want := range map[string]string{"solvi": "Sølvi Live", "十字": "十字路", "ame": "Fièvre"} {
		hits, err := browser.Search(t.Context(), query, 5)
		if err != nil || len(hits) == 0 || hits[0].Name != want {
			t.Errorf("Search(%q) = %+v, %v; want %q", query, hits, err, want)
		}
	}
}

func TestRefreshSortKeys_ALocaleChangeTakesEffectWithoutAScan(t *testing.T) {
	t.Parallel()
	lib := newWorld(t, "movies", "nfo")
	lib.scan(video("Destiny (1998).mkv"), video("Ödet (2010).mkv"), video("Apa (2000).mkv"))
	browser := library.NewBrowser(lib.database, lib.settings)
	names := func() []string {
		t.Helper()
		byName := library.ListQuery{Kind: library.ListFilms, Sort: library.SortName, Limit: 10}
		listed, err := browser.List(t.Context(), byName)
		if err != nil {
			t.Fatal(err)
		}
		var order []string
		for _, entry := range listed.Entries {
			order = append(order, entry.Name)
		}
		return order
	}
	settingstest.Set(t, lib.settings, settings.SortLocale, `"sv"`)
	if written, err := lib.pass.RefreshSortKeys(t.Context()); err != nil || written != 3 {
		t.Fatalf("RefreshSortKeys = %d, %v; want every film keyed again", written, err)
	}
	if got := names(); !slices.Equal(got, []string{"Apa", "Destiny", "Ödet"}) {
		t.Errorf("in Swedish, with no scan since the change: %v", got)
	}
	if written, err := lib.pass.RefreshSortKeys(t.Context()); err != nil || written != 0 {
		t.Errorf("a second refresh wrote %d (%v); nothing was missing", written, err)
	}
}

// D211 §2: a name from a path or an NFO is stored as a display may show it.
func TestPass_ANameIsStoredClean(t *testing.T) {
	t.Parallel()
	lib := newWorld(t, "movies", "nfo_then_tmdb")
	hostile := "<movie><title>Metropolis\u202Evkm.exe\u200B</title><year>1995</year>" +
		"<plot>A saga.\n\n\n\nThe end.</plot></movie>"
	lib.scan(video("Metropolis (1995).mkv"), nfo("Metropolis (1995).nfo", hostile))
	if got := name(lib.files()["Metropolis (1995).mkv"]); got != "Heatvkm.exe" {
		t.Errorf("stored name %q; want the override and the zero-width space gone", got)
	}
}
