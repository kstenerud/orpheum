// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/core/unexpected"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// The admin's needs-attention counts are what is offline, folder and disk apart, and nothing while
// nothing is (D280); a person who is not an admin is refused.
func TestNeedsAttention_CountsTheFoldersAndDisksOffline(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	counts := func() api.NeedsAttentionOutput {
		t.Helper()
		got := harness.do(t, http.MethodGet, "/needs-attention", admin, nil)
		var output api.NeedsAttentionOutput
		if err := json.Unmarshal([]byte(got.body), &output.Body); err != nil || got.status != http.StatusOK {
			t.Fatalf("GET /needs-attention = %d %s", got.status, got.body)
		}
		return output
	}
	if nothing := counts().Body; nothing.RootCount+nothing.DiskCount+nothing.WaitingRootCount+nothing.UnreadCount != 0 {
		t.Errorf("with nothing offline: %+v, want nothing counted", nothing)
	}
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at)
		 VALUES (1, '/media', 'unreadable', 9000)`,
		`INSERT INTO unavailable_place (node_id, path, root_path, since, cause)
		 VALUES (1, '/media', '/media', 5000, 'unread')`,
		`INSERT INTO root_mount (node_id, root_path, mount_path, filesystem_device, is_present)
		 VALUES (1, '/media', '/media/disk1', 21, 0)`,
		`INSERT INTO unavailable_place (node_id, path, root_path, since, cause)
		 VALUES (1, '/media/disk1', '/media', 3000, 'missing')`)
	if offline := counts().Body; offline.RootCount != 1 || offline.DiskCount != 1 {
		t.Errorf("counts = %+v, want one folder and one disk offline", offline)
	}
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	person := harness.signIn(t, "partner", "partner-password")
	if got := harness.do(t, http.MethodGet, "/needs-attention", person, nil); got.status != http.StatusForbidden {
		t.Errorf("GET /needs-attention by a person who is not an admin = %d, want 403", got.status)
	}
}

// The owner, 2026-10-13: "Needs a match should show up as one of the things under the exclamation
// point tray icon." A film waiting for a person's choice is counted.
func TestNeedsAttention_CountsWhatWaitsForAMatch(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.ambiguousTitle(t)
	got := harness.do(t, http.MethodGet, "/needs-attention", admin, nil)
	var output api.NeedsAttentionOutput
	if err := json.Unmarshal([]byte(got.body), &output.Body); err != nil || got.status != http.StatusOK {
		t.Fatalf("GET /needs-attention = %d %s", got.status, got.body)
	}
	if output.Body.MatchCount != 1 {
		t.Errorf("counts = %+v; want the one film waiting for its match", output.Body)
	}
}

// A series some of whose episodes have no number outside a specials or extras folder is told, its
// files' names likely read wrongly (D306 §6).
func TestNeedsAttention_TellsTheSeriesWhoseEpisodesWereLikelyReadWrongly(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	var statements []string
	for index := range 5 {
		id := int64(40 + index)
		librarytest.Title(t, harness.database, librarytest.TitleSpec{
			ID: id, Library: 1, Kind: "episode", IdentificationState: "unidentified", AddedAt: 2,
		})
		librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: id, TitleID: id, CreatedAt: 2})
		librarytest.File(t, harness.database, librarytest.FileSpec{
			ID: id, LibraryID: 1, NodeID: 1, Path: fmt.Sprintf("/m/BB/Brass Arena - 1x%d.mkv", index+2),
			SizeBytes: 10, PayloadHash: []byte{byte(index + 3)}, Filing: "listed", VersionID: id,
		})
		statements = append(statements, storetest.Positions(fmt.Sprintf(
			`(1, %d, 0, NULL, %d)`, id, 20000+index)))
	}
	storetest.ExecRaw(t, harness.databasePath, statements...)
	got := harness.do(t, http.MethodGet, "/needs-attention", admin, nil)
	if !strings.Contains(got.body,
		`"seriesReadWrongly":[{"seriesId":20,"name":"Brass Arena","unplacedCount":5}],"seriesReadWronglyCount":1`) {
		t.Errorf("GET /needs-attention = %d %s; want Brass Arena's five unplaced episodes told", got.status, got.body)
	}
}

// The owner, 2026-11-02: "videos without sound should be included in the list of problem films".
// A film whose read file holds a picture and no sound track is listed by name; one with sound is
// not.
func TestNeedsAttention_ListsTheFilmsWhoseFileHasNoSound(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	librarytest.Title(t, harness.database, librarytest.TitleSpec{ID: 11, Library: 1, Year: 1950, AddedAt: 3})
	librarytest.Version(t, harness.database, librarytest.VersionSpec{ID: 3, TitleID: 11, CreatedAt: 3})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 3, LibraryID: 1, NodeID: 1, Path: "/m/Mister 517 (1950).mp4", SizeBytes: 1000, PayloadHash: []byte{0x03},
		Filing: "listed", VersionID: 3,
	})
	librarytest.Stream(t, harness.database,
		librarytest.StreamSpec{BytesFile: 3, StreamIndex: 0, Kind: "video", Codec: "h264"})
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO item_text (item_id, language, is_original, name, overview, search_alternates)
		 VALUES (11, 'und', 1, 'Mister 517', NULL, 'mister 517')`)
	got := harness.do(t, http.MethodGet, "/needs-attention", admin, nil)
	if !strings.Contains(got.body, `"filmsWithoutSound":[{"itemId":11,"name":"Mister 517"}]`) {
		t.Errorf("GET /needs-attention = %d %s; want only Mister 517 listed as without sound", got.status, got.body)
	}
}

