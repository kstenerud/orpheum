// SPDX-License-Identifier: AGPL-3.0-or-later

package librarytest_test

import (
	"slices"
	"strconv"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// TestItem_LetsTheDatabaseChooseOrTakesAChosenID covers both of Item's paths: through the store's
// own writer, and, with an id a test names to link other rows to it, built directly.
func TestItem_LetsTheDatabaseChooseOrTakesAChosenID(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	chosen := librarytest.Item(t, database, librarytest.ItemSpec{ID: 20, Kind: "collection", LibraryID: 0, AddedAt: 5})
	if chosen != 20 {
		t.Fatalf("Item with ID 20 returned %d", chosen)
	}
	got := storetest.QueryRaw(t, database.Path(), "SELECT kind, library_id, added_at FROM item WHERE id = 20")
	if want := []string{"collection|<nil>|5"}; !slices.Equal(got, want) {
		t.Errorf("item 20 = %v, want %v", got, want)
	}

	chosenByDatabase := librarytest.Item(t, database, librarytest.ItemSpec{})
	if chosenByDatabase == 0 || chosenByDatabase == 20 {
		t.Fatalf("Item with no ID returned %d, want a fresh one", chosenByDatabase)
	}
	got = storetest.QueryRaw(t, database.Path(),
		"SELECT kind, library_id, added_at FROM item WHERE id = "+itoa(chosenByDatabase))
	if want := []string{"title|<nil>|1"}; !slices.Equal(got, want) {
		t.Errorf("item %d (defaults) = %v, want %v", chosenByDatabase, got, want)
	}
}

// TestTitle_MakesTheItemAndTheTitleRowTogether covers Title's own columns and that it shares the
// item's id, with both a chosen id and one the database assigns.
func TestTitle_MakesTheItemAndTheTitleRowTogether(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	titleID := librarytest.Title(t, database, librarytest.TitleSpec{
		ID: 10, Kind: "movie", Year: 1995, FollowedSource: "tmdb", FollowedSourceID: "603",
		IdentificationState: "identified",
	})
	if titleID != 10 {
		t.Fatalf("Title with ID 10 returned %d", titleID)
	}
	librarytest.Version(t, database, librarytest.VersionSpec{TitleID: titleID})
	got := storetest.QueryRaw(t, database.Path(),
		"SELECT kind, year, followed_source, followed_source_id, identification_state FROM title WHERE id = 10")
	if want := []string{"movie|1995|tmdb|4801|identified"}; !slices.Equal(got, want) {
		t.Errorf("title 10 = %v, want %v", got, want)
	}

	seriesID := librarytest.Title(t, database, librarytest.TitleSpec{Kind: "episode"})
	librarytest.Version(t, database, librarytest.VersionSpec{TitleID: seriesID})
	got = storetest.QueryRaw(t, database.Path(),
		"SELECT kind, year, followed_source, identification_state FROM title WHERE id = "+itoa(seriesID))
	if want := []string{"episode|<nil>|<nil>|identified"}; !slices.Equal(got, want) {
		t.Errorf("title %d (defaults) = %v, want %v", seriesID, got, want)
	}
}

// TestVersion_LetsTheDatabaseChooseOrTakesAChosenID covers both of Version's paths and that
// RemovedAt tombstones it afterward.
func TestVersion_LetsTheDatabaseChooseOrTakesAChosenID(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	titleID := librarytest.Title(t, database, librarytest.TitleSpec{ID: 10})

	chosen := librarytest.Version(t, database, librarytest.VersionSpec{ID: 20, TitleID: titleID, CreatedAt: 3})
	if chosen != 20 {
		t.Fatalf("Version with ID 20 returned %d", chosen)
	}
	got := storetest.QueryRaw(t, database.Path(), "SELECT title_id, created_at, removed_at FROM version WHERE id = 20")
	if want := []string{"10|3|<nil>"}; !slices.Equal(got, want) {
		t.Errorf("version 20 = %v, want %v", got, want)
	}

	removed := librarytest.Version(t, database, librarytest.VersionSpec{TitleID: titleID, RemovedAt: 9})
	got = storetest.QueryRaw(t, database.Path(),
		"SELECT title_id, created_at, removed_at FROM version WHERE id = "+itoa(removed))
	if want := []string{"10|1|9"}; !slices.Equal(got, want) {
		t.Errorf("removed version %d = %v, want %v", removed, got, want)
	}
}

// TestFile_LetsTheDatabaseChooseOrTakesAChosenID covers File's ordinary path (through the store's
// own writer, with its version and attachment setters) and the chosen-id path a test uses to link
// other rows to it.
func TestFile_LetsTheDatabaseChooseOrTakesAChosenID(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	nodeID, libraryID := librarytest.NodeWithLibrary(t, database,
		librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "nfo", Root: "/media"})
	titleID := librarytest.Title(t, database, librarytest.TitleSpec{Library: libraryID})
	versionID := librarytest.Version(t, database, librarytest.VersionSpec{TitleID: titleID})

	auto := librarytest.File(t, database, librarytest.FileSpec{
		LibraryID: libraryID, NodeID: nodeID, Path: "/media/Metropolis (1995).mkv", SizeBytes: 1000,
		PayloadHash: []byte{0x01}, VersionID: versionID,
	})
	got := storetest.QueryRaw(t, database.Path(),
		"SELECT path, reported_path_raw, size_bytes, payload_hash, filing, version_id, first_seen_at, last_seen_at "+
			"FROM file WHERE id = "+itoa(auto))
	if want := []string{"/media/Metropolis (1995).mkv|2f6d656469612f4d6574726f706f6c6973202831393935292e6d6b76" +
		"|1000|01|listed|" + itoa(versionID) + "|1|1"}; !slices.Equal(got, want) {
		t.Errorf("auto file %d = %v, want %v", auto, got, want)
	}

	chosen := librarytest.File(t, database, librarytest.FileSpec{
		ID: 50, LibraryID: libraryID, NodeID: nodeID, Path: "/media/Locked", Filing: "errored",
		FilingReason: "permission_denied", FirstSeenAt: 7000, LastSeenAt: 9000,
	})
	if chosen != 50 {
		t.Fatalf("File with ID 50 returned %d", chosen)
	}
	got = storetest.QueryRaw(t, database.Path(),
		"SELECT filing, filing_reason, first_seen_at, last_seen_at FROM file WHERE id = 50")
	if want := []string{"errored|permission_denied|7000|9000"}; !slices.Equal(got, want) {
		t.Errorf("chosen file 50 = %v, want %v", got, want)
	}

	sidecar := librarytest.File(t, database, librarytest.FileSpec{
		LibraryID: libraryID, NodeID: nodeID, Path: "/media/Metropolis (1995).en.srt", SizeBytes: 5,
		SidecarKind: "subtitle", AttachedToFile: auto,
	})
	got = storetest.QueryRaw(t, database.Path(),
		"SELECT filing, sidecar_kind, attached_to_file FROM file WHERE id = "+itoa(sidecar))
	if want := []string{"attached|subtitle|" + itoa(auto)}; !slices.Equal(got, want) {
		t.Errorf("sidecar file %d = %v, want %v", sidecar, got, want)
	}
}

