// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

const libraryRoot = "/media/library"

// testLibrary is a library on one node, filled by the real ingest from node records.
type testLibrary struct {
	t         *testing.T
	database  *store.Database
	cache     *store.Cache
	logger    *slog.Logger
	ingester  *library.Ingester
	pass      *identify.Pass
	provider  *identifytest.Provider
	matcher   *identify.Matcher
	chooser   *identify.Chooser
	settings  *settings.Service
	clock     *clocktest.Fake
	nodeID    int64
	libraryID int64
	rootPath  string // the library's one root, as its node names it
}

func newTestLibrary(t *testing.T, kind, source string) *testLibrary {
	t.Helper()
	return newTestLibraryAt(t, kind, source, libraryRoot)
}

// newTestLibraryAt is newTestLibrary with the library's root at rootPath.
func newTestLibraryAt(t *testing.T, kind, source, rootPath string) *testLibrary {
	t.Helper()
	database := storetest.OpenMigrated(t)
	now := clocktest.NewFake(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))
	logger := slog.New(slog.DiscardHandler)
	provider := identifytest.NewProvider()
	providers := identify.Providers{"tmdb": provider}
	config := settingstest.Seeded(t, database)
	created := &testLibrary{
		t: t, database: database, cache: storetest.OpenCache(t), logger: logger, clock: now, provider: provider,
		settings: config, rootPath: rootPath,
		ingester: library.NewIngester(database, now, config, logger),
		pass:     identify.NewPass(database, now, logger, providers, config),
	}
	created.matcher = identify.NewMatcher(database, created.cache, created.pass, now, logger)
	created.nodeID, created.libraryID = librarytest.NodeWithLibrary(t, database,
		librarytest.Library{Name: "Library", Kind: kind, MetadataSource: source, Root: rootPath})
	created.chooser = identify.NewChooser(created.matcher, created.pass)
	return created
}

func videoRecord(path string) nodeproto.ScanRecord {
	sum := sha256.Sum256([]byte(path))
	return nodeproto.ScanRecord{Type: nodeproto.RecordObserved, Observed: &nodeproto.Observed{
		Path: nodeproto.Path{Text: path}, SizeBytes: 1, ModifiedAt: 1,
		Identity: nodeproto.Identity{
			Scheme: "payload-v1", Anchor: "whole-file",
			PayloadHash: hex.EncodeToString(sum[:]),
		},
		Probe: nodeproto.Probe{Format: nodeproto.ProbeFormat{DurationMs: 5_000}},
	}}
}

// videoMoved is videoRecord at a new path with the payload it had at from.
func videoMoved(from, to string) nodeproto.ScanRecord {
	record := videoRecord(from)
	record.Observed.Path.Text = to
	return record
}

func nfoRecord(path, body string) nodeproto.ScanRecord {
	return nodeproto.ScanRecord{Type: nodeproto.RecordSidecar, Sidecar: &nodeproto.Sidecar{
		Path: nodeproto.Path{Text: path}, Kind: nodeproto.SidecarNFO, SizeBytes: int64(len(body)), ModifiedAt: 1,
		NFOBody: []byte(body),
	}}
}

// scan ingests a complete scan of the root, then runs the pass.
func (lib *testLibrary) scan(records ...nodeproto.ScanRecord) identify.Report {
	lib.t.Helper()
	lib.clock.Advance(time.Minute)
	all := librarytest.Scan(nodeproto.ScanBegin{RootPath: lib.rootPath}, records, true)
	if err := lib.ingester.IngestScan(lib.t.Context(), lib.nodeID, lib.rootPath, all); err != nil {
		lib.t.Fatal(err)
	}
	report, err := lib.pass.IdentifyLibrary(lib.t.Context(), lib.libraryID)
	if err != nil {
		lib.t.Fatal(err)
	}
	return report
}

// runOwedPasses runs the passes matches and refreshes left owed, as their sweep does.
func (lib *testLibrary) runOwedPasses() {
	lib.t.Helper()
	if _, err := lib.pass.IdentifyOwed(lib.t.Context()); err != nil {
		lib.t.Fatal(err)
	}
}

// titlesByPath is each live video's title, by path below the root.
func (lib *testLibrary) titlesByPath() map[string]sqlcgen.ListFileTitlesRow {
	lib.t.Helper()
	rows, err := lib.database.Read().ListFileTitles(lib.t.Context(), lib.libraryID)
	if err != nil {
		lib.t.Fatal(err)
	}
	byPath := map[string]sqlcgen.ListFileTitlesRow{}
	for _, row := range rows {
		byPath[strings.TrimPrefix(row.Path, strings.TrimSuffix(library.PathText([]byte(lib.rootPath)), "/")+"/")] = row
	}
	return byPath
}

