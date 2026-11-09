// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

// The commands of a scan (D240 §2–§4): the node lists what is under a root, which reads no file,
// and examines the files the core asks it to, a batch at a time.
const (
	// CommandList lists what is under a root, answered by Result.Listing.
	CommandList CommandType = "list"
	// CommandExamine reads a batch of files under a root, answered by Result.Examined.
	CommandExamine CommandType = "examine"
)

// RecordListed is an entry a listing found: a file or a directory, with what its stat says.
const RecordListed RecordType = "listed"

// ScanRecord is one record of a listing or an examining: Type says which one field is set. It is a
// report's Record without the stream's sequence and status, which name no scan.
type ScanRecord struct {
	Type          RecordType     `json:"type"`
	ScanBegin     *ScanBegin     `json:"scanBegin,omitempty"`
	Listed        *Listed        `json:"listed,omitempty"`
	Observed      *Observed      `json:"observed,omitempty"`
	Sidecar       *Sidecar       `json:"sidecar,omitempty"`
	Mount         *Mount         `json:"mount,omitempty"`
	Skip          *Skip          `json:"skip,omitempty"`
	Error         *ScanError     `json:"error,omitempty"`
	ScanComplete  *ScanComplete  `json:"scanComplete,omitempty"`
	ChunksChecked *ChunksChecked `json:"chunksChecked,omitempty"`
}

// List asks the node to list what is under Path, relative to RootPath, one of its roots, to Depth
// levels below it: 1 for a directory's own entries, 0 for the whole tree.
type List struct {
	RootPath string `json:"rootPath"`
	// Path is relative to the root, "." for the root itself.
	Path  string `json:"path"`
	Depth int    `json:"depth"`
	// ErrorLimit is how many errors the listing may meet before the node stops listing the root and
	// says it stopped (D40: a dead share must not flood the listing): the core's scan_root_error_limit.
	// Zero lists without one.
	ErrorLimit int `json:"errorLimit"`
}

// Listing is a listing's answer: scanBegin with the root's facts, then a record for every entry,
// in byte order of name, a directory's entries after it — listed with its stat; mount for a
// directory on another device than its parent; skip for a link out of the roots or what is neither
// a file nor a directory; error for what could not be read — and scanComplete, unless it was
// stopped. It reads no file: which files need reading is the core's to say (D240 §3).
type Listing struct {
	Records []ScanRecord `json:"records"`
	// Refusal says why the node listed nothing: a root it was not given (D107).
	Refusal string `json:"refusal,omitempty"`
	// Stall is a listing whose reading did not move for its window, waiting on its disk: the node
	// answered it so and left its reading to end when the disk answers (D233 §10).
	Stall *Stall `json:"stall,omitempty"`
}

// Listed is an entry a listing found.
type Listed struct {
	Path        Path  `json:"path"`
	IsDirectory bool  `json:"isDirectory"`
	SizeBytes   int64 `json:"sizeBytes"`
	ModifiedAt  int64 `json:"modifiedAt"`
	// FilesystemDevice is the device number of the filesystem the entry is on.
	FilesystemDevice uint64 `json:"filesystemDevice"`
	// Inode is the entry's inode on that filesystem, 0 where the platform gives none: with the
	// device, size and modified time it tells a file moved on its disk (D304 §5, D240 §3).
	Inode uint64 `json:"inode,omitempty"`
}

// ExamineKind is what the node reads of a file it examines.
type ExamineKind string

// The kinds of examining.
const (
	// ExamineVideo probes and hashes a video (D179), with its OpenSubtitles moviehash (D224 §3).
	ExamineVideo ExamineKind = "video"
	// ExamineNFO reads an NFO's bytes, to a bound (D182).
	ExamineNFO ExamineKind = "nfo"
	// ExamineSubtitle hashes a text subtitle (D217 §6).
	ExamineSubtitle ExamineKind = "subtitle"
)

// Examine asks the node to read a batch of files under RootPath, one of its roots: what each needs
// is the core's choice (D240 §4, §8).
type Examine struct {
	RootPath string        `json:"rootPath"`
	Files    []ExamineFile `json:"files"`
}

// ExamineFile is one file to examine, by its path relative to the root, as the listing gave it.
type ExamineFile struct {
	Path Path        `json:"path"`
	Kind ExamineKind `json:"kind"`
	// ChecksChunksOf is, for a video a full rescan reads again, the core's id of its file: the node
	// checks every file of its stores of chunks against its record after examining it, drops what is
	// missing or damaged, and says so in a chunksChecked record (D392 §4). Zero checks nothing.
	ChecksChunksOf int64 `json:"checksChunksOf,omitempty"`
}

// ChunksChecked is what the node found checking a file's stores of chunks against their records: how
// many of their files, inits and chunks, it checked, and each it dropped, missing, damaged or
// unrecorded, which is made again when a player asks for it (D392 §4).
type ChunksChecked struct {
	FileID       int64          `json:"fileId"`
	CheckedCount int            `json:"checkedCount"`
	Dropped      []DroppedChunk `json:"dropped,omitempty"`
}

// DroppedChunk is a file of a store dropped because it was not what its record says: Name, its init or
// a chunk, in the store at StoreDir; or, with Name empty, the whole store, made before its chunks were
// recorded. FileCount is how many files went, and Why what differed.
type DroppedChunk struct {
	StoreDir  string `json:"storeDir"`
	Name      string `json:"name,omitempty"`
	FileCount int    `json:"fileCount"`
	Why       string `json:"why"`
}

// Examined is an examining's answer: for each file in turn, observed for a video, sidecar for an
// NFO or subtitle, or error for one it could not read; then a chunksChecked record for each video
// whose stores it was asked to check (ExamineFile.ChecksChunksOf).
type Examined struct {
	Records []ScanRecord `json:"records"`
	// Refusal says why the node examined nothing: a root it was not given (D107).
	Refusal string `json:"refusal,omitempty"`
	// Stall is an examining whose reading did not move for its window, as a listing's (Listing.Stall).
	Stall *Stall `json:"stall,omitempty"`
}
