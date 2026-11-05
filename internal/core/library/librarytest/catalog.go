// SPDX-License-Identifier: AGPL-3.0-or-later

package librarytest

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// This file is the one place the catalog's own tables (item, title, version, file, file_probe,
// stream) are built for a test: a schema rename (D344, D338, D349) edits it, not every test that
// shelves a file (docs/contributors/design/research/test-architecture-2026-10-26.md, finding 6).
//
// Each builder goes through the store's own writer when the row's id can be whatever the database
// assigns: InsertItem, InsertTitle, InsertVersion, InsertFile, InsertSidecarFile, UpsertFileProbe,
// UpsertStream, and the setters a file's version, attachment or removal go through in production
// (SetFileVersionAndIdentifiedPath, SetFileAttachedTo, TombstoneFile, RemoveVersion). A test that
// needs a chosen id, to name a row from elsewhere in its fixture, cannot ask those for one (none
// takes an explicit id for an autoincrement column), so that one case is built directly, through
// storetest.ExecRaw, still in this one place.

// defaultString returns value unless it is empty, when it returns fallback.
func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// defaultInt returns value unless it is zero, when it returns fallback.
func defaultInt(value, fallback int64) int64 {
	if value == 0 {
		return fallback
	}
	return value
}

// sqlString is value as a single-quoted SQL literal.
func sqlString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// sqlNull is the literal a nullable column's zero value (empty string, zero, nil bytes) is built
// with: a test builds no column whose NULL it means to say with its Go zero value instead.
const sqlNull = "NULL"

// sqlNullableString is value as a single-quoted SQL literal, or sqlNull when it is empty.
func sqlNullableString(value string) string {
	if value == "" {
		return sqlNull
	}
	return sqlString(value)
}

// sqlNullableInt is value as a literal, or sqlNull when it is zero.
func sqlNullableInt(value int64) string {
	if value == 0 {
		return sqlNull
	}
	return strconv.FormatInt(value, 10)
}

// sqlBlob is bytes as a SQL blob literal, or sqlNull when it is empty.
func sqlBlob(bytes []byte) string {
	if len(bytes) == 0 {
		return sqlNull
	}
	return fmt.Sprintf("X'%x'", bytes)
}

// ItemSpec is an item row a test builds directly: a series' or a person's, or a title's own,
// which Title makes for it.
type ItemSpec struct {
	ID                  int64  // 0: let the database choose
	Kind                string // movie/series/episode/extra/other/person; defaults to "movie"
	LibraryID           int64  // the library whose pass files it (item.filing_library_id); 0: none
	AddedAt             int64  // defaults to 1
	Year                int64  // 0: NULL
	FollowedSource      string // "": NULL; else nfo/tmdb/tvdb/none
	FollowedSourceID    string // "": NULL
	IdentificationState string // defaults to "unidentified" for a series, "identified" for a title; NULL for a person
}

// Item adds an item row and returns its id.
func Item(t *testing.T, database *store.Database, spec ItemSpec) int64 {
	t.Helper()
	kind := defaultString(spec.Kind, "movie")
	addedAt := defaultInt(spec.AddedAt, 1)
	var state string
	switch kind {
	case "person":
	case "series":
		state = defaultString(spec.IdentificationState, "unidentified")
	default:
		state = defaultString(spec.IdentificationState, "identified")
	}
	if spec.ID != 0 {
		storetest.ExecRaw(t, database.Path(), fmt.Sprintf(
			"INSERT INTO item (id, kind, filing_library_id, added_at, year, followed_source, followed_source_id, "+
				"identification_state) VALUES (%d, %s, %s, %d, %s, %s, %s, %s)",
			spec.ID, sqlString(kind), sqlNullableInt(spec.LibraryID), addedAt, sqlNullableInt(spec.Year),
			sqlNullableString(spec.FollowedSource), sqlNullableString(spec.FollowedSourceID), sqlNullableString(state)))
		return spec.ID
	}
	var id int64
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		var err error
		id, err = queries.InsertItem(t.Context(), sqlcgen.InsertItemParams{
			Kind: kind, LibraryID: ptr.NilIfZero(spec.LibraryID), AddedAt: addedAt, Year: ptr.NilIfZero(spec.Year),
			FollowedSource: ptr.NilIfZero(spec.FollowedSource), FollowedSourceID: ptr.NilIfZero(spec.FollowedSourceID),
			IdentificationState: ptr.NilIfZero(state),
		})
		return err
	})
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	return id
}