func titleName(row sqlcgen.ListFileTitlesRow) string {
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
	lib := newTestLibrary(t, "movies", "nfo")
	report := lib.scan(
		nfoRecord("Metropolis (1995)/Metropolis (1995).nfo", `<movie><title>Metropolis</title><year>1995</year>`+
			`<uniqueid type="tmdb">9019</uniqueid><lockedfields>Name</lockedfields></movie>`),
		videoRecord("Metropolis (1995)/Metropolis (1995).mkv"),
		videoRecord("Two Versions Film (2019)/Two Versions Film (2019) - 1080p.mkv"),
		videoRecord("Two Versions Film (2019)/Two Versions Film (2019) - 720p.mkv"),
		nfoRecord("Two Versions Film (2019)/Two Versions Film (2019).nfo", devNFO),
		videoRecord("Two Versions Film (2019)/trailers/Two Versions Film trailer.mp4"),
		videoRecord("Unidentified/untitled rip final v2.mkv"),
	)
	files := lib.titlesByPath()
	checkMetropolis(t, files["Metropolis (1995)/Metropolis (1995).mkv"])
	checkVersionsAndTrailer(t, files)
	checkUnidentified(t, files["Unidentified/untitled rip final v2.mkv"])
	if report.ListedCount != 3 || report.ExtraCount != 1 || report.UnidentifiedCount != 1 {
		t.Errorf("report %+v, want 3 listed, the trailer an extra, and 1 unidentified", report)
	}
}

func checkMetropolis(t *testing.T, heat sqlcgen.ListFileTitlesRow) {
	t.Helper()
	if titleName(heat) != "Metropolis" || heat.Year == nil || *heat.Year != 1995 ||
		heat.IdentificationState != identify.IdentificationIdentified || heat.Filing != identify.FilingListed ||
		heat.FollowedSource == nil || *heat.FollowedSource != identify.SourceNFO {
		t.Errorf("Metropolis = %+v, want identified from its NFO and listed", heat)
	}
}

func checkVersionsAndTrailer(t *testing.T, files map[string]sqlcgen.ListFileTitlesRow) {
	t.Helper()
	high := files["Two Versions Film (2019)/Two Versions Film (2019) - 1080p.mkv"]
	low := files["Two Versions Film (2019)/Two Versions Film (2019) - 720p.mkv"]
	if high.TitleID != low.TitleID || titleName(high) != "Two Versions Film" || high.Filing != identify.FilingListed {
		t.Errorf("the two versions are %+v and %+v, want one listed title (D155)", high, low)
	}
	trailer := files["Two Versions Film (2019)/trailers/Two Versions Film trailer.mp4"]
	if trailer.Kind != identify.KindExtra || trailer.TitleID == high.TitleID {
		t.Errorf("the trailer is %+v, want an extra of its own (D162)", trailer)
	}
	if trailer.Filing != identify.FilingExtra || trailer.FilingReason == nil ||
		*trailer.FilingReason != "trailer of Two Versions Film (2019), by its trailers folder" {
		t.Errorf("the trailer is filed %s (%v), want an extra with its owner, type and evidence (D163)",
			trailer.Filing, ptr.ValueOrZero(trailer.FilingReason))
	}
}

func checkUnidentified(t *testing.T, rip sqlcgen.ListFileTitlesRow) {
	t.Helper()
	if rip.Filing != identify.FilingUnidentified || rip.FilingReason == nil || *rip.FilingReason != "no NFO file" {
		t.Errorf("the rip = %+v, want unidentified: no NFO in an NFO-only library", rip)
	}
}

