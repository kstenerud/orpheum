// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package store

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// The reference scale (performance-targets.md; schema.md measured its numbers here): a household
// library of films and series, with what a year of two people's watching leaves behind.
const (
	refFilms           = 3_500
	refSeries          = 200
	refEpisodesPerShow = 50
	refPlaySpans       = 4_000
	// A step that takes longer than stepBudget at the reference scale fails (migrations.md).
	stepBudget = 10 * time.Second
)

// refFiller writes the reference-scale rows through prepared statements in one transaction, at
// schema version 1, so every later step migrates real volumes.
type refFiller struct {
	tb      testing.TB
	ctx     context.Context //nolint:containedctx // the test's, for the statements it runs
	tx      *sql.Tx
	random  *rand.Rand
	nextID  int64
	fileID  int64
	streams int64
	now     int64
}

func (filler *refFiller) exec(query string, args ...any) {
	filler.tb.Helper()
	if _, err := filler.tx.ExecContext(filler.ctx, query, args...); err != nil {
		filler.tb.Fatalf("%s: %v", strings.Fields(query)[:3], err)
	}
}

// bytes is n bytes, a multiple of 8, the fill cannot compress away, as stored responses and
// hashes are.
func (filler *refFiller) bytes(n int) []byte {
	out := make([]byte, n)
	for index := 0; index+8 <= n; index += 8 {
		binary.LittleEndian.PutUint64(out[index:], filler.random.Uint64())
	}
	return out
}

// words is a synthetic phrase of n words (D187: no real titles in fixtures).
func (filler *refFiller) words(n int) string {
	parts := make([]string, n)
	for index := range parts {
		parts[index] = refWords[filler.random.IntN(len(refWords))]
	}
	return strings.Join(parts, " ")
}

var refWords = strings.Fields("harbour lantern quiet orchard copper winter paper signal hollow meridian " +
	"salt violet granite ember tidal northern glass velvet iron amber river station letter garden " +
	"silent autumn falcon island bridge shadow marble crimson canyon echo")

// item inserts an item and its text and sort key, and returns its id.
func (filler *refFiller) item(kind string, libraryID int64, listing string) int64 {
	filler.nextID++
	id := filler.nextID
	name := filler.words(1 + filler.random.IntN(4))
	filler.exec("INSERT INTO item (id, kind, library_id, created_at) VALUES (?, ?, ?, ?)", id, kind, libraryID,
		filler.now-filler.random.Int64N(365*24*3600*1000))
	filler.exec("INSERT INTO item_text (item_id, lang, is_original, name, overview, tagline) "+
		"VALUES (?, 'und', 1, ?, ?, ?)",
		id, name, filler.words(60), filler.words(8))
	if listing != "" {
		filler.exec("INSERT INTO sort_key (item_id, locale, listing, key) VALUES (?, 'en', ?, ?)", id, listing,
			append([]byte(name), 0, 1, 2, 3))
	}
	return id
}