// TitleSpec is a title's own columns, beside the item row Title makes to hold them.
type TitleSpec struct {
	ID                  int64  // 0: let the database choose (its item's id)
	Library             int64  // the library whose pass files it; 0: none
	Kind                string // movie/episode/extra/other; defaults to "movie"
	Year                int64  // 0: NULL
	RuntimeMs           int64  // 0: NULL
	FollowedSource      string // "": NULL; else nfo/tmdb/tvdb/none
	FollowedSourceID    string // "": NULL
	IdentificationState string // defaults to "identified"
	AddedAt             int64  // the item's added_at; defaults to 1
}

// Title adds an item of the title's kind and the title row on it, and returns their shared id.
func Title(t *testing.T, database *store.Database, spec TitleSpec) int64 {
	t.Helper()
	kind := defaultString(spec.Kind, "movie")
	itemID := Item(t, database, ItemSpec{
		ID: spec.ID, Kind: kind, LibraryID: spec.Library, AddedAt: spec.AddedAt, Year: spec.Year,
		FollowedSource: spec.FollowedSource, FollowedSourceID: spec.FollowedSourceID,
		IdentificationState: defaultString(spec.IdentificationState, "identified"),
	})
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.InsertTitle(t.Context(), sqlcgen.InsertTitleParams{
			ID: itemID, Kind: kind, RuntimeMs: ptr.NilIfZero(spec.RuntimeMs),
		})
	})
	if err != nil {
		t.Fatalf("title: %v", err)
	}
	return itemID
}

// VersionSpec is a version row a test builds directly.
type VersionSpec struct {
	ID        int64 // 0: let the database choose
	TitleID   int64 // required
	CreatedAt int64 // defaults to 1
	RemovedAt int64 // 0: none
}

// Version adds a version row and returns its id.
func Version(t *testing.T, database *store.Database, spec VersionSpec) int64 {
	t.Helper()
	createdAt := defaultInt(spec.CreatedAt, 1)
	var id int64
	if spec.ID != 0 {
		storetest.ExecRaw(t, database.Path(), fmt.Sprintf(
			"INSERT INTO version (id, title_id, created_at) VALUES (%d, %d, %d)", spec.ID, spec.TitleID, createdAt))
		id = spec.ID
	} else {
		err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
			var err error
			id, err = queries.InsertVersion(t.Context(), sqlcgen.InsertVersionParams{
				TitleID: spec.TitleID, CreatedAt: createdAt,
			})
			return err
		})
		if err != nil {
			t.Fatalf("version: %v", err)
		}
	}
	if spec.RemovedAt != 0 {
		removedAt := spec.RemovedAt
		err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
			return queries.RemoveVersion(t.Context(), sqlcgen.RemoveVersionParams{RemovedAt: &removedAt, ID: id})
		})
		if err != nil {
			t.Fatalf("version removed_at: %v", err)
		}
	}
	return id
}

// FileSpec is a file row a test builds directly: a video or audio file a scan listed, or a
// sidecar (SidecarKind set) attached to one, at a place (D363 §2).
type FileSpec struct {
	ID int64 // 0: let the database choose
	// LibraryID is the library the test means it in: its kind files it, and one of its folders on
	// NodeID must cover Path, which File checks, so the file is not left in no library unseen.
	// 0: a file no library files, which no folder need cover.
	LibraryID       int64
	NodeID          int64  // required
	Path            string // required
	ReportedPathRaw []byte // nil: path's own bytes, as a scan of an NFC name would report it
	SizeBytes       int64  // 0: NULL
	ModifiedAt      int64  // 0: NULL
	PayloadHash     []byte // nil: NULL
	PayloadBytes    int64  // 0: NULL
	// Filing defaults to "listed": listed, extra or unidentified is a video as LibraryID's kind files
	// it (file_filing); video, copy, skipped or errored the scan's own filing alone. Ignored when
	// SidecarKind is set (always "attached").
	Filing            string
	FilingReason      string // "": NULL
	ErrorDetailJSON   string // "": NULL
	VersionID         int64  // 0: none; the version LibraryID's kind files it in
	CopyOf            int64  // 0: none; the file it is a copy of, at another place (Filing "copy")
	AttachedToFile    int64  // 0: none
	SidecarKind       string // "": an ordinary file; else nfo/subtitle/image
	FirstSeenAt       int64  // defaults to 1
	LastSeenAt        int64  // defaults to FirstSeenAt; a different value needs an explicit ID (below)
	RemovedAt         int64  // 0: none
	OpensubtitlesHash string // "": NULL
	FilesystemDevice  int64  // 0: NULL
	Inode             int64  // 0: NULL
}

