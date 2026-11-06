// SPDX-License-Identifier: AGPL-3.0-or-later

package rowstest

import (
	"fmt"
	"strconv"
	"strings"
)

// Each builder returns the statements that insert its rows, separated by semicolons, for a test's
// execRaw or storetest.ExecRaw, which run every statement a string holds. A row the database gives
// an id when its spec's ID is zero; a row built with it (a title's work, a file's place) takes the
// same id through last_insert_rowid().

// The columns more than one table names.
const (
	columnName      = "name"
	columnKind      = "kind"
	columnRemovedAt = "removed_at"
	columnFileID    = "file_id"
)

// column is one column of an inserted row and its value as a SQL literal; an empty value leaves the
// column out, for its default.
type column struct {
	name  string
	value string
}

// insert is the statement that inserts one row of table with columns, leaving out those without a
// value.
func insert(table string, columns ...column) string {
	var names, values []string
	for _, each := range columns {
		if each.value == "" {
			continue
		}
		names = append(names, each.name)
		values = append(values, each.value)
	}
	return fmt.Sprintf("INSERT INTO %q (%s) VALUES (%s)", table, strings.Join(names, ", "), strings.Join(values, ", "))
}

// Join joins builders' statements into one string, for a test's own builder of several rows.
func Join(each ...string) string {
	return strings.Join(each, ";\n")
}

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