func TestPass_ASeriesFromItsFoldersAndItsTvshowNFO(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "shows", "nfo")
	lib.scan(
		nfoRecord("Test Show/tvshow.nfo", `<tvshow><title>Test Show</title><year>2024</year></tvshow>`),
		videoRecord("Test Show/Season 01/Test Show - S01E01.mkv"),
		videoRecord("Test Show/Season 01/Test Show - S01E02.mkv"),
	)
	files := lib.titlesByPath()
	first := files["Test Show/Season 01/Test Show - S01E01.mkv"]
	second := files["Test Show/Season 01/Test Show - S01E02.mkv"]
	if first.Kind != "episode" || first.Filing != "listed" || first.TitleID == second.TitleID {
		t.Errorf("episodes %+v and %+v, want two listed episode titles", first, second)
	}
	series, err := lib.database.Read().ListLibrarySeries(t.Context(), &lib.libraryID)
	if err != nil || len(series) != 1 || series[0].Name == nil || *series[0].Name != "Test Show" {
		t.Fatalf("series %+v, %v, want Test Show", series, err)
	}
	episodes, err := lib.database.Read().ListSeriesEpisodePlaces(t.Context(), series[0].ID)
	if err != nil || len(episodes) != 2 || *episodes[0].NumberInGroup != 1 || *episodes[1].NumberInGroup != 2 {
		t.Errorf("aired order %+v, %v, want episodes 1 and 2", episodes, err)
	}
}

// D232: what makes two names one work is one fold. A series whose folders spell its name two
// ways, as "Lantern's X" and "Marvels X", is one series, as the matcher already took it to be.
func TestPass_ASeriesWhoseFoldersSpellItsNameTwoWaysIsOneSeries(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "shows", "nfo")
	lib.scan(
		videoRecord("Lantern's Agents/Season 01/Lantern's Agents - S01E01.mkv"),
		videoRecord("Lanterns Agents/Season 01/Lanterns Agents - S01E02.mkv"),
	)
	series, err := lib.database.Read().ListLibrarySeries(t.Context(), &lib.libraryID)
	if err != nil || len(series) != 1 {
		t.Fatalf("series %+v, %v; want one series for both spellings", series, err)
	}
	episodes, err := lib.database.Read().ListSeriesEpisodePlaces(t.Context(), series[0].ID)
	if err != nil || len(episodes) != 2 {
		t.Errorf("the series' episodes %+v, %v; want both", episodes, err)
	}
}

// The series already filed, under the name its tvshow.nfo gives it, is found by a new folder
// that spells that name another way.
func TestPass_ANewFolderSpellingAFiledSeriesNameAnotherWayJoinsIt(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "shows", "nfo")
	filed := []nodeproto.ScanRecord{
		nfoRecord("Agents/tvshow.nfo", `<tvshow><title>Lantern's Agents</title></tvshow>`),
		videoRecord("Agents/Season 01/Agents - S01E01.mkv"),
	}
	lib.scan(filed...)
	lib.scan(append(filed, videoRecord("Lanterns Agents/Season 01/Lanterns Agents - S01E02.mkv"))...)
	series, err := lib.database.Read().ListLibrarySeries(t.Context(), &lib.libraryID)
	if err != nil || len(series) != 1 || series[0].Name == nil || *series[0].Name != "Lantern's Agents" {
		t.Fatalf("series %+v, %v; want Lantern's Agents alone", series, err)
	}
}

// A film whose file and folder spell its name another way than the one already filed is the
// same film.
func TestPass_AFilmSpelledAnotherWayWithItsYearIsTheSameFilm(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "nfo")
	lib.scan(
		videoRecord("Moth-Lantern (2002)/Moth-Lantern (2002).mkv"),
		videoRecord("Moth Lantern (2002)/Moth Lantern (2002) 4K.mkv"),
	)
	files := lib.titlesByPath()
	first := files["Moth-Lantern (2002)/Moth-Lantern (2002).mkv"]
	second := files["Moth Lantern (2002)/Moth Lantern (2002) 4K.mkv"]
	if first.TitleID == 0 || first.TitleID != second.TitleID {
		t.Errorf("the two files are titles %d and %d; want one film", first.TitleID, second.TitleID)
	}
}

func TestPass_RunningAgainChangesNothing(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "nfo")
	records := []nodeproto.ScanRecord{
		videoRecord("Metropolis (1995)/Metropolis (1995).mkv"), nfoRecord("Metropolis (1995)/Metropolis (1995).nfo",
			`<movie><title>Metropolis</title></movie>`),
		videoRecord("Metropolis (1995)/trailers/Metropolis trailer.mkv"),
	}
	lib.scan(records...)
	before := lib.titlesByPath()
	lib.scan(records...)
	for path, row := range lib.titlesByPath() {
		if before[path].TitleID != row.TitleID {
			t.Errorf("%s moved from title %d to %d on a second pass over the same files",
				path, before[path].TitleID, row.TitleID)
		}
	}
}