// TestProbe_UpsertsTheFilesProbe covers Probe's defaults and that a second call replaces it.
func TestProbe_UpsertsTheFilesProbe(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	nodeID, libraryID := librarytest.NodeWithLibrary(t, database,
		librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "nfo", Root: "/media"})
	fileID := librarytest.File(t, database,
		librarytest.FileSpec{LibraryID: libraryID, NodeID: nodeID, Path: "/media/a.mkv"})

	librarytest.Probe(t, database, librarytest.ProbeSpec{FileID: fileID})
	got := storetest.QueryRaw(t, database.Path(),
		"SELECT probe_json, duration_ms FROM file_probe WHERE file_id = "+itoa(fileID))
	if want := []string{"{}|0"}; !slices.Equal(got, want) {
		t.Errorf("default probe of %d = %v, want %v", fileID, got, want)
	}

	librarytest.Probe(t, database, librarytest.ProbeSpec{FileID: fileID, ProbeJSON: `{"format":{}}`, DurationMs: 1000})
	got = storetest.QueryRaw(t, database.Path(),
		"SELECT probe_json, duration_ms FROM file_probe WHERE file_id = "+itoa(fileID))
	if want := []string{`{"format":{}}|1000`}; !slices.Equal(got, want) {
		t.Errorf("replaced probe of %d = %v, want %v", fileID, got, want)
	}
}

// TestStream_CoversItsOwnTrackAndASidecars covers all three of Stream's paths: a video's own
// track with a chosen id, one the database assigns, and a sidecar's track on the video it serves.
func TestStream_CoversItsOwnTrackAndASidecars(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	nodeID, libraryID := librarytest.NodeWithLibrary(t, database,
		librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "nfo", Root: "/media"})
	video := librarytest.File(t, database,
		librarytest.FileSpec{LibraryID: libraryID, NodeID: nodeID, Path: "/media/a.mkv"})
	subtitleFile := librarytest.File(t, database, librarytest.FileSpec{
		LibraryID: libraryID, NodeID: nodeID, Path: "/media/a.en.srt", SidecarKind: "subtitle", AttachedToFile: video,
	})

	chosen := librarytest.Stream(t, database, librarytest.StreamSpec{
		ID: 5, BytesFile: video, StreamIndex: 0, Kind: "video", Codec: "h264",
	})
	if chosen != 5 {
		t.Fatalf("Stream with ID 5 returned %d", chosen)
	}
	got := storetest.QueryRaw(t, database.Path(),
		"SELECT bytes_file, video_file, stream_index, kind, codec FROM stream WHERE id = 5")
	if want := []string{itoa(video) + "|" + itoa(video) + "|0|video|h264"}; !slices.Equal(got, want) {
		t.Errorf("stream 5 = %v, want %v", got, want)
	}

	auto := librarytest.Stream(t, database, librarytest.StreamSpec{
		BytesFile: video, StreamIndex: 1, Kind: "audio", Codec: "aac", Language: "en", ChannelCount: 2,
	})
	got = storetest.QueryRaw(t, database.Path(),
		"SELECT bytes_file, video_file, stream_index, kind, codec, language, channel_count FROM stream WHERE id = "+
			itoa(auto))
	if want := []string{itoa(video) + "|" + itoa(video) + "|1|audio|aac|en|2"}; !slices.Equal(got, want) {
		t.Errorf("auto stream %d = %v, want %v", auto, got, want)
	}

	sidecarStream := librarytest.Stream(t, database, librarytest.StreamSpec{
		BytesFile: subtitleFile, VideoFile: video, StreamIndex: 0, Kind: "subtitle", Codec: "subrip", Language: "en",
	})
	got = storetest.QueryRaw(t, database.Path(),
		"SELECT bytes_file, video_file, stream_index, kind, codec, language FROM stream WHERE id = "+
			itoa(sidecarStream))
	if want := []string{itoa(subtitleFile) + "|" + itoa(video) + "|0|subtitle|subrip|en"}; !slices.Equal(got, want) {
		t.Errorf("sidecar stream %d = %v, want %v", sidecarStream, got, want)
	}
}

// itoa is strconv.FormatInt, spelled short for the queries above.
func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}