// text is value as a single-quoted SQL literal.
func text(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// optionalText is value as a literal, or nothing (the column's default, NULL) when it is empty.
func optionalText(value string) string {
	if value == "" {
		return ""
	}
	return text(value)
}

// integer is value as a literal.
func integer(value int64) string {
	return strconv.FormatInt(value, 10)
}

// optionalInteger is value as a literal, or nothing (the column's default) when it is zero.
func optionalInteger(value int64) string {
	if value == 0 {
		return ""
	}
	return integer(value)
}

// optionalBlob is bytes as a blob literal, or nothing (NULL) when it is empty.
func optionalBlob(bytes []byte) string {
	if len(bytes) == 0 {
		return ""
	}
	return fmt.Sprintf("X'%x'", bytes)
}

// idOrLastInserted is id as a literal, or the id of the row just inserted when it is zero.
func idOrLastInserted(id int64) string {
	if id == 0 {
		return "last_insert_rowid()"
	}
	return integer(id)
}

// NodeSpec is an enrolled node.
type NodeSpec struct {
	ID         int64  // 0: let the database choose
	Name       string // defaults to "nas"
	EnrolledAt int64  // defaults to 1
}

// Node inserts a node.
func Node(spec NodeSpec) string {
	return insert("node",
		column{"id", optionalInteger(spec.ID)},
		column{columnName, text(defaultString(spec.Name, "nas"))},
		column{"enrolled_at", integer(defaultInt(spec.EnrolledAt, 1))})
}

// LibrarySpec is a library, and its root on a node when NodeID is set.
type LibrarySpec struct {
	ID              int64  // 0: let the database choose
	Name            string // defaults to "Films"
	Kind            string // movie/series/other; defaults to "movie"
	MetadataSource  string // defaults to "nfo"
	ChangeDetection string // defaults to "manual"
	CreatedAt       int64  // defaults to 1
	NodeID          int64  // the node its root is on; 0: no root
	RootPath        string // its root's path; defaults to "/m"
}

// Library inserts a library and, when its spec names a node, the library's root on it.
func Library(spec LibrarySpec) string {
	library := insert("library",
		column{"id", optionalInteger(spec.ID)},
		column{columnName, text(defaultString(spec.Name, "Films"))},
		column{columnKind, text(defaultString(spec.Kind, "movie"))},
		column{"metadata_source", text(defaultString(spec.MetadataSource, "nfo"))},
		column{"change_detection", text(defaultString(spec.ChangeDetection, "manual"))},
		column{"is_writable", "0"},
		column{"created_at", integer(defaultInt(spec.CreatedAt, 1))})
	if spec.NodeID == 0 {
		return library
	}
	return Join(library, insert("library_root",
		column{"library_id", idOrLastInserted(spec.ID)},
		column{"node_id", integer(spec.NodeID)},
		column{"root_path", text(defaultString(spec.RootPath, "/m"))}))
}

// ItemSpec is an item row: a series', a person's, or a title's own, which Title makes for it.
type ItemSpec struct {
	ID                  int64  // 0: let the database choose
	Kind                string // movie/series/episode/extra/other/person; defaults to "movie"
	LibraryID           int64  // the library whose pass files it (item.filing_library_id); 0: none
	AddedAt             int64  // defaults to 1
	RemovedAt           int64  // 0: none
	FollowedSource      string // "": NULL; else nfo/tmdb/tvdb/none
	IdentificationState string // defaults to "unidentified" for a series, "identified" for a title; NULL for a person
}

// Item inserts an item.
func Item(spec ItemSpec) string {
	kind := defaultString(spec.Kind, "movie")
	var state string
	switch kind {
	case "person":
	case "series":
		state = text(defaultString(spec.IdentificationState, "unidentified"))
	default:
		state = text(defaultString(spec.IdentificationState, "identified"))
	}
	return insert("item",
		column{"id", optionalInteger(spec.ID)},
		column{columnKind, text(kind)},
		column{"filing_library_id", optionalInteger(spec.LibraryID)},
		column{"added_at", integer(defaultInt(spec.AddedAt, 1))},
		column{columnRemovedAt, optionalInteger(spec.RemovedAt)},
		column{"followed_source", optionalText(spec.FollowedSource)},
		column{"identification_state", state})
}

// Title inserts a title: its item, of spec's kind, and its work.
func Title(spec ItemSpec) string {
	kind := defaultString(spec.Kind, "movie")
	spec.Kind = kind
	return Join(Item(spec), insert("work",
		column{"id", idOrLastInserted(spec.ID)},
		column{columnKind, text(kind)}))
}

// Series inserts a series: its item, whose Kind is "series" whatever spec says, and its collection,
// made by the scanner.
func Series(spec ItemSpec) string {
	spec.Kind = "series"
	return Join(Item(spec), insert("sequence",
		column{"id", idOrLastInserted(spec.ID)},
		column{columnKind, "'series'"},
		column{"created_by", "'scanner'"}))
}

// OrderingSpec is one of a series' orderings.
type OrderingSpec struct {
	ID       int64  // 0: let the database choose
	SeriesID int64  // required
	Name     string // defaults to "aired"
	Kind     string // "": NULL
}

// Ordering inserts an ordering of a series.
func Ordering(spec OrderingSpec) string {
	return insert("ordering",
		column{"id", optionalInteger(spec.ID)},
		column{"sequence_id", integer(spec.SeriesID)},
		column{columnName, text(defaultString(spec.Name, "aired"))},
		column{columnKind, optionalText(spec.Kind)})
}

// PositionSpec is a title's place in an ordering.
type PositionSpec struct {
	OrderingID int64   // required
	TitleID    int64   // required
	GroupID    int64   // its season, or disc; 0: none
	SortOrder  float64 // its order within the ordering
}

// Position inserts a title's position in an ordering.
func Position(spec PositionSpec) string {
	return insert("position",
		column{"ordering_id", integer(spec.OrderingID)},
		column{"work_id", integer(spec.TitleID)},
		column{"grp_id", optionalInteger(spec.GroupID)},
		column{"sort_order", strconv.FormatFloat(spec.SortOrder, 'g', -1, 64)})
}

// VersionSpec is a version of a title.
type VersionSpec struct {
	ID        int64 // 0: let the database choose
	TitleID   int64 // required
	CreatedAt int64 // defaults to 1
	RemovedAt int64 // 0: none
}

// Version inserts a version of a title.
func Version(spec VersionSpec) string {
	return insert("version",
		column{"id", optionalInteger(spec.ID)},
		column{"work_id", integer(spec.TitleID)},
		column{"created_at", integer(defaultInt(spec.CreatedAt, 1))},
		column{columnRemovedAt, optionalInteger(spec.RemovedAt)})
}

// PlaceSpec is a place a file is found at.
type PlaceSpec struct {
	ID          int64  // 0: let the database choose
	FileID      int64  // required
	NodeID      int64  // required
	Path        string // required; its reported bytes are its own
	ModifiedAt  int64  // 0: NULL
	Filing      string // read/skipped/errored; defaults to "read"
	FirstSeenAt int64  // defaults to 1
	LastSeenAt  int64  // defaults to FirstSeenAt
	RemovedAt   int64  // 0: none
}

// Place inserts a place a file is found at.
func Place(spec PlaceSpec) string {
	return place(spec, optionalInteger(spec.ID))
}

// place inserts a place with id, a literal: a file's first place takes the file's id.
func place(spec PlaceSpec, id string) string {
	firstSeenAt := defaultInt(spec.FirstSeenAt, 1)
	return insert("place",
		column{"id", id},
		column{columnFileID, idOrLastInserted(spec.FileID)},
		column{"node_id", integer(spec.NodeID)},
		column{"path", text(spec.Path)},
		column{"reported_path_raw", fmt.Sprintf("X'%x'", spec.Path)},
		column{"modified_at", optionalInteger(spec.ModifiedAt)},
		column{"filing", text(defaultString(spec.Filing, "read"))},
		column{"first_seen_at", integer(firstSeenAt)},
		column{"last_seen_at", integer(defaultInt(spec.LastSeenAt, firstSeenAt))},
		column{columnRemovedAt, optionalInteger(spec.RemovedAt)})
}

// FileSpec is a file, the place it was first found at, whose id is the file's, and its filing in a
// kind of library.
type FileSpec struct {
	ID          int64  // 0: let the database choose
	SizeBytes   int64  // 0: NULL
	PayloadHash []byte // nil: NULL
	NodeID      int64  // required
	Path        string // required
	ModifiedAt  int64  // 0: NULL
	Filing      string // the place's: read/skipped/errored; defaults to "read"
	FirstSeenAt int64  // defaults to 1
	LastSeenAt  int64  // defaults to FirstSeenAt
	RemovedAt   int64  // 0: none
	LibraryKind string // the kind of library it is filed in: movie/series/other; "": no filing
	FiledAs     string // listed/extra/unidentified; defaults to "listed"
	VersionID   int64  // the version it is filed as; 0: none
}

// File inserts a file, its place, and its filing when its spec names a kind of library.
func File(spec FileSpec) string {
	built := []string{
		insert("file",
			column{"id", optionalInteger(spec.ID)},
			column{"size_bytes", optionalInteger(spec.SizeBytes)},
			column{"payload_hash", optionalBlob(spec.PayloadHash)}),
		place(PlaceSpec{
			FileID: spec.ID, NodeID: spec.NodeID, Path: spec.Path, ModifiedAt: spec.ModifiedAt,
			Filing: spec.Filing, FirstSeenAt: spec.FirstSeenAt, LastSeenAt: spec.LastSeenAt, RemovedAt: spec.RemovedAt,
		}, idOrLastInserted(spec.ID)),
	}
	if spec.LibraryKind != "" {
		built = append(built, insert("file_filing",
			column{columnFileID, idOrLastInserted(spec.ID)},
			column{"library_kind", text(spec.LibraryKind)},
			column{"filing", text(defaultString(spec.FiledAs, "listed"))},
			column{"version_id", optionalInteger(spec.VersionID)}))
	}
	return Join(built...)
}

// ProbeSpec is what a node probed of a file.
type ProbeSpec struct {
	FileID    int64  // required
	ProbeJSON string // defaults to "{}"
}

// Probe inserts a file's probe.
func Probe(spec ProbeSpec) string {
	return insert("file_probe",
		column{columnFileID, integer(spec.FileID)},
		column{"probe_json", text(defaultString(spec.ProbeJSON, "{}"))})
}

// StreamSpec is a stream of a file.
type StreamSpec struct {
	BytesFile        int64  // required: the file whose bytes carry it
	VideoFile        int64  // 0: the same file
	StreamIndex      int64  // its index in the container
	Kind             string // required: video/audio/subtitle/attachment
	Codec            string // "": NULL
	DispositionFlags int64  // defaults to 0
}

// Stream inserts a stream of a file.
func Stream(spec StreamSpec) string {
	return insert("stream",
		column{"bytes_file", integer(spec.BytesFile)},
		column{"video_file", integer(defaultInt(spec.VideoFile, spec.BytesFile))},
		column{"stream_index", integer(spec.StreamIndex)},
		column{columnKind, text(spec.Kind)},
		column{"codec", optionalText(spec.Codec)},
		column{"disposition_flags", integer(spec.DispositionFlags)})
}