// File adds a file row, through the store's own writer unless ID chooses one no writer takes
// (fileWithChosenID), and returns its id.
func File(t *testing.T, database *store.Database, spec FileSpec) int64 {
	t.Helper()
	libraryKind := requireCovered(t, database, spec)
	var id int64
	switch {
	case spec.ID != 0:
		id = fileWithChosenID(t, database, spec)
	case spec.SidecarKind != "":
		id = insertSidecarFile(t, database, spec)
	default:
		id = insertOrdinaryFile(t, database, spec)
	}
	applyFileFollowUps(t, database, spec, id, libraryKind)
	return id
}

// isIdentificationFiling is whether a filing is one a kind of library's identification gives a video,
// kept in file_filing; any other is the scan's own, on the file's row.
func isIdentificationFiling(filing string) bool {
	return filing == "listed" || filing == "extra" || filing == "unidentified"
}

// rowFiling is the filing a file's own row takes: a video for one a kind of library files, and for a
// copy made through the writer, which makes a video a copy (MakeFileACopy).
func rowFiling(spec FileSpec) string {
	filing := defaultString(spec.Filing, "listed")
	if isIdentificationFiling(filing) || (filing == "copy" && spec.ID == 0) {
		return "video"
	}
	return filing
}

// requireCovered fails the test when spec names a library none of whose folders on its node covers
// its path, and returns that library's kind: a file is in the libraries that cover its place (D363
// §4), so one left in none would be in no list a test reads.
func requireCovered(t *testing.T, database *store.Database, spec FileSpec) string {
	t.Helper()
	if spec.LibraryID == 0 {
		return "movie"
	}
	var kind string
	var roots []sqlcgen.ListLibraryRootsRow
	err := database.ReadTx(t.Context(), func(queries *sqlcgen.Queries) error {
		library, err := queries.GetLibraryForIdentify(t.Context(), spec.LibraryID)
		if err != nil {
			return err
		}
		kind = library.Kind
		roots, err = queries.ListLibraryRoots(t.Context(), spec.LibraryID)
		return err
	})
	if err != nil {
		t.Fatalf("read library %d: %v", spec.LibraryID, err)
	}
	for _, root := range roots {
		prefix := strings.TrimSuffix(root.RootPath, "/") + "/"
		if root.NodeID == spec.NodeID && strings.HasPrefix(string(fileReportedPathRaw(spec)), prefix) {
			return kind
		}
	}
	t.Fatalf("file %s on node %d is under no folder of library %d (%+v): a file is in the libraries "+
		"that cover its place (D363)", spec.Path, spec.NodeID, spec.LibraryID, roots)
	return kind
}

