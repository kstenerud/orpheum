// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package store

import (
	"context"
	"database/sql"
	"encoding/binary"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
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
	// A year of sign-ins on the two people's four devices, most of them ended since.
	refSessions = 40
	// A step that takes longer than stepBudget at the reference scale fails (migrations.md).
	stepBudget = 10 * time.Second
)

// referenceScaleFiller writes the reference-scale rows through prepared statements in one transaction, at
// the baseline's schema (D228), so every later step migrates real volumes. A fold that moves the
// baseline up moves the filler's names with it.
type referenceScaleFiller struct {
	tb      testing.TB
	ctx     context.Context //nolint:containedctx // the test's, for the statements it runs
	tx      *sql.Tx
	random  *rand.Rand
	nextID  int64
	fileID  int64
	streams int64
	now     int64
}

func (filler *referenceScaleFiller) exec(query string, args ...any) {
	filler.tb.Helper()
	if _, err := filler.tx.ExecContext(filler.ctx, query, args...); err != nil {
		filler.tb.Fatalf("%s: %v", strings.Fields(query)[:3], err)
	}
}

// bytes is n bytes, a multiple of 8, the fill cannot compress away, as stored responses and
// hashes are.
func (filler *referenceScaleFiller) bytes(n int) []byte {
	out := make([]byte, n)
	for index := 0; index+8 <= n; index += 8 {
		binary.LittleEndian.PutUint64(out[index:], filler.random.Uint64())
	}
	return out
}

// words is a synthetic phrase of n words (D187: no real titles in fixtures).
func (filler *referenceScaleFiller) words(n int) string {
	parts := make([]string, n)
	for index := range parts {
		parts[index] = refWords[filler.random.IntN(len(refWords))]
	}
	return strings.Join(parts, " ")
}

var refWords = strings.Fields("harbour lantern quiet orchard copper winter paper signal hollow meridian " +
	"salt violet granite ember tidal northern glass velvet iron amber river station letter garden " +
	"silent autumn falcon island bridge shadow marble crimson canyon echo")

// insertItem inserts an item and its text and sort key, and returns its id.
func (filler *referenceScaleFiller) insertItem(kind string, libraryID int64, listing string) int64 {
	filler.nextID++
	id := filler.nextID
	name := filler.words(1 + filler.random.IntN(4))
	filler.exec("INSERT INTO item (id, kind, library_id, added_at) VALUES (?, ?, ?, ?)", id, kind, libraryID,
		filler.now-filler.random.Int64N(365*24*3600*1000))
	filler.exec("INSERT INTO item_text (item_id, language, is_original, name, overview, tagline) "+
		"VALUES (?, 'und', 1, ?, ?, ?)",
		id, name, filler.words(60), filler.words(8))
	if listing != "" {
		filler.exec("INSERT INTO sort_key (item_id, locale, listing, collation_key) VALUES (?, 'en', ?, ?)",
			id, listing,
			append([]byte(name), 0, 1, 2, 3))
	}
	return id
}