// The owner, 2026-11-13: a rescan that gave up, its library's files changing every time they were
// read, puts its library on Needs attention, named, with when it gave up; nothing is listed while no
// rescan has given up. That a later finished rescan takes it away is identify's
// (TestRescan_ALibraryThatNeverStopsChangingGivesUpWithTheWriteThatChangesIt).
func TestNeedsAttention_ListsTheLibrariesWhoseRescanGaveUp(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	if before := harness.do(t, http.MethodGet, "/needs-attention", admin, nil); before.status != http.StatusOK ||
		strings.Contains(before.body, `"rescansGivenUp"`) {
		t.Fatalf("GET /needs-attention with no rescan given up = %d %s; want none listed", before.status,
			before.body)
	}
	err := harness.cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		return queries.UpsertRescanGivenUp(t.Context(), cachegen.UpsertRescanGivenUpParams{
			LibraryID: 1, GivenUpAt: 7000,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	got := harness.do(t, http.MethodGet, "/needs-attention", admin, nil)
	if !strings.Contains(got.body, `"rescansGivenUp":[{"libraryId":1,"name":"Films","givenUpAt":7000}]`) {
		t.Errorf("GET /needs-attention = %d %s; want library 1, Films, listed as given up at 7000", got.status,
			got.body)
	}
}

// Needs attention lists at most 100 films without sound, and counts them all (D432 §4), the count a
// true total of more films than the files its list reads (filmsWithoutSoundRead, 500).
func TestNeedsAttention_ListsAHundredAndCountsThemAll(t *testing.T) {
	t.Parallel()
	const silentFilms = 520
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	seedSoundlessFilms(t, harness, 1000, silentFilms)
	var got api.NeedsAttentionOutput
	harness.readJSON(t, "/needs-attention", admin, &got.Body)
	if len(got.Body.FilmsWithoutSound) != 100 || got.Body.FilmsWithoutSoundCount != silentFilms {
		t.Errorf("GET /needs-attention listed %d films without sound and counted %d; want 100 and %d",
			len(got.Body.FilmsWithoutSound), got.Body.FilmsWithoutSoundCount, silentFilms)
	}
}

// A series' unplaced episodes' files are counted in full however many there are (D432 §4): the read
// of unnumbered episodes' files once stopped at 5000 rows, under-reporting a series with more. The count
// is of files.
func TestNeedsAttention_CountsASeriesUnplacedEpisodesPastFiveThousand(t *testing.T) {
	t.Parallel()
	const episodes, filesEach = 1001, 5
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	places := make([]string, 0, episodes)
	for index := range episodes {
		places = append(places, fmt.Sprintf(`(1, %d, 0, NULL, %d)`, 100+index, 20000+index))
	}
	seedTitles(t, harness.databasePath, titleRows{
		firstID: 100, count: episodes, kind: "episode", state: "unidentified", addedAt: 2,
	})
	storetest.ExecRaw(t, harness.databasePath, storetest.Positions(strings.Join(places, ", ")))
	seedEpisodeFiles(t, harness, 100, episodes, filesEach)
	got := harness.do(t, http.MethodGet, "/needs-attention", admin, nil)
	unplaced := episodes * filesEach
	want := fmt.Sprintf(`"seriesReadWrongly":[{"seriesId":20,"name":"Brass Arena","unplacedCount":%d}]`, unplaced)
	if !strings.Contains(got.body, want) {
		t.Errorf("GET /needs-attention = %d %.400s; want Brass Arena's %d unplaced files counted", got.status,
			got.body, unplaced)
	}
}

// seedSoundlessFilms writes count films in library 1, ids from firstID, each named "Silent <n>"
// from 1950 with one listed file whose one stream is its video: no sound.
func seedSoundlessFilms(t *testing.T, harness testHarness, firstID int64, count int) {
	t.Helper()
	names := make([]string, 0, count)
	files := make([]listedFile, 0, count)
	for index := range count {
		id := firstID + int64(index)
		names = append(names, fmt.Sprintf("(%d, 'und', 1, 'Silent %d', 'silent')", id, index))
		files = append(files, listedFile{
			workID: id, path: fmt.Sprintf("/m/Silent %d (1950).mp4", index), sizeBytes: 1000,
			payloadHash: []byte{byte(index), 0xf0}, versionCreatedAt: 3, videoCodec: "h264",
		})
	}
	seedTitles(t, harness.databasePath, titleRows{
		firstID: firstID, count: int64(count), kind: "movie", state: "identified", year: 1950, addedAt: 3,
	})
	seedListedFiles(t, harness, files)
	storetest.ExecRaw(t, harness.databasePath,
		"INSERT INTO item_text (item_id, language, is_original, name, search_alternates) VALUES "+
			strings.Join(names, ", "))
}

// titleRows is count titles of one kind in library 1 (an item and its work each), numbered from
// firstID; year 0 is none.
type titleRows struct {
	firstID, count int64
	kind, state    string
	year, addedAt  int64
}

// seedTitles writes the titles as librarytest.Title writes one, the items in one statement and the
// works in another: a transaction or two per title, a thousand of them, took most of a minute.
func seedTitles(t *testing.T, databasePath string, rows titleRows) {
	t.Helper()
	year := "NULL"
	if rows.year != 0 {
		year = fmt.Sprint(rows.year)
	}
	items := make([]string, 0, rows.count)
	works := make([]string, 0, rows.count)
	for id := rows.firstID; id < rows.firstID+rows.count; id++ {
		items = append(items, fmt.Sprintf("(%d, '%s', 1, %d, %s, '%s')", id, rows.kind, rows.addedAt, year, rows.state))
		works = append(works, fmt.Sprintf("(%d, '%s')", id, rows.kind))
	}
	storetest.ExecRaw(t, databasePath,
		"INSERT INTO item (id, kind, filing_library_id, added_at, year, identification_state) VALUES "+
			strings.Join(items, ", "),
		"INSERT INTO work (id, kind) VALUES "+strings.Join(works, ", "))
}

// seedEpisodeFiles gives each of the episodes from firstID filesEach files, each in a version of its
// own. Every file in one title would take minutes: title_found's trigger recomputes over a title's
// files.
func seedEpisodeFiles(t *testing.T, harness testHarness, firstID int64, episodes, filesEach int) {
	t.Helper()
	files := make([]listedFile, 0, episodes*filesEach)
	for index := range episodes * filesEach {
		files = append(files, listedFile{
			workID: firstID + int64(index/filesEach), path: fmt.Sprintf("/m/BB/Brass Arena - part %d.mkv", index),
			sizeBytes: 10, versionCreatedAt: 2,
		})
	}
	seedListedFiles(t, harness, files)
}

// listedFile is a file seedListedFiles writes: at path, in a new version of title workID, with one
// video stream of videoCodec unless that is empty.
type listedFile struct {
	workID           int64
	path             string
	sizeBytes        int64
	payloadHash      []byte // nil: the path's bytes
	versionCreatedAt int64
	videoCodec       string
}

// seedListedFiles writes files in library 1, each read and listed in a new version of its title,
// with the rows and in the order the store's writers leave them (InsertFile, InsertPlace,
// UpsertFileFiling, InsertVersion, SetFileVersionAndIdentifiedPath, UpsertStream), at the ids they
// would take, but in one statement per table for all of them: SQLite compiles a statement, its
// triggers' programs included, each time it is prepared, and the writers' statements once a file
// spent 16 of a test's 18 s compiling for 5005 files (test-weight.md §34).
func seedListedFiles(t *testing.T, harness testHarness, files []listedFile) {
	t.Helper()
	library, err := harness.database.Read().GetLibraryForIdentify(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	firstFile, firstVersion := nextID(t, harness.databasePath, "file"), nextID(t, harness.databasePath, "version")
	var fileRows, placeRows, filingRows, versionRows, streamRows []string
	for index, file := range files {
		fileID, versionID := firstFile+int64(index), firstVersion+int64(index)
		payloadHash := file.payloadHash
		if payloadHash == nil {
			payloadHash = []byte(file.path)
		}
		path := "'" + strings.ReplaceAll(file.path, "'", "''") + "'"
		fileRows = append(fileRows, fmt.Sprintf("(%d, %d, X'%x')", fileID, file.sizeBytes, payloadHash))
		placeRows = append(placeRows,
			fmt.Sprintf("(%d, %d, 1, %s, X'%x', 'read', 1, 1)", fileID, fileID, path, file.path))
		filingRows = append(filingRows, fmt.Sprintf("(%d, '%s', 'listed')", fileID, library.Kind))
		versionRows = append(versionRows, fmt.Sprintf("(%d, %d, %d)", versionID, file.workID, file.versionCreatedAt))
		if file.videoCodec != "" {
			streamRows = append(streamRows, fmt.Sprintf("(%d, %d, 0, 'video', '%s')", fileID, fileID, file.videoCodec))
		}
	}
	statements := []string{
		"INSERT INTO file (id, size_bytes, payload_hash) VALUES " + strings.Join(fileRows, ", "),
		`INSERT INTO place (id, file_id, node_id, path, reported_path_raw, filing, first_seen_at, last_seen_at)
		 VALUES ` + strings.Join(placeRows, ", "),
		"INSERT INTO file_filing (file_id, library_kind, filing) VALUES " + strings.Join(filingRows, ", "),
		"INSERT INTO version (id, work_id, created_at) VALUES " + strings.Join(versionRows, ", "),
		// SetFileVersionAndIdentifiedPath's statement, over every file at once.
		fmt.Sprintf(`INSERT INTO file_filing (file_id, library_kind, filing, version_id, identified_from_path)
		 SELECT place.id, '%s', 'unidentified', place.id + %d, place.path FROM place WHERE place.id BETWEEN %d AND %d
		 ON CONFLICT (file_id, library_kind) DO UPDATE SET version_id = excluded.version_id,
		   identified_from_path = excluded.identified_from_path
		 WHERE (file_filing.version_id, file_filing.identified_from_path)
		   IS NOT (excluded.version_id, excluded.identified_from_path)`,
			library.Kind, firstVersion-firstFile, firstFile, firstFile+int64(len(files))-1),
	}
	if len(streamRows) > 0 {
		statements = append(statements,
			"INSERT INTO stream (bytes_file, video_file, stream_index, kind, codec) VALUES "+
				strings.Join(streamRows, ", "))
	}
	storetest.ExecRaw(t, harness.databasePath, statements...)
	// One statement for every file fails for none of them alone: each file is checked written.
	lastFile := firstFile + int64(len(files)) - 1
	versioned := storetest.QueryRaw(t, harness.databasePath, fmt.Sprintf(
		`SELECT count(*) FROM file_filing JOIN place ON place.id = file_filing.file_id
		 WHERE file_filing.file_id BETWEEN %d AND %d AND file_filing.filing = 'listed'
		   AND file_filing.version_id = file_filing.file_id + %d AND identified_from_path = place.path`,
		firstFile, lastFile, firstVersion-firstFile))
	if want := fmt.Sprint(len(files)); versioned[0] != want {
		t.Fatalf("%s of %s seeded files listed in their own version, from their place's path", versioned[0], want)
	}
	streams := storetest.QueryRaw(t, harness.databasePath, fmt.Sprintf(
		"SELECT count(*) FROM stream WHERE video_file BETWEEN %d AND %d", firstFile, lastFile))
	if want := fmt.Sprint(len(streamRows)); streams[0] != want {
		t.Fatalf("%s of %s seeded files' video streams written", streams[0], want)
	}
}

// nextID is the id the database gives the next row of table, an AUTOINCREMENT table's.
func nextID(t *testing.T, databasePath, table string) int64 {
	t.Helper()
	got := storetest.QueryRaw(t, databasePath, fmt.Sprintf(
		`SELECT max(coalesce((SELECT max(id) FROM %s), 0),
		   coalesce((SELECT seq FROM sqlite_sequence WHERE name = '%s'), 0)) + 1`,
		table, table))
	var id int64
	if _, err := fmt.Sscan(got[0], &id); err != nil {
		t.Fatalf("the next %s id, %q: %v", table, got, err)
	}
	return id
}

// The warning sign lights for failures of unknown cause piling up (D517 §2): needs-attention counts
// the kinds whose alert is raised, none while no kind is over the setting's count in the hour.
func TestNeedsAttention_CountsTheKindsWhoseUnknownFailuresAlertIsRaised(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	settingstest.Set(t, harness.settings, settings.UnknownFailuresAlertPerHour, "1")
	alertCount := func() int {
		t.Helper()
		var got api.NeedsAttentionOutput
		harness.readJSON(t, "/needs-attention", admin, &got.Body)
		return got.Body.UnknownFailureAlertCount
	}
	record := func(subject string) {
		t.Helper()
		err := harness.unknownFailures.Record(t.Context(), unexpected.Failure{
			Kind: "fetch_metadata", Subject: subject, Error: "not JSON", Output: "<html>",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := alertCount(); got != 0 {
		t.Errorf("with no failure of unknown cause: %d alerts counted, want 0", got)
	}
	record("item 1")
	if got := alertCount(); got != 0 {
		t.Errorf("with one failure, not over the setting's 1: %d alerts counted, want 0", got)
	}
	record("item 2")
	if got := alertCount(); got != 1 {
		t.Errorf("with two failures of one kind, over the setting's 1: %d alerts counted, want 1", got)
	}
}