// fileWithChosenID builds a file row directly: a chosen id, or first_seen_at and last_seen_at
// apart (which no writer below produces: a rescan's UpdateObservedFile also revives an errored or
// skipped filing, which this file may mean to keep), has no production path to go through.
func fileWithChosenID(t *testing.T, database *store.Database, spec FileSpec) int64 {
	t.Helper()
	reportedPathRaw := fileReportedPathRaw(spec)
	filing := rowFiling(spec)
	if spec.SidecarKind != "" {
		filing = "attached"
	}
	firstSeenAt := defaultInt(spec.FirstSeenAt, 1)
	lastSeenAt := defaultInt(spec.LastSeenAt, firstSeenAt)
	storetest.ExecRaw(t, database.Path(), fmt.Sprintf(
		`INSERT INTO file (id, node_id, path, reported_path_raw, size_bytes, modified_at,
		 payload_hash, payload_bytes, filing, filing_reason, error_detail_json, copy_of,
		 sidecar_kind, first_seen_at, last_seen_at, opensubtitles_hash, filesystem_device, inode)
		 VALUES (%d, %d, %s, %s, %s, %s, %s, %s, %s, %s,
		         %s, %s, %s, %d, %d, %s, %s, %s)`,
		spec.ID, spec.NodeID, sqlString(spec.Path), sqlBlob(reportedPathRaw),
		sqlNullableInt(spec.SizeBytes), sqlNullableInt(spec.ModifiedAt), sqlBlob(spec.PayloadHash),
		sqlNullableInt(spec.PayloadBytes), sqlString(filing), sqlNullableString(spec.FilingReason),
		sqlNullableString(spec.ErrorDetailJSON), sqlNullableInt(spec.CopyOf),
		sqlNullableString(spec.SidecarKind), firstSeenAt, lastSeenAt,
		sqlNullableString(spec.OpensubtitlesHash), sqlNullableInt(spec.FilesystemDevice), sqlNullableInt(spec.Inode)))
	return spec.ID
}

// fileReportedPathRaw is spec's reported_path_raw, or, unset, the path's own bytes, as a scan of
// an NFC name would report it.
func fileReportedPathRaw(spec FileSpec) []byte {
	if spec.ReportedPathRaw != nil {
		return spec.ReportedPathRaw
	}
	return []byte(spec.Path)
}

// insertSidecarFile is File's path for a sidecar (SidecarKind set), through InsertSidecarFile,
// which always files it attached.
func insertSidecarFile(t *testing.T, database *store.Database, spec FileSpec) int64 {
	t.Helper()
	var sizeBytes, modifiedAt, payloadBytes *int64
	if spec.SizeBytes != 0 {
		sizeBytes = &spec.SizeBytes
	}
	if spec.ModifiedAt != 0 {
		modifiedAt = &spec.ModifiedAt
	}
	if spec.PayloadBytes != 0 {
		payloadBytes = &spec.PayloadBytes
	}
	var filingReason *string
	if spec.FilingReason != "" {
		filingReason = &spec.FilingReason
	}
	sidecarKind := spec.SidecarKind
	var id int64
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		var err error
		id, err = queries.InsertSidecarFile(t.Context(), sqlcgen.InsertSidecarFileParams{
			NodeID: spec.NodeID, Path: spec.Path,
			ReportedPathRaw: fileReportedPathRaw(spec), SizeBytes: sizeBytes, ModifiedAt: modifiedAt,
			PayloadHash: spec.PayloadHash, PayloadBytes: payloadBytes,
			FilingReason: filingReason, SidecarKind: &sidecarKind, SeenAt: defaultInt(spec.FirstSeenAt, 1),
		})
		return err
	})
	if err != nil {
		t.Fatalf("sidecar file: %v", err)
	}
	return id
}

// insertOrdinaryFile is File's path for everything SidecarKind does not claim, through InsertFile.
func insertOrdinaryFile(t *testing.T, database *store.Database, spec FileSpec) int64 {
	t.Helper()
	var sizeBytes, modifiedAt, payloadBytes, filesystemDevice, inode *int64
	if spec.SizeBytes != 0 {
		sizeBytes = &spec.SizeBytes
	}
	if spec.ModifiedAt != 0 {
		modifiedAt = &spec.ModifiedAt
	}
	if spec.PayloadBytes != 0 {
		payloadBytes = &spec.PayloadBytes
	}
	if spec.FilesystemDevice != 0 {
		filesystemDevice = &spec.FilesystemDevice
	}
	if spec.Inode != 0 {
		inode = &spec.Inode
	}
	var filingReason, errorDetailJSON, opensubtitlesHash *string
	if spec.FilingReason != "" {
		filingReason = &spec.FilingReason
	}
	if spec.ErrorDetailJSON != "" {
		errorDetailJSON = &spec.ErrorDetailJSON
	}
	if spec.OpensubtitlesHash != "" {
		opensubtitlesHash = &spec.OpensubtitlesHash
	}
	var id int64
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		var err error
		id, err = queries.InsertFile(t.Context(), sqlcgen.InsertFileParams{
			NodeID: spec.NodeID, Path: spec.Path,
			ReportedPathRaw: fileReportedPathRaw(spec), SizeBytes: sizeBytes, ModifiedAt: modifiedAt,
			PayloadHash: spec.PayloadHash, PayloadBytes: payloadBytes,
			Filing: rowFiling(spec), FilingReason: filingReason, ErrorDetailJSON: errorDetailJSON,
			SeenAt: defaultInt(spec.FirstSeenAt, 1), OpensubtitlesHash: opensubtitlesHash,
			FilesystemDevice: filesystemDevice, Inode: inode,
		})
		return err
	})
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	return id
}