// S1, D208 §1: fixing a typo in a name re-identifies the file; a confirmed match never moves.
func TestPass_ARenamedFileIsIdentifiedAgainUnlessConfirmed(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "nfo_then_tmdb")
	lib.scan(videoRecord("Metropols (1995).mkv"), videoRecord("Nosferatu (1998).mkv"))
	typo := lib.titlesByPath()["Metropols (1995).mkv"]
	ronin := lib.titlesByPath()["Nosferatu (1998).mkv"]
	if err := lib.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.ConfirmTitleMatch(t.Context(), ronin.TitleID)
	}); err != nil {
		t.Fatal(err)
	}

	lib.scan(videoMoved("Metropols (1995).mkv", "Metropolis (1995).mkv"), videoMoved("Nosferatu (1998).mkv", "Nosferatu 2 (1998).mkv"))
	fixed := lib.titlesByPath()["Metropolis (1995).mkv"]
	if fixed.TitleID == typo.TitleID || titleName(fixed) != "Metropolis" {
		t.Errorf("after the rename, the file is title %d %q, want a title called Metropolis (D208 §1)",
			fixed.TitleID, titleName(fixed))
	}
	if kept := lib.titlesByPath()["Nosferatu 2 (1998).mkv"]; kept.TitleID != ronin.TitleID {
		t.Errorf("the confirmed file moved from title %d to %d; a person's match never moves (D172)",
			ronin.TitleID, kept.TitleID)
	}
}

// S6, D208 §2: a film moved to another library takes its title, and so its history, with it,
// found by the provider id its NFO names.
func TestPass_ATitleFollowsItsFilesToAnotherLibrary(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "nfo")
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
	lib.scan(videoRecord("The Kid (2014)/The Kid (2014).mkv"),
		nfoRecord("The Kid (2014)/The Kid (2014).nfo", body),
		videoRecord("Metropolis (1995).mkv"))
	the kid := lib.titlesByPath()["The Kid (2014)/The Kid (2014).mkv"].TitleID

	// The film leaves this library...
	lib.scan(videoRecord("Metropolis (1995).mkv"))
	// ...and arrives in Kids.
	lib.clock.Advance(time.Minute)
	records := librarytest.Scan(nodeproto.ScanBegin{RootPath: kids}, []nodeproto.ScanRecord{
		videoRecord("The Kid (2014)/The Kid (2014).mkv"),
		nfoRecord("The Kid (2014)/The Kid (2014).nfo", body),
	}, true)
	if ingestErr := lib.ingester.IngestScan(t.Context(), lib.nodeID, kids, records); ingestErr != nil {
		t.Fatal(ingestErr)
	}
	if _, passErr := lib.pass.IdentifyLibrary(t.Context(), kidsID); passErr != nil {
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
	lib := newTestLibrary(t, "movies", "nfo")
	lib.scan(videoRecord("Sølvi Live (2020).mkv"), videoRecord("Fièvre (2001).mkv"), videoRecord("十字路 (2001).mkv"))
	browser := library.NewBrowser(lib.database, lib.settings)
	byName := library.ListQuery{Reach: auth.EveryLibrary(), Kind: library.ListFilms, Sort: library.SortName, Limit: 10}
	listed, err := browser.List(t.Context(), byName)
	var order []string
	for _, entry := range listed.Entries {
		order = append(order, entry.Name)
	}
	if err != nil || !slices.Equal(order, []string{"Fièvre", "Sølvi Live", "十字路"}) {
		t.Errorf("films by name = %v, %v", order, err)
	}
	for query, want := range map[string]string{"solvi": "Sølvi Live", "十字": "十字路", "ame": "Fièvre"} {
		hits, err := browser.Search(t.Context(), auth.EveryLibrary(), query, 5)
		if err != nil || len(hits) == 0 || hits[0].Name != want {
			t.Errorf("Search(%q) = %+v, %v; want %q", query, hits, err, want)
		}
	}
}

func TestRefreshSortAndSearchKeys_ALocaleChangeTakesEffectWithoutAScan(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "nfo")
	lib.scan(videoRecord("Destiny (1998).mkv"), videoRecord("Ödet (2010).mkv"), videoRecord("Apa (2000).mkv"))
	browser := library.NewBrowser(lib.database, lib.settings)
	names := func() []string {
		t.Helper()
		byName := library.ListQuery{
			Reach: auth.EveryLibrary(), Kind: library.ListFilms, Sort: library.SortName, Limit: 10,
		}
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
	if written, err := lib.pass.RefreshSortAndSearchKeys(t.Context()); err != nil || written != 3 {
		t.Fatalf("RefreshSortAndSearchKeys = %d, %v; want every film keyed again", written, err)
	}
	if got := names(); !slices.Equal(got, []string{"Apa", "Destiny", "Ödet"}) {
		t.Errorf("in Swedish, with no scan since the change: %v", got)
	}
	if written, err := lib.pass.RefreshSortAndSearchKeys(t.Context()); err != nil || written != 0 {
		t.Errorf("a second refresh wrote %d (%v); nothing was missing", written, err)
	}
}

