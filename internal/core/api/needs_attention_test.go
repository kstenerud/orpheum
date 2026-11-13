// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
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

// Needs attention lists at most 100 films without sound, and counts them all (D432 §4), the count a
// true total of more films than the files its list reads (filmsWithoutSoundRead, 500).
func TestNeedsAttention_ListsAHundredAndCountsThemAll(t *testing.T) {
	t.Parallel()
	const silentFilms = 520
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelve(t)
	names := make([]string, 0, silentFilms)
	for index := range silentFilms {
		names = append(names, fmt.Sprintf("(%d, 'und', 1, 'Silent %d', 'silent')", 1000+index, index))
	}
	seedTitles(t, harness.databasePath, titleRows{
		firstID: 1000, count: silentFilms, kind: "movie", state: "identified", year: 1950, addedAt: 3,
	})
	library, err := harness.database.Read().GetLibraryForIdentify(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	err = harness.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		for index := range silentFilms {
			id := int64(1000 + index)
			fileID, seedErr := seedListedFile(t.Context(), queries, library.Kind, listedFile{
				workID: id, path: fmt.Sprintf("/m/Silent %d (1950).mp4", index), sizeBytes: 1000,
				payloadHash: []byte{byte(index), 0xf0}, placeIndex: index, versionCreatedAt: 3,
			})
			if seedErr != nil {
				return seedErr
			}
			streamIndex := int64(0)
			seedErr = queries.UpsertStream(t.Context(), sqlcgen.UpsertStreamParams{
				FileID: fileID, StreamIndex: &streamIndex, Kind: "video", Codec: new("h264"),
			})
			if seedErr != nil {
				return fmt.Errorf("stream %d: %w", index, seedErr)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("films: %v", err)
	}
	storetest.ExecRaw(t, harness.databasePath,
		"INSERT INTO item_text (item_id, language, is_original, name, search_alternates) VALUES "+
			strings.Join(names, ", "))
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
	seedEpisodeFiles(t, harness.database, 100, episodes, filesEach)
	got := harness.do(t, http.MethodGet, "/needs-attention", admin, nil)
	unplaced := episodes * filesEach
	want := fmt.Sprintf(`"seriesReadWrongly":[{"seriesId":20,"name":"Brass Arena","unplacedCount":%d}]`, unplaced)
	if !strings.Contains(got.body, want) {
		t.Errorf("GET /needs-attention = %d %.400s; want Brass Arena's %d unplaced files counted", got.status,
			got.body, unplaced)
	}
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

// seedEpisodeFiles gives each of the episodes from firstID a version holding filesEach files, as
// librarytest.File writes each, in one transaction: a transaction per file, or every file in one
// title (title_found's trigger recomputes over a title's files), takes minutes.
func seedEpisodeFiles(t *testing.T, database *store.Database, firstID int64, episodes, filesEach int) {
	t.Helper()
	library, err := database.Read().GetLibraryForIdentify(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	err = database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		for index := range episodes * filesEach {
			_, seedErr := seedListedFile(t.Context(), queries, library.Kind, listedFile{
				workID: firstID + int64(index/filesEach), path: fmt.Sprintf("/m/BB/Brass Arena - part %d.mkv", index),
				sizeBytes: 10, placeIndex: index, versionCreatedAt: 2,
			})
			if seedErr != nil {
				return seedErr
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("files: %v", err)
	}
}

// listedFile is a file seedListedFile writes: at path, in a new version of title workID.
type listedFile struct {
	workID           int64
	path             string
	sizeBytes        int64
	payloadHash      []byte // nil: the path's bytes
	placeIndex       int    // the file's number in its test, for the errors
	versionCreatedAt int64
}

// seedListedFile writes file, read and listed, in a new version of its title, and returns its id.
func seedListedFile(
	ctx context.Context, queries *sqlcgen.Queries, libraryKind string, file listedFile,
) (int64, error) {
	index := file.placeIndex
	payloadHash := file.payloadHash
	if payloadHash == nil {
		payloadHash = []byte(file.path)
	}
	id, err := queries.InsertFile(ctx, sqlcgen.InsertFileParams{
		SizeBytes: &file.sizeBytes, PayloadHash: payloadHash,
	})
	if err != nil {
		return 0, fmt.Errorf("file %d: %w", index, err)
	}
	err = queries.InsertPlace(ctx, sqlcgen.InsertPlaceParams{
		FileID: id, NodeID: 1, Path: file.path, ReportedPathRaw: []byte(file.path), Filing: "read", SeenAt: 1,
	})
	if err != nil {
		return 0, fmt.Errorf("place %d: %w", index, err)
	}
	err = queries.UpsertFileFiling(ctx, sqlcgen.UpsertFileFilingParams{
		FileID: id, LibraryKind: libraryKind, Filing: "listed",
	})
	if err != nil {
		return 0, fmt.Errorf("filing %d: %w", index, err)
	}
	versionID, err := queries.InsertVersion(ctx,
		sqlcgen.InsertVersionParams{WorkID: file.workID, CreatedAt: file.versionCreatedAt})
	if err != nil {
		return 0, fmt.Errorf("version %d: %w", index, err)
	}
	err = queries.SetFileVersionAndIdentifiedPath(ctx, sqlcgen.SetFileVersionAndIdentifiedPathParams{
		LibraryKind: libraryKind, VersionID: &versionID, FileID: id,
	})
	if err != nil {
		return 0, fmt.Errorf("file %d's version: %w", index, err)
	}
	return id, nil
}