// insertVideo inserts a title's version, and its file with a probe and four streams.
func (filler *referenceScaleFiller) insertVideo(titleID, libraryID int64, path string) {
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
	filler.exec(`INSERT INTO file (id, library_id, node_id, path, reported_path_raw, size_bytes, modified_at,
		payload_hash, payload_bytes, filing, version_id, first_seen_at, last_seen_at)
		VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?, 'listed', ?, ?, ?)`,
		filler.fileID, libraryID, path, []byte(path), 20_000_000_000, filler.now, filler.bytes(32), 19_000_000_000,
		versionID, filler.now, filler.now)
	filler.exec("INSERT INTO file_probe (file_id, probe_json, duration_ms) VALUES (?, ?, ?)", filler.fileID, probe,
		5_400_000)
	for index, stream := range []struct{ kind, codec, language string }{
		{"video", "hevc", ""}, {"audio", "truehd", "en"}, {"audio", "eac3", "de"}, {"subtitle", "subrip", "en"},
	} {
		filler.streams++
		filler.exec(`INSERT INTO stream (bytes_file, video_file, stream_index, kind, codec, container_language,
			language, channel_count) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			filler.fileID, filler.fileID, index, stream.kind, stream.codec,
			stream.language, stream.language, 8)
	}
}

// insertSidecar inserts an NFO beside the video inserted last, attached to it.
func (filler *referenceScaleFiller) insertSidecar(libraryID int64, path string) {
	videoID := filler.fileID
	filler.fileID++
	filler.exec(`INSERT INTO file (id, library_id, node_id, path, reported_path_raw, size_bytes, modified_at,
		filing, sidecar_kind, attached_to_file, first_seen_at, last_seen_at)
		VALUES (?, ?, 1, ?, ?, 2000, ?, 'attached', 'nfo', ?, ?, ?)`,
		filler.fileID, libraryID, path, []byte(path), filler.now, videoID, filler.now, filler.now)
}

// insertFilm inserts one film: its title, one or two versions, artwork, ids and stored response.
func (filler *referenceScaleFiller) insertFilm(index int) {
	id := filler.insertItem("title", 1, "movie")
	filler.exec(`INSERT INTO title (id, kind, year, runtime_ms, followed_source, followed_source_id,
		identification_state) VALUES (?, 'movie', ?, ?, 'tmdb', ?, 'identified')`,
		id, 1950+filler.random.IntN(76), 6_000_000, fmt.Sprint(index))
	for version := range 1 + boolInt(index%10 == 0) {
		path := fmt.Sprintf("/media/films/Film %d (%d)/Film %d - %d.mkv", index, index, index, version)
		filler.insertVideo(id, 1, path)
	}
	filler.insertSidecar(1, fmt.Sprintf("/media/films/Film %d (%d)/Film %d - 0.nfo", index, index, index))
	filler.insertProviderData(id, "movie", index)
}

// insertProviderData inserts an item's artwork, provider ids and stored provider response.
func (filler *referenceScaleFiller) insertProviderData(itemID int64, kind string, index int) {
	for _, art := range []string{"poster", "backdrop"} {
		filler.exec("INSERT INTO artwork (item_id, kind, source, source_ref, source_sha256) "+
			"VALUES (?, ?, 'tmdb', ?, ?)",
			itemID, art, fmt.Sprintf("/%s%d.jpg", art, index), filler.bytes(32))
	}
	filler.exec("INSERT INTO item_provider_id (item_id, provider, provider_id) VALUES (?, 'tmdb', ?), (?, 'imdb', ?)",
		itemID, fmt.Sprint(index), itemID, fmt.Sprintf("tt%07d", index))
	if kind == "movie" {
		filler.exec("INSERT INTO provider_response (item_id, provider, fetched_at, last_asked_at, body_gzip) "+
			"VALUES (?, 'tmdb', ?, ?, ?)", itemID, filler.now, filler.now, filler.bytes(2_000))
	}
}

// insertSeries inserts one series with its episodes, their places in the aired order and their stills.
func (filler *referenceScaleFiller) insertSeries(index int) {
	id := filler.insertItem("collection", 2, "series")
	filler.exec("INSERT INTO collection (id, kind, membership, created_by) "+
		"VALUES (?, 'series', 'explicit', 'scanner')", id)
	filler.exec("INSERT INTO ordering (collection_id, name, provider) VALUES (?, 'aired', 'tmdb')", id)
	var orderingID int64
	if err := filler.tx.QueryRowContext(filler.ctx, "SELECT last_insert_rowid()").Scan(&orderingID); err != nil {
		filler.tb.Fatal(err)
	}
	filler.insertProviderData(id, "series", index)
	for episode := range refEpisodesPerShow {
		season, number := 1+episode/10, 1+episode%10
		episodeID := filler.insertItem("title", 2, "")
		filler.exec(`INSERT INTO title (id, kind, year, runtime_ms, followed_source, followed_source_id,
			identification_state) VALUES (?, 'episode', ?, ?, 'tmdb', ?, 'identified')`,
			episodeID, 2000+index%26, 2_700_000, fmt.Sprintf("%d-%d", index, episode))
		filler.insertVideo(episodeID, 2, fmt.Sprintf("/media/shows/Show %d/Season %02d/Show %d - S%02dE%02d.mkv", index,
			season, index, season, number))
		sort := float64(season*10000 + number)
		filler.exec("INSERT INTO position (ordering_id, title_id, group_number, number_in_group, sort_order) "+
			"VALUES (?, ?, ?, ?, ?)",
			orderingID, episodeID, season, number, sort)
		filler.exec("INSERT INTO collection_member (collection_id, member_title, position) VALUES (?, ?, ?)", id,
			episodeID, sort)
		filler.exec("INSERT INTO artwork (item_id, kind, source, source_ref, source_sha256) "+
			"VALUES (?, 'still', 'tmdb', ?, ?)",
			episodeID, fmt.Sprintf("/still%d-%d.jpg", index, episode), filler.bytes(32))
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// fillReferenceScale fills a database at the baseline's schema with the reference-scale library.
func fillReferenceScale(t *testing.T, database *Database) {
	t.Helper()
	transaction, err := database.writer.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	random := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // a fixture, the same every run
	filler := &referenceScaleFiller{
		tb: t, ctx: t.Context(), tx: transaction, random: random,
		now: time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC).UnixMilli(),
	}
	filler.exec("INSERT INTO node (id, name, enrolled_at) VALUES (1, 'local', 1)")
	filler.exec(`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
		VALUES (1, 'Films', 'movies', 'nfo_then_tmdb', 'watch', 0, 1), (2, 'Series', 'shows', 'tmdb', 'watch', 0, 1)`)
	filler.exec("INSERT INTO library_root (library_id, node_id, root_path) " +
		"VALUES (1, 1, '/media/films'), (2, 1, '/media/shows')")
	filler.exec("INSERT INTO user (id, name, role_id, created_at) VALUES (1, 'owner', 1, 1), (2, 'partner', 2, 1)")
	filler.exec(`INSERT INTO device (id, user_id, name, platform, added_at, last_signed_in_at) VALUES
		(1, 1, 'Firefox on Linux', 'web', 1, 1), (2, 1, 'Living room TV', 'cast', 1, 1),
		(3, 2, 'Chrome on Android', 'web', 1, 1), (4, 2, 'Safari on iPhone', 'web', 1, 1)`)
	for session := range refSessions {
		var revokedAt any = filler.now - int64(session)*86_400_000
		if session%5 == 0 {
			revokedAt = nil // the live ones
		}
		secret := make([]byte, 32)
		binary.BigEndian.PutUint64(secret, uint64(session)+1)
		filler.exec(`INSERT INTO "grant" (secret_sha256, kind, user_id, scope, issued_at, revoked_at, device_id)
			VALUES (?, 'session', ?, 'user', ?, ?, ?)`,
			secret, 1+session%2, filler.now-int64(session)*86_400_000, revokedAt, deviceOfPerson(session))
	}
	for index := range refFilms {
		filler.insertFilm(index)
	}
	for index := range refSeries {
		filler.insertSeries(index)
	}
	for span := range refPlaySpans {
		fileID := 1 + filler.random.Int64N(filler.fileID)
		start := filler.now - int64(span)*3_600_000
		// A third of what was watched was finished, past 90% of the file's 5,400,000 ms: watched (D370).
		positionMs := 1_800_000
		if span%3 == 0 {
			positionMs = 5_300_000
		}
		filler.exec(`INSERT INTO play_span (user_id, file_id, from_position_ms, to_position_ms, started_at, ended_at,
			device_id) VALUES (?, ?, 0, 1800000, ?, ?, ?)`,
			1+span%2, fileID, start, start+1_800_000, deviceOfPerson(span))
		filler.exec(`INSERT INTO play_position (user_id, file_id, position_ms, report_seq, updated_at, device_id)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (user_id, file_id) DO UPDATE SET report_seq = excluded.report_seq`,
			1+span%2, fileID, positionMs, span, start, deviceOfPerson(span))
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
}

// deviceOfPerson is one of the two devices of the person 1+n%2 is, in turn: 1 and 2 are the
// owner's, 3 and 4 the partner's.
func deviceOfPerson(n int) int {
	return 1 + n%2*2 + n/2%2
}

// TestReferenceScale fills a database at the baseline's schema with the reference-scale library
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
	t.Logf("reference scale at schema %d: %.1f MB, filled in %v", steps[0].Version, float64(info.Size())/(1<<20),
		wall.Now().Sub(filled))
	if info.Size() < 30<<20 {
		t.Fatalf("the reference-scale database is %d bytes; it should be about 46 MB", info.Size())
	}
	// In order: every step after the baseline, then a rebuild of the files, the table most others
	// reference, as a step's worst case; the plans and pages are read on the result.
	checkStepBudgets(t, database, steps)
	checkStepBudgets(t, database, append(steps, rebuildStep(t, database, "file", lastVersion(steps)+1)))
	if *updateStatistics {
		writeStatistics(t, database)
	} else if fresh := statisticsText(t, database); fresh != persistentStatistics {
		// The committed file is only ever what the generator writes (the owner, 2026-10-26: no
		// hand-edits of generated data); a schema step that changes it is written again here.
		t.Errorf("statistics/persistent.tsv is not what ANALYZE of the reference library writes; write "+
			"it again with go test -tags integration -run TestReferenceScale ./internal/store "+
			"-update-statistics\nfresh:\n%s", fresh)
	}
	checkQueryPlans(t, database)
	checkPageBudgets(t, database)
}

// updateStatistics rewrites statistics/persistent.tsv from what ANALYZE finds of the reference
// library. Never set by default or in CI: the statistics are committed (D303 §3, DL15).
var updateStatistics = flag.Bool("update-statistics", false,
	"rewrite statistics/persistent.tsv from ANALYZE of the reference library")

// writeStatistics runs ANALYZE on a copy of the reference library and writes its sqlite_stat1 as
// the committed statistics, sorted, a sample-free index of each table's rows. A table the
// reference library leaves empty gets none: on one, ANALYZE writes a row for a partial index but
// not for a plain one (invitation, 2026-10-26), which the statistics test refuses as half a table,
// and no row at all is what the planner has of an empty table anyway.
func writeStatistics(t *testing.T, database *Database) {
	t.Helper()
	text := statisticsText(t, database)
	if err := os.WriteFile(filepath.Join("statistics", "persistent.tsv"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote statistics/persistent.tsv: %d bytes", len(text))
}

// statisticsText is what writeStatistics writes: the reference library's statistics, as the file holds them.
func statisticsText(t *testing.T, database *Database) string {
	t.Helper()
	copyPath := filepath.Join(t.TempDir(), "analyzed.db")
	if err := database.BackupInto(t.Context(), copyPath); err != nil {
		t.Fatal(err)
	}
	analyzed, err := sql.Open("sqlite", copyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = analyzed.Close() }() //nolint:errcheck // a scratch copy
	if _, err = analyzed.ExecContext(t.Context(), "ANALYZE"); err != nil {
		t.Fatal(err)
	}
	rows, err := analyzed.QueryContext(t.Context(), "SELECT tbl, coalesce(idx, ''), stat FROM sqlite_stat1 "+
		"WHERE tbl NOT GLOB 'item_fts*' ORDER BY tbl, coalesce(idx, '')")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck // a read
	var found []statisticsRow
	for rows.Next() {
		var row statisticsRow
		if err := rows.Scan(&row.table, &row.index, &row.stat); err != nil {
			t.Fatal(err)
		}
		found = append(found, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	empty := map[string]bool{}
	for _, row := range found {
		if _, known := empty[row.table]; !known {
			var holdsRows bool
			query := `SELECT EXISTS (SELECT 1 FROM "` + strings.ReplaceAll(row.table, `"`, `""`) + `")`
			if err := analyzed.QueryRowContext(t.Context(), query).Scan(&holdsRows); err != nil {
				t.Fatal(err)
			}
			empty[row.table] = !holdsRows
		}
		if !empty[row.table] {
			fmt.Fprintf(&text, "%s\t%s\t%s\n", row.table, row.index, row.stat)
		}
	}
	return text.String()
}

// checkStepBudgets migrates through the pending steps, timing each (migrations.md: under 10 s
// each at the reference scale).
func checkStepBudgets(t *testing.T, database *Database, steps []Step) {
	t.Helper()
	state, err := database.State(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	pending := pendingSteps(steps, state)
	wall := clock.System{}
	var took []time.Duration
	var started time.Time
	options := testMigrateOptions(t.TempDir())
	options.OnBeforeStep = func(int, int) {
		if !started.IsZero() {
			took = append(took, wall.Now().Sub(started))
		}
		started = wall.Now()
	}
	if err := database.migrate(t.Context(), steps, options); err != nil {
		t.Fatal(err)
	}
	if !started.IsZero() {
		took = append(took, wall.Now().Sub(started))
	}
	for index, duration := range took {
		step := pending[index]
		t.Logf("step %04d %s: %v", step.Version, step.Name, duration)
		if duration > stepBudget {
			t.Errorf("step %04d %s took %v at the reference scale; the budget is %v (migrations.md)", step.Version,
				step.Name, duration, stepBudget)
		}
	}
}

// rebuildStep is a step that rebuilds table in the documented pattern (migrations.md): the views and
// the other tables' triggers dropped, a copy made with its rows and id sequence, the old table
// dropped and the copy renamed, and its indexes, the views and every trigger made again. It moves the
// rows and changes no schema, so the plans read after it are the schema's.
func rebuildStep(t *testing.T, database *Database, table string, version int) Step {
	t.Helper()
	schemaSQL := func(query string) []string {
		var texts []string
		for _, row := range queryRows(t, database, query, 1) {
			texts = append(texts, row[0])
		}
		return texts
	}
	copyName := table + "_rebuilt"
	created := schemaSQL("SELECT sql FROM sqlite_schema WHERE type = 'table' AND name = '" + table + "'")
	if len(created) != 1 || !strings.HasPrefix(created[0], "CREATE TABLE "+table+" ") {
		t.Fatalf("the rebuild wants CREATE TABLE %s, and found %q", table, created)
	}
	columns := schemaSQL("SELECT name FROM pragma_table_info('" + table + "')")
	var text strings.Builder
	for _, view := range schemaSQL("SELECT name FROM sqlite_schema WHERE type = 'view' ORDER BY name") {
		fmt.Fprintf(&text, "DROP VIEW %s;\n", view)
	}
	// Another table's trigger may read the table or a view (step 0058's read file through title_found),
	// and a rename checks every trigger: each is dropped, and made again after the views.
	triggers := schemaSQL("SELECT sql FROM sqlite_schema WHERE type = 'trigger' ORDER BY name")
	for _, trigger := range schemaSQL("SELECT name FROM sqlite_schema WHERE type = 'trigger' AND tbl_name <> '" +
		table + "' ORDER BY name") {
		fmt.Fprintf(&text, "DROP TRIGGER %s;\n", trigger)
	}
	fmt.Fprintf(&text, "CREATE TABLE %s %s;\n", copyName, strings.TrimPrefix(created[0], "CREATE TABLE "+table+" "))
	fmt.Fprintf(&text, "INSERT INTO %s (%s) SELECT %[2]s FROM %s;\n", copyName, strings.Join(columns, ", "), table)
	fmt.Fprintf(&text, "UPDATE sqlite_sequence SET seq = (SELECT seq FROM sqlite_sequence WHERE name = '%s') "+
		"WHERE name = '%s';\n", table, copyName)
	fmt.Fprintf(&text, "DROP TABLE %s;\nALTER TABLE %s RENAME TO %[1]s;\n", table, copyName)
	remade := schemaSQL("SELECT sql FROM sqlite_schema WHERE type = 'index' AND tbl_name = '" + table +
		"' AND sql IS NOT NULL ORDER BY name")
	remade = append(remade, schemaSQL("SELECT sql FROM sqlite_schema WHERE type = 'view' ORDER BY name")...)
	remade = append(remade, triggers...)
	for _, statement := range remade {
		fmt.Fprintf(&text, "%s;\n", statement)
	}
	return Step{Version: version, Name: fmt.Sprintf("%04d_rebuild_%s.sql", version, table), SQL: text.String()}
}

// fullScansAllowed are the listing queries that read a whole table, each with why that is right
// at the reference scale. A new full scan fails until it is listed here with a reason, and a listed
// query that no longer scans fails until it is taken off (sql.md section 11).
func fullScansAllowed() map[string]string {
	return map[string]string{
		"ListTitlesByIdentification":    "the admin's needs-a-match list: 1.3 ms at the reference scale",
		"ListLibraries":                 "a household has a handful of libraries",
		"ListRoles":                     "a household has a handful of roles; the admin lists them (D243 section 4)",
		"ListRoleLibraries":             "a role reaches a handful of libraries; the admin lists them (D243 section 4)",
		"ListLiveNodes":                 "a household has a handful of nodes",
		"ListProviderMatchedLibraryIDs": "a household has a handful of libraries; the first-match sweep lists them",
		"ListLiveLibraryIDs":            "a household has a handful of libraries; the identification sweep lists them",
		"ListRefreshRequests": "the refresh sweep reads every request a person made, a handful; it is driven " +
			"from them, never from every item (D303)",
		"ListOtherVideoFileNames": "the key refresh reads every other video's file name with its key, as it reads " +
			"every name, at start and after a pass (D47, D349)",
		"ListLiveInvitationLinks": "a household has a handful of invitations; the admin's people page lists the " +
			"waiting ones' links (D267 section 5)",
		"ListInvitations": "a household has a handful of invitations; the admin's people page lists the waiting " +
			"ones, a used one dropped (D403)",
	}
}

// checkQueryPlans reads every listing query's plan at the reference scale and fails on a full
// scan of a table not allowed with its reason.
func checkQueryPlans(t *testing.T, database *Database) {
	t.Helper()
	allowed := fullScansAllowed()
	for _, query := range listingQueries(t) {
		scanned := fullyScannedTables(t, database, query)
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

// pageOfIDs is the ids query lists, as a JSON array.
func pageOfIDs(t *testing.T, database *Database, query string) string {
	t.Helper()
	var ids []string
	for _, row := range queryRows(t, database, query, 1) {
		ids = append(ids, row[0])
	}
	return "[" + strings.Join(ids, ",") + "]"
}

// checkPageBudgets times the first page of each browse list at the reference scale, best of five,
// against the page budget: a query shaped badly enough to read every file for every title took
// seconds here while the dev library showed nothing (audit-2026-09-25).
func checkPageBudgets(t *testing.T, database *Database) {
	t.Helper()
	filmIDs := pageOfIDs(t, database, "SELECT id FROM title WHERE kind = 'movie' ORDER BY id LIMIT 60")
	var named []string
	for _, row := range queryRows(t, database, "SELECT id FROM collection ORDER BY id LIMIT 60", 1) {
		named = append(named, "["+row[0]+",\"aired\"]")
	}
	orderings := "[" + strings.Join(named, ",") + "]"
	queries := sqlcgen.New(database.readers)
	ctx := t.Context()
	pages := map[string]func() error{
		"films by name": func() error {
			first := sqlcgen.ListFilmsByNameParams{
				Locale: "en", AfterKey: []byte{}, MaxRows: 60, IsEveryLibrary: 1, LibraryIdsJSON: "[]",
			}
			_, err := queries.ListFilmsByName(ctx, first)
			return err //nolint:wrapcheck // a test
		},
		// A person's, who reaches the films alone: each film is listed by its versions' coverage of
		// the libraries they reach (D363 section 8).
		"films by name, a person's": func() error {
			first := sqlcgen.ListFilmsByNameParams{
				Locale: "en", AfterKey: []byte{}, MaxRows: 60, IsEveryLibrary: 0, LibraryIdsJSON: "[1]",
			}
			_, err := queries.ListFilmsByName(ctx, first)
			return err //nolint:wrapcheck // a test
		},
		"series newest first, a person's": func() error {
			first := sqlcgen.ListSeriesByAddedParams{
				BeforeFoundAt: 1 << 62, BeforeModifiedAt: 1 << 62, BeforeID: 1 << 62, MaxRows: 60, IsEveryLibrary: 0,
				LibraryIdsJSON: "[2]",
			}
			_, err := queries.ListSeriesByAdded(ctx, first)
			return err //nolint:wrapcheck // a test
		},
		"films newest first": func() error {
			first := sqlcgen.ListFilmsByAddedParams{
				BeforeFoundAt: 1 << 62, BeforeModifiedAt: 1 << 62, BeforeID: 1 << 62, MaxRows: 60, IsEveryLibrary: 1,
				LibraryIdsJSON: "[]",
			}
			_, err := queries.ListFilmsByAdded(ctx, first)
			return err //nolint:wrapcheck // a test
		},
		"series by name": func() error {
			first := sqlcgen.ListSeriesByNameParams{
				Locale: "en", AfterKey: []byte{}, MaxRows: 60, IsEveryLibrary: 1, LibraryIdsJSON: "[]",
			}
			_, err := queries.ListSeriesByName(ctx, first)
			return err //nolint:wrapcheck // a test
		},
		"series newest first": func() error {
			first := sqlcgen.ListSeriesByAddedParams{
				BeforeFoundAt: 1 << 62, BeforeModifiedAt: 1 << 62, BeforeID: 1 << 62, MaxRows: 60, IsEveryLibrary: 1,
				LibraryIdsJSON: "[]",
			}
			_, err := queries.ListSeriesByAdded(ctx, first)
			return err //nolint:wrapcheck // a test
		},
		"a page of films' progress": func() error {
			_, err := queries.ListTitlesPlayPositions(ctx, sqlcgen.ListTitlesPlayPositionsParams{
				TitleIdsJSON: filmIDs, UserID: 1,
			})
			return err //nolint:wrapcheck // a test
		},
		// D370: a page's films' watched marks, read by their keys; D185 section 2 measured the same
		// rollup computed from the plays at 0.84 ms.
		"a page of films' watched marks": func() error {
			_, err := queries.ListTitlesWatched(ctx, sqlcgen.ListTitlesWatchedParams{TitleIdsJSON: filmIDs, UserID: 1})
			return err //nolint:wrapcheck // a test
		},
		// A page of series' episodes with a file, and how many of them the person watched (D370 section 5).
		"a page of series' progress": func() error {
			counted := sqlcgen.CountOrderingMembersParams{
				UserID: 1, OrderingsJSON: orderings, IsEveryLibrary: 1, LibraryIdsJSON: "[]",
			}
			_, err := queries.CountOrderingMembers(ctx, counted)
			return err //nolint:wrapcheck // a test
		},
		"a series' episodes": func() error {
			_, err := queries.ListSeriesEpisodeDetails(ctx, sqlcgen.ListSeriesEpisodeDetailsParams{
				CollectionID: int64(refFilms) + 1, OrderingName: "aired", IsEveryLibrary: 1, LibraryIdsJSON: "[]",
			})
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