// A build that collates or folds otherwise finds every key and fold the last one stored; each is
// made again from its name, not only the missing ones (D47, D48).
func TestRefreshSortAndSearchKeys_AKeyOrFoldAnotherBuildMadeIsMadeAgain(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "nfo")
	lib.scan(videoRecord("Destiny (1998).mkv"), videoRecord("Sølvi Live (2020).mkv"), videoRecord("Apa (2000).mkv"))
	// What an older build stored: a key that sorted Apa last, and Sølvi folded with its ø kept.
	storetest.ExecRaw(t, lib.database.Path(),
		"UPDATE sort_key SET collation_key = X'FFFFFFFF' "+
			"WHERE item_id = (SELECT item_id FROM item_text WHERE name = 'Apa')",
		"UPDATE item_text SET search_alternates = 'sølvi live' WHERE name = 'Sølvi Live'")
	browser := library.NewBrowser(lib.database, lib.settings)
	if hits, err := browser.Search(t.Context(), auth.EveryLibrary(), "solvi", 5); err != nil || len(hits) != 0 {
		t.Fatalf("Search(solvi) under the older fold = %+v, %v; the setup did not take", hits, err)
	}
	if rewrittenCount, err := lib.pass.RefreshSortAndSearchKeys(t.Context()); err != nil || rewrittenCount != 2 {
		t.Fatalf("RefreshSortAndSearchKeys = %d, %v; want Apa keyed and Sølvi folded again", rewrittenCount, err)
	}
	byName := library.ListQuery{Reach: auth.EveryLibrary(), Kind: library.ListFilms, Sort: library.SortName, Limit: 10}
	listed, err := browser.List(t.Context(), byName)
	var order []string
	for _, entry := range listed.Entries {
		order = append(order, entry.Name)
	}
	if err != nil || !slices.Equal(order, []string{"Apa", "Sølvi Live", "Destiny"}) {
		t.Errorf("films by name after the refresh = %v, %v", order, err)
	}
	hits, err := browser.Search(t.Context(), auth.EveryLibrary(), "solvi", 5)
	if err != nil || len(hits) != 1 || hits[0].Name != "Sølvi Live" {
		t.Errorf("Search(solvi) after the refresh = %+v, %v; want Sølvi Live", hits, err)
	}
	if rewrittenCount, err := lib.pass.RefreshSortAndSearchKeys(t.Context()); err != nil || rewrittenCount != 0 {
		t.Errorf("a second refresh rewrote %d (%v); every key and fold was current", rewrittenCount, err)
	}
}

// D211 §2: a name from a path or an NFO is stored as a display may show it.
func TestPass_ANameIsStoredClean(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "nfo_then_tmdb")
	hostile := "<movie><title>Metropolis\u202Evkm.exe\u200B</title><year>1995</year>" +
		"<plot>A saga.\n\n\n\nThe end.</plot></movie>"
	lib.scan(videoRecord("Metropolis (1995).mkv"), nfoRecord("Metropolis (1995).nfo", hostile))
	if got := titleName(lib.titlesByPath()["Metropolis (1995).mkv"]); got != "Heatvkm.exe" {
		t.Errorf("stored name %q; want the override and the zero-width space gone", got)
	}
}

func TestPass_FilesUnderARootNotInNFCOrOfSlashAreFiled(t *testing.T) {
	t.Parallel()
	for _, rootPath := range []string{"/media/Filme\u0301s", "/"} {
		lib := newTestLibraryAt(t, "movies", "nfo", rootPath)
		lib.scan(
			nfoRecord("Metropolis (1995)/Metropolis (1995).nfo", "<movie><title>Metropolis</title><year>1995</year></movie>"),
			videoRecord("Metropolis (1995)/Metropolis (1995).mkv"),
		)
		heat := lib.titlesByPath()["Metropolis (1995)/Metropolis (1995).mkv"]
		if titleName(heat) != "Metropolis" || heat.Filing != identify.FilingListed {
			t.Errorf("root %s: Metropolis is %+v, want it filed from its NFO and listed", rootPath, heat)
		}
	}
}
