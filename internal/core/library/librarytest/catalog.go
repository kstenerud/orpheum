// SPDX-License-Identifier: AGPL-3.0-or-later

package librarytest

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

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

// ItemSpec is an item row a test builds directly: a collection's or a person's, or a title's own,
// which Title makes for it.
type ItemSpec struct {
	ID        int64  // 0: let the database choose
	Kind      string // "title", "collection" or "person"; defaults to "title"
	LibraryID int64  // 0: no library
	AddedAt   int64  // defaults to 1
}

// Item adds an item row and returns its id.
func Item(t *testing.T, database *store.Database, spec ItemSpec) int64 {
	t.Helper()
	kind := defaultString(spec.Kind, "title")
	addedAt := defaultInt(spec.AddedAt, 1)
	if spec.ID != 0 {
		storetest.ExecRaw(t, database.Path(), fmt.Sprintf(
			"INSERT INTO item (id, kind, library_id, added_at) VALUES (%d, %s, %s, %d)",
			spec.ID, sqlString(kind), sqlNullableInt(spec.LibraryID), addedAt))
		return spec.ID
	}
	var libraryID *int64
	if spec.LibraryID != 0 {
		libraryID = &spec.LibraryID
	}
	var id int64
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		var err error
		id, err = queries.InsertItem(t.Context(), sqlcgen.InsertItemParams{
			Kind: kind, LibraryID: libraryID, AddedAt: addedAt,
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
	Library             int64  // 0: no library
	Kind                string // movie/episode/extra/other; defaults to "movie"
	Year                int64  // 0: NULL
	RuntimeMs           int64  // 0: NULL
	FollowedSource      string // "": NULL; else nfo/tmdb/tvdb/none
	FollowedSourceID    string // "": NULL
	IdentificationState string // defaults to "identified"
	AddedAt             int64  // the item's added_at; defaults to 1
}

// Title adds an item of kind "title" and the title row on it, and returns their shared id.
func Title(t *testing.T, database *store.Database, spec TitleSpec) int64 {
	t.Helper()
	itemID := Item(t, database, ItemSpec{ID: spec.ID, Kind: "title", LibraryID: spec.Library, AddedAt: spec.AddedAt})
	kind := defaultString(spec.Kind, "movie")
	state := defaultString(spec.IdentificationState, "identified")
	var year, runtimeMs *int64
	if spec.Year != 0 {
		year = &spec.Year
	}
	if spec.RuntimeMs != 0 {
		runtimeMs = &spec.RuntimeMs
	}
	var followedSource, followedSourceID *string
	if spec.FollowedSource != "" {
		followedSource = &spec.FollowedSource
	}
	if spec.FollowedSourceID != "" {
		followedSourceID = &spec.FollowedSourceID
	}
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.InsertTitle(t.Context(), sqlcgen.InsertTitleParams{
			ID: itemID, Kind: kind, Year: year, RuntimeMs: runtimeMs,
			FollowedSource: followedSource, FollowedSourceID: followedSourceID, IdentificationState: state,
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
// sidecar (SidecarKind set) attached to one.
type FileSpec struct {
	ID                int64  // 0: let the database choose
	LibraryID         int64  // required
	NodeID            int64  // required
	Path              string // required
	ReportedPathRaw   []byte // nil: path's own bytes, as a scan of an NFC name would report it
	SizeBytes         int64  // 0: NULL
	ModifiedAt        int64  // 0: NULL
	PayloadHash       []byte // nil: NULL
	PayloadBytes      int64  // 0: NULL
	Filing            string // defaults to "listed"; ignored when SidecarKind is set (always "attached")
	FilingReason      string // "": NULL
	ErrorDetailJSON   string // "": NULL
	VersionID         int64  // 0: none
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
	if spec.ID != 0 {
		return fileWithChosenID(t, database, spec)
	}
	var id int64
	if spec.SidecarKind != "" {
		id = insertSidecarFile(t, database, spec)
	} else {
		id = insertOrdinaryFile(t, database, spec)
	}
	applyFileFollowUps(t, database, spec, id)
	return id
}

// fileWithChosenID builds a file row directly: a chosen id, or first_seen_at and last_seen_at
// apart (which no writer below produces: a rescan's UpdateObservedFile also revives an errored or
// skipped filing, which this file may mean to keep), has no production path to go through.
func fileWithChosenID(t *testing.T, database *store.Database, spec FileSpec) int64 {
	t.Helper()
	reportedPathRaw := fileReportedPathRaw(spec)
	filing := defaultString(spec.Filing, "listed")
	firstSeenAt := defaultInt(spec.FirstSeenAt, 1)
	lastSeenAt := defaultInt(spec.LastSeenAt, firstSeenAt)
	storetest.ExecRaw(t, database.Path(), fmt.Sprintf(
		`INSERT INTO file (id, library_id, node_id, path, reported_path_raw, size_bytes, modified_at,
		 payload_hash, payload_bytes, filing, filing_reason, error_detail_json, version_id,
		 attached_to_file, sidecar_kind, first_seen_at, last_seen_at, removed_at, opensubtitles_hash,
		 filesystem_device, inode)
		 VALUES (%d, %d, %d, %s, %s, %s, %s, %s, %s, %s, %s,
		         %s, %s, %s, %s, %d, %d, %s, %s, %s, %s)`,
		spec.ID, spec.LibraryID, spec.NodeID, sqlString(spec.Path), sqlBlob(reportedPathRaw),
		sqlNullableInt(spec.SizeBytes), sqlNullableInt(spec.ModifiedAt), sqlBlob(spec.PayloadHash),
		sqlNullableInt(spec.PayloadBytes), sqlString(filing), sqlNullableString(spec.FilingReason),
		sqlNullableString(spec.ErrorDetailJSON), sqlNullableInt(spec.VersionID), sqlNullableInt(spec.AttachedToFile),
		sqlNullableString(spec.SidecarKind), firstSeenAt, lastSeenAt, sqlNullableInt(spec.RemovedAt),
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
			LibraryID: spec.LibraryID, NodeID: spec.NodeID, Path: spec.Path,
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
			LibraryID: spec.LibraryID, NodeID: spec.NodeID, Path: spec.Path,
			ReportedPathRaw: fileReportedPathRaw(spec), SizeBytes: sizeBytes, ModifiedAt: modifiedAt,
			PayloadHash: spec.PayloadHash, PayloadBytes: payloadBytes,
			Filing: defaultString(spec.Filing, "listed"), FilingReason: filingReason, ErrorDetailJSON: errorDetailJSON,
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

// applyFileFollowUps sets id's version, attachment and removal, each through the setter
// production uses, when spec asks for one.
func applyFileFollowUps(t *testing.T, database *store.Database, spec FileSpec, id int64) {
	t.Helper()
	if spec.VersionID != 0 {
		versionID := spec.VersionID
		err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
			return queries.SetFileVersionAndIdentifiedPath(t.Context(),
				sqlcgen.SetFileVersionAndIdentifiedPathParams{VersionID: &versionID, ID: id})
		})
		if err != nil {
			t.Fatalf("file version_id: %v", err)
		}
	}
	if spec.AttachedToFile != 0 {
		attachedTo := spec.AttachedToFile
		err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
			return queries.SetFileAttachedTo(t.Context(),
				sqlcgen.SetFileAttachedToParams{AttachedToFile: &attachedTo, ID: id})
		})
		if err != nil {
			t.Fatalf("file attached_to_file: %v", err)
		}
	}
	if spec.RemovedAt != 0 {
		removedAt := spec.RemovedAt
		err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
			return queries.TombstoneFile(t.Context(), sqlcgen.TombstoneFileParams{RemovedAt: &removedAt, ID: id})
		})
		if err != nil {
			t.Fatalf("file removed_at: %v", err)
		}
	}
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
	StreamIndex       int64  // required: its index in the container
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
		storetest.ExecRaw(t, database.Path(), fmt.Sprintf(
			`INSERT INTO stream (id, bytes_file, video_file, stream_index, kind, codec, container_language,
			 language, container_title, disposition_flags, channel_count)
			 VALUES (%d, %d, %d, %d, %s, %s, %s, %s, %s, %d, %s)`,
			id, spec.BytesFile, videoFile, spec.StreamIndex, sqlString(spec.Kind), sqlNullableString(spec.Codec),
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