// applyFileFollowUps sets id's filing of libraryKind and its version, its copy, its attachment and
// its removal, each through the setter production uses, when spec asks for one.
func applyFileFollowUps(t *testing.T, database *store.Database, spec FileSpec, id int64, libraryKind string) {
	t.Helper()
	if isIdentificationFiling(defaultString(spec.Filing, "listed")) && spec.SidecarKind == "" {
		applyFileFiling(t, database, spec, id, libraryKind)
	}
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return applyPlaceFollowUps(t, queries, spec, id)
	})
	if err != nil {
		t.Fatalf("file %d: %v", id, err)
	}
}

// applyFileFiling sets id's filing by libraryKind, and its version when spec names one.
func applyFileFiling(t *testing.T, database *store.Database, spec FileSpec, id int64, libraryKind string) {
	t.Helper()
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		var reason *string
		if spec.FilingReason != "" {
			reason = &spec.FilingReason
		}
		upsertErr := queries.UpsertFileFiling(t.Context(), sqlcgen.UpsertFileFilingParams{
			FileID: id, LibraryKind: libraryKind, Filing: defaultString(spec.Filing, "listed"), FilingReason: reason,
		})
		if upsertErr != nil || spec.VersionID == 0 {
			return upsertErr
		}
		versionID := spec.VersionID
		return queries.SetFileVersionAndIdentifiedPath(t.Context(), sqlcgen.SetFileVersionAndIdentifiedPathParams{
			LibraryKind: libraryKind, VersionID: &versionID, FileID: id,
		})
	})
	if err != nil {
		t.Fatalf("file filing: %v", err)
	}
}

// applyPlaceFollowUps makes id the copy, the sidecar or the tombstone spec says.
func applyPlaceFollowUps(t *testing.T, queries *sqlcgen.Queries, spec FileSpec, id int64) error {
	t.Helper()
	if spec.CopyOf != 0 && spec.ID == 0 {
		copyOf := spec.CopyOf
		made, err := queries.MakeFileACopy(t.Context(), sqlcgen.MakeFileACopyParams{CopyOf: &copyOf, ID: id})
		if err == nil && made != 1 {
			err = fmt.Errorf("file %d was not made a copy of file %d", id, copyOf)
		}
		if err != nil {
			return err
		}
	}
	if spec.AttachedToFile != 0 {
		attachedTo := spec.AttachedToFile
		err := queries.SetFileAttachedTo(t.Context(),
			sqlcgen.SetFileAttachedToParams{AttachedToFile: &attachedTo, ID: id})
		if err != nil {
			return fmt.Errorf("attached_to_file: %w", err)
		}
	}
	if spec.RemovedAt != 0 {
		removedAt := spec.RemovedAt
		err := queries.TombstoneFile(t.Context(), sqlcgen.TombstoneFileParams{RemovedAt: &removedAt, ID: id})
		if err != nil {
			return fmt.Errorf("removed_at: %w", err)
		}
	}
	return nil
}

// ProbeSpec is a file's probe row, what the node reported.
type ProbeSpec struct {
	FileID     int64  // required
	ProbeJSON  string // defaults to "{}"
	DurationMs int64  // defaults to 0
}

// Probe adds, or replaces, the probe of spec.FileID.
func Probe(t *testing.T, database *store.Database, spec ProbeSpec) {
	t.Helper()
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.UpsertFileProbe(t.Context(), sqlcgen.UpsertFileProbeParams{
			FileID: spec.FileID, ProbeJSON: defaultString(spec.ProbeJSON, "{}"), DurationMs: spec.DurationMs,
		})
	})
	if err != nil {
		t.Fatalf("file probe: %v", err)
	}
}