// video inserts a title's version, and its file with a probe and four streams.
func (filler *refFiller) video(titleID, libraryID int64, path string) {
	filler.exec("INSERT INTO version (title_id, created_at) VALUES (?, ?)", titleID, filler.now)
	var versionID int64
	if err := filler.tx.QueryRowContext(filler.ctx, "SELECT last_insert_rowid()").Scan(&versionID); err != nil {
		filler.tb.Fatal(err)
	}
	filler.fileID++
	probe := fmt.Sprintf(`{"format":{"format_name":"matroska,webm","duration":"%d.0","bit_rate":"%d","tags":%q},`+
		`"streams":[{"codec_name":"hevc","width":3840,"height":2160},{"codec_name":"truehd","channels":8},`+
		`{"codec_name":"eac3","channels":6},{"codec_name":"subrip"}],"chapters":%q}`,
		5400+filler.random.IntN(3600), 20_000_000+filler.random.IntN(40_000_000), filler.words(40), filler.words(70))
	filler.exec(`INSERT INTO file (id, library_id, node_id, path, size, mtime, payload_hash, payload_len, filing,
		probe, version_id, first_seen, last_seen) VALUES (?, ?, 1, ?, ?, ?, ?, ?, 'listed', ?, ?, ?, ?)`,
		filler.fileID, libraryID, path, 20_000_000_000, filler.now, filler.bytes(32), 19_000_000_000, probe,
		versionID, filler.now, filler.now)
	for index, stream := range []struct{ kind, codec, lang string }{
		{"video", "hevc", ""}, {"audio", "truehd", "en"}, {"audio", "eac3", "de"}, {"subtitle", "subrip", "en"},
	} {
		filler.streams++
		filler.exec(`INSERT INTO stream (file_id, media_file, stream_index, kind, codec, lang_raw, lang, channels)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, filler.fileID, filler.fileID, index, stream.kind, stream.codec,
			stream.lang, stream.lang, 8)
	}
}

// film inserts one film: its title, one or two versions, artwork, ids and stored response.
func (filler *refFiller) film(index int) {
	id := filler.item("title", 1, "movie")
	filler.exec(`INSERT INTO title (id, kind, year, runtime_ms, source, source_id, ident)
		VALUES (?, 'movie', ?, ?, 'tmdb', ?, 'identified')`,
		id, 1950+filler.random.IntN(76), 6_000_000, fmt.Sprint(index))
	for version := range 1 + boolInt(index%10 == 0) {
		filler.video(id, 1, fmt.Sprintf("/media/films/Film %d (%d)/Film %d - %d.mkv", index, index, index, version))
	}
	filler.provided(id, "movie", index)
}

// provided inserts an item's artwork, provider ids and stored provider response.
func (filler *refFiller) provided(itemID int64, kind string, index int) {
	for _, art := range []string{"poster", "backdrop"} {
		filler.exec("INSERT INTO artwork (item_id, kind, source, source_ref, content_hash) VALUES (?, ?, 'tmdb', ?, ?)",
			itemID, art, fmt.Sprintf("/%s%d.jpg", art, index), filler.bytes(32))
	}
	filler.exec("INSERT INTO item_provider_id (item_id, provider, provider_id) VALUES (?, 'tmdb', ?), (?, 'imdb', ?)",
		itemID, fmt.Sprint(index), itemID, fmt.Sprintf("tt%07d", index))
	if kind == "movie" {
		filler.exec("INSERT INTO provider_response (title_id, provider, fetched_at, body) VALUES (?, 'tmdb', ?, ?)",
			itemID, filler.now, filler.bytes(2_000))
	}
}

// series inserts one series with its episodes, their places in the aired order and their stills.
func (filler *refFiller) series(index int) {
	id := filler.item("collection", 2, "series")
	filler.exec("INSERT INTO collection (id, kind, membership, created_by) "+
		"VALUES (?, 'series', 'explicit', 'scanner')", id)
	filler.exec("INSERT INTO ordering (collection_id, name, provider) VALUES (?, 'aired', 'tmdb')", id)
	var orderingID int64
	if err := filler.tx.QueryRowContext(filler.ctx, "SELECT last_insert_rowid()").Scan(&orderingID); err != nil {
		filler.tb.Fatal(err)
	}
	filler.provided(id, "series", index)
	for episode := range refEpisodesPerShow {
		season, number := 1+episode/10, 1+episode%10
		episodeID := filler.item("title", 2, "")
		filler.exec(`INSERT INTO title (id, kind, year, runtime_ms, source, source_id, ident)
			VALUES (?, 'episode', ?, ?, 'tmdb', ?, 'identified')`, episodeID, 2000+index%26, 2_700_000,
			fmt.Sprintf("%d-%d", index, episode))
		filler.video(episodeID, 2, fmt.Sprintf("/media/shows/Show %d/Season %02d/Show %d - S%02dE%02d.mkv", index,
			season, index, season, number))
		sort := float64(season*10000 + number)
		filler.exec("INSERT INTO position (ordering_id, title_id, grp, number, sort) VALUES (?, ?, ?, ?, ?)",
			orderingID, episodeID, season, number, sort)
		filler.exec("INSERT INTO collection_member (collection_id, member_title, position) VALUES (?, ?, ?)", id,
			episodeID, sort)
		filler.exec("INSERT INTO artwork (item_id, kind, source, source_ref, content_hash) "+
			"VALUES (?, 'thumb', 'tmdb', ?, ?)",
			episodeID, fmt.Sprintf("/still%d-%d.jpg", index, episode), filler.bytes(32))
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// fillReferenceScale fills a database at schema version 1 with the reference-scale library.
func fillReferenceScale(t *testing.T, database *Database) {
	t.Helper()
	transaction, err := database.writer.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	random := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // a fixture, the same every run
	filler := &refFiller{
		tb: t, ctx: t.Context(), tx: transaction, random: random,
		now: time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC).UnixMilli(),
	}
	filler.exec("INSERT INTO node (id, name, enrolled_at) VALUES (1, 'local', 1)")
	filler.exec(`INSERT INTO library (id, name, kind, metadata_source, change_detection, writable, created_at)
		VALUES (1, 'Films', 'movies', 'nfo_then_tmdb', 'watch', 0, 1), (2, 'Series', 'shows', 'tmdb', 'watch', 0, 1)`)
	filler.exec("INSERT INTO library_root (library_id, node_id, path) " +
		"VALUES (1, 1, '/media/films'), (2, 1, '/media/shows')")
	filler.exec("INSERT INTO user (id, name, role_id, created_at) VALUES (1, 'owner', 1, 1), (2, 'partner', 2, 1)")
	for index := range refFilms {
		filler.film(index)
	}
	for index := range refSeries {
		filler.series(index)
	}
	for span := range refPlaySpans {
		fileID := 1 + filler.random.Int64N(filler.fileID)
		start := filler.now - int64(span)*3_600_000
		filler.exec(`INSERT INTO play_span (user_id, file_id, pos_from_ms, pos_to_ms, t_from, t_to)
			VALUES (?, ?, 0, 1800000, ?, ?)`, 1+span%2, fileID, start, start+1_800_000)
		filler.exec(`INSERT INTO play_position (user_id, file_id, position_ms, seq, updated_at)
			VALUES (?, ?, 1800000, ?, ?)
			ON CONFLICT (user_id, file_id) DO UPDATE SET seq = excluded.seq`, 1+span%2, fileID, span, start)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
}

// TestReferenceScale fills a database at schema version 1 with the reference-scale library
// (about 46 MB), then migrates it and reads its listing queries' plans. It is in the integration
// tier: the fill takes seconds.
func TestReferenceScale(t *testing.T) {
	t.Parallel()
	steps, err := Steps()
	if err != nil {
		t.Fatal(err)
	}
	database := openTemporary(t)
	if err = database.migrate(t.Context(), steps[:1], testMigrateOptions(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	wall := clock.System{}
	filled := wall.Now()
	fillReferenceScale(t, database)
	info, err := os.Stat(database.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("reference scale at schema 1: %.1f MB, filled in %v", float64(info.Size())/(1<<20), wall.Now().Sub(filled))
	if info.Size() < 30<<20 {
		t.Fatalf("the reference-scale database is %d bytes; it should be about 46 MB", info.Size())
	}
	// In order: the plans and pages are read on the migrated database.
	checkStepBudgets(t, database, steps)
	checkQueryPlans(t, database)
	checkPageBudgets(t, database)
}

// checkStepBudgets migrates from schema 1, timing each step (migrations.md: under 10 s each at
// the reference scale).
func checkStepBudgets(t *testing.T, database *Database, steps []Step) {
	t.Helper()
	wall := clock.System{}
	var took []time.Duration
	var started time.Time
	options := testMigrateOptions(t.TempDir())
	options.BeforeStep = func(int, int) {
		if !started.IsZero() {
			took = append(took, wall.Now().Sub(started))
		}
		started = wall.Now()
	}
	if err := database.migrate(t.Context(), steps, options); err != nil {
		t.Fatal(err)
	}
	took = append(took, wall.Now().Sub(started))
	for index, duration := range took {
		step := steps[index+1]
		t.Logf("step %04d %s: %v", step.Version, step.Name, duration)
		if duration > stepBudget {
			t.Errorf("step %04d %s took %v at the reference scale; the budget is %v (migrations.md)", step.Version,
				step.Name, duration, stepBudget)
		}
	}
}

// listQuery is one of the store's listing queries, as its file writes it.
type listQuery struct {
	name string
	sql  string
}

var (
	queryName = regexp.MustCompile(`(?m)^-- name: (\w+) :(\w+)\s*$`)
	queryArg  = regexp.MustCompile(`sqlc\.n?arg\(\w+\)`)
	fullScan  = regexp.MustCompile(`^SCAN (\w+)$`)
)

// listingQueries are the :many queries named List… in the query files, their arguments made
// placeholders.
func listingQueries(t *testing.T) []listQuery {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("queries", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	var found []listQuery
	for _, path := range paths {
		text, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		marks := queryName.FindAllSubmatchIndex(text, -1)
		for index, mark := range marks {
			name, kind := string(text[mark[2]:mark[3]]), string(text[mark[4]:mark[5]])
			end := len(text)
			if index+1 < len(marks) {
				end = marks[index+1][0]
			}
			if kind != "many" || !strings.HasPrefix(name, "List") {
				continue
			}
			var body []string
			for line := range strings.SplitSeq(string(text[mark[1]:end]), "\n") {
				if !strings.HasPrefix(strings.TrimSpace(line), "--") {
					body = append(body, line)
				}
			}
			found = append(found, listQuery{name: name, sql: queryArg.ReplaceAllString(strings.Join(body, "\n"), "?")})
		}
	}
	return found
}

// scans are the tables a query's plan reads whole.
func scans(t *testing.T, database *Database, query listQuery) []string {
	t.Helper()
	args := make([]any, strings.Count(query.sql, "?"))
	rows, err := database.readers.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+query.sql, args...)
	if err != nil {
		t.Fatalf("%s: %v", query.name, err)
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck // read-only
	var scanned []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		if match := fullScan.FindStringSubmatch(detail); match != nil {
			scanned = append(scanned, match[1])
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return scanned
}

// fullScansAllowed are the listing queries that read a whole table, each with why that is right
// at the reference scale. A new full scan fails until it is listed here with a reason, and a listed
// query that no longer scans fails until it is taken off (sql.md section 11).
func fullScansAllowed() map[string]string {
	return map[string]string{
		"ListArtworkHashes": "the artwork cache's sweep reads every stored image, once a day (D212 section 6)",
		"ListFilmsByAdded": "newest first sorts every live film: 12.5 ms at the reference scale against 50; " +
			"an index on item(created_at) when it is not",
		"ListSeriesByAdded": "newest first over the series, which are few: 200 at the reference scale",
		"ListTitlesByIdent": "the admin's needs-a-match list: 1.3 ms at the reference scale",
		"ListSeriesByIdent": "the same list's series, which are few",
		"ListLibraries":     "a household has a handful of libraries",
		"ListLiveNodes":     "a household has a handful of nodes",
	}
}

// checkQueryPlans reads every listing query's plan at the reference scale and fails on a full
// scan of a table not allowed with its reason.
func checkQueryPlans(t *testing.T, database *Database) {
	t.Helper()
	allowed := fullScansAllowed()
	for _, query := range listingQueries(t) {
		scanned := scans(t, database, query)
		reason, isAllowed := allowed[query.name]
		delete(allowed, query.name)
		switch {
		case len(scanned) > 0 && !isAllowed:
			t.Errorf("%s reads all of %v at the reference scale; give it an index, or list it in "+
				"fullScansAllowed with why that is right (sql.md section 11)", query.name, scanned)
		case len(scanned) == 0 && isAllowed:
			t.Errorf("%s no longer scans a table; take it off fullScansAllowed (%s)", query.name, reason)
		}
	}
	for name := range allowed {
		t.Errorf("fullScansAllowed names %s, which is no listing query", name)
	}
}

// pageBudget is performance-targets.md's for a library page or a poster grid.
const pageBudget = 50 * time.Millisecond

// checkPageBudgets times the first page of each browse list at the reference scale, best of five,
// against the page budget: a query shaped badly enough to read every file for every title took
// seconds here while the dev library showed nothing (audit-2026-09-25).
func checkPageBudgets(t *testing.T, database *Database) {
	t.Helper()
	queries := sqlcgen.New(database.readers)
	ctx := t.Context()
	pages := map[string]func() error{
		"films by name": func() error {
			first := sqlcgen.ListFilmsByNameParams{Locale: "en", AfterKey: []byte{}, MaxRows: 60}
			_, err := queries.ListFilmsByName(ctx, first)
			return err //nolint:wrapcheck // a test
		},
		"films newest first": func() error {
			first := sqlcgen.ListFilmsByAddedParams{BeforeAt: 1 << 62, BeforeID: 1 << 62, MaxRows: 60}
			_, err := queries.ListFilmsByAdded(ctx, first)
			return err //nolint:wrapcheck // a test
		},
		"series by name": func() error {
			first := sqlcgen.ListSeriesByNameParams{Locale: "en", AfterKey: []byte{}, MaxRows: 60}
			_, err := queries.ListSeriesByName(ctx, first)
			return err //nolint:wrapcheck // a test
		},
		"series newest first": func() error {
			first := sqlcgen.ListSeriesByAddedParams{BeforeAt: 1 << 62, BeforeID: 1 << 62, MaxRows: 60}
			_, err := queries.ListSeriesByAdded(ctx, first)
			return err //nolint:wrapcheck // a test
		},
		"a series' episodes": func() error {
			_, err := queries.ListSeriesEpisodeDetails(ctx, int64(refFilms)+1)
			return err //nolint:wrapcheck // a test
		},
	}
	wall := clock.System{}
	for name, page := range pages {
		best := time.Hour
		for range 5 {
			started := wall.Now()
			if err := page(); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			best = min(best, wall.Now().Sub(started))
		}
		t.Logf("%s: %v", name, best)
		if best > pageBudget {
			t.Errorf("%s took %v at the reference scale; a page's budget is %v (performance-targets.md)", name, best,
				pageBudget)
		}
	}
}