// StreamSpec is one of a file's tracks: its own (VideoFile left at 0, taken from BytesFile), or a
// sidecar's track on the video it serves (VideoFile set to that file's id).
type StreamSpec struct {
	ID                int64  // 0: let the database choose
	BytesFile         int64  // required: the file whose bytes carry it
	VideoFile         int64  // 0: the same file (its own track)
	StreamIndex       int64  // its index in the container; ignored (NULL) for a sidecar's own track
	Kind              string // required: video/audio/subtitle/attachment
	Codec             string // "": NULL
	ContainerLanguage string // "": NULL
	Language          string // "": NULL
	ContainerTitle    string // "": NULL
	DispositionFlags  int64  // defaults to 0
	ChannelCount      int64  // 0: NULL
}

// Stream adds a track row and returns its id.
func Stream(t *testing.T, database *store.Database, spec StreamSpec) int64 {
	t.Helper()
	videoFile := defaultInt(spec.VideoFile, spec.BytesFile)
	if spec.ID != 0 || videoFile != spec.BytesFile {
		// A chosen id, or a sidecar's track on the video it serves (no writer upserts that pairing
		// by id), is built directly.
		id := spec.ID
		if id == 0 {
			next := storetest.QueryRaw(t, database.Path(), "SELECT coalesce(max(id), 0) + 1 FROM stream")
			generated, err := strconv.ParseInt(next[0], 10, 64)
			if err != nil {
				t.Fatalf("parse the next stream id %q: %v", next[0], err)
			}
			id = generated
		}
		// A sidecar's track (bytes_file apart from video_file) carries no stream index of its own:
		// the whole file is the track, as InsertSidecarStream also always writes NULL (schema.md,
		// Files: "NULL for a whole-file sidecar").
		streamIndex := strconv.FormatInt(spec.StreamIndex, 10)
		if videoFile != spec.BytesFile {
			streamIndex = sqlNull
		}
		storetest.ExecRaw(t, database.Path(), fmt.Sprintf(
			`INSERT INTO stream (id, bytes_file, video_file, stream_index, kind, codec, container_language,
			 language, container_title, disposition_flags, channel_count)
			 VALUES (%d, %d, %d, %s, %s, %s, %s, %s, %s, %d, %s)`,
			id, spec.BytesFile, videoFile, streamIndex, sqlString(spec.Kind), sqlNullableString(spec.Codec),
			sqlNullableString(spec.ContainerLanguage), sqlNullableString(spec.Language),
			sqlNullableString(spec.ContainerTitle), spec.DispositionFlags, sqlNullableInt(spec.ChannelCount)))
		return id
	}

	var codec, containerLanguage, language, containerTitle *string
	if spec.Codec != "" {
		codec = &spec.Codec
	}
	if spec.ContainerLanguage != "" {
		containerLanguage = &spec.ContainerLanguage
	}
	if spec.Language != "" {
		language = &spec.Language
	}
	if spec.ContainerTitle != "" {
		containerTitle = &spec.ContainerTitle
	}
	var channelCount *int64
	if spec.ChannelCount != 0 {
		channelCount = &spec.ChannelCount
	}
	streamIndex := spec.StreamIndex
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.UpsertStream(t.Context(), sqlcgen.UpsertStreamParams{
			FileID: spec.BytesFile, StreamIndex: &streamIndex, Kind: spec.Kind, Codec: codec,
			ContainerLanguage: containerLanguage, Language: language, ContainerTitle: containerTitle,
			DispositionFlags: spec.DispositionFlags, ChannelCount: channelCount,
		})
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	found := storetest.QueryRaw(t, database.Path(),
		fmt.Sprintf("SELECT id FROM stream WHERE bytes_file = %d AND stream_index = %d", spec.BytesFile, streamIndex))
	if len(found) != 1 {
		t.Fatalf("find the stream just written: %d rows, want 1", len(found))
	}
	generated, err := strconv.ParseInt(found[0], 10, 64)
	if err != nil {
		t.Fatalf("parse the stream id %q: %v", found[0], err)
	}
	return generated
}
