// SPDX-License-Identifier: GPL-3.0-or-later

package nodeproto

// RecordType names one kind of report record (cut-a §4). The set is closed; the core keys its
// logic on it.
type RecordType string

// The record types. A job's progress and end are not records: each kind of job posts its own
// answer to its own path (PathPrepare, PathSubtitle, PathLoudness, PathArtwork).
const (
	RecordStatus       RecordType = "status"
	RecordScanBegin    RecordType = "scan_begin"
	RecordObserved     RecordType = "observed"
	RecordSkip         RecordType = "skip"
	RecordError        RecordType = "error"
	RecordScanComplete RecordType = "scan_complete"
	RecordMount        RecordType = "mount"
	RecordSidecar      RecordType = "sidecar"
)

// Record is one line of a JSONL report. Type says which one field is set; ReportSeq is the
// node's sequence number, increasing by one per record across the node's life.
type Record struct {
	ReportSeq    int64         `json:"reportSeq"`
	Type         RecordType    `json:"type"`
	ScanBegin    *ScanBegin    `json:"scanBegin,omitempty"`
	Observed     *Observed     `json:"observed,omitempty"`
	Skip         *Skip         `json:"skip,omitempty"`
	Error        *Error        `json:"error,omitempty"`
	ScanComplete *ScanComplete `json:"scanComplete,omitempty"`
	Status       *Status       `json:"status,omitempty"`
	Mount        *Mount        `json:"mount,omitempty"`
	Sidecar      *Sidecar      `json:"sidecar,omitempty"`
}

// SidecarKind is what a sidecar file is for.
type SidecarKind string

// The sidecar kinds.
const (
	// SidecarNFO is Kodi's metadata file. Its bytes travel in the record: identification reads
	// them in the core, which never opens a file (identification.md, Reading the sidecars).
	SidecarNFO SidecarKind = "nfo"
	// SidecarSubtitle is a text subtitle file beside its video, with its payload hash. Its bytes
	// are read when it is extracted.
	SidecarSubtitle SidecarKind = "subtitle"
	// SidecarImage is an image beside the media: a poster, a backdrop. Its bytes never travel in
	// a record; the node reads and re-encodes it when the core asks (D212 §8, D215).
	SidecarImage SidecarKind = "image"
)

// Sidecar is a file beside the media that is not media: an NFO, a subtitle or an image. The core
// files it as attached to its video (schema.md, attached).
type Sidecar struct {
	Path      Path        `json:"path"`
	Kind      SidecarKind `json:"kind"`
	SizeBytes int64       `json:"sizeBytes"`
	MtimeMs   int64       `json:"mtimeMs"`
	// NFOBody is the file's bytes, for an NFO only, and bounded: a larger NFO is an error record.
	NFOBody []byte `json:"nfoBody,omitempty"`
	// Identity is a subtitle's payload hash (D179): a text file is hashed whole, so a changed
	// subtitle has a new one, and its extracted text is kept by it (D217 §6).
	Identity *Identity `json:"identity,omitempty"`
}

// Mount is a directory below the root that is on another device than its parent: a mount
// boundary the walk crossed (D42). The core compares a scan's boundaries with the last complete
// scan's, so a disk unplugged from a mount point inside a root removes nothing under it (D208).
type Mount struct {
	Path             Path   `json:"path"`
	FilesystemDevice uint64 `json:"filesystemDevice"`
}

// Status is the node saying it is alive and what it is doing. The core marks a node silent when
// none arrives for a while (cut-a §4, liveness); silence is only ever read as silence (D34).
type Status struct {
	// SentAt is when the node sent it, by the node's clock, in Unix milliseconds.
	SentAt int64 `json:"sentAt"`
	// Activity is what the node is busy with: "idle", or "scanning <root>".
	Activity string `json:"activity"`
	// PreparedUsage is what the node's prepared directory holds against its budget, absent when the
	// node prepares nothing (D217 §5).
	PreparedUsage *PreparedUsage `json:"preparedUsage,omitempty"`
	// Disks are the filesystems the node's roots and prepared directory are on (D225 §3).
	Disks []Disk `json:"disks,omitempty"`
}

// Disk is one filesystem a node uses, named by a directory on it.
type Disk struct {
	// Path is the root or prepared directory it holds; several may share one filesystem.
	Path       string `json:"path"`
	TotalBytes int64  `json:"totalBytes"`
	// AvailableBytes are what a user other than root can still write there.
	AvailableBytes int64 `json:"availableBytes"`
	// Failure says why the node could not measure it; the sizes are zero then.
	Failure string `json:"failure,omitempty"`
}

// PreparedUsage is what a node's prepared directory holds, against its budget and its disk's
// limit (D217 §5).
type PreparedUsage struct {
	UsedBytes            int64 `json:"usedBytes"`
	BudgetBytes          int64 `json:"budgetBytes"`
	DiskUsedPercent      int   `json:"diskUsedPercent"`
	DiskUsedLimitPercent int   `json:"diskUsedLimitPercent"`
}

// ScanBegin opens a scan of one media root. It carries the node's effective uid and gid, so a
// permission error below it can be read against who was asking (D163 §8).
type ScanBegin struct {
	Root string `json:"root"`
	UID  int    `json:"uid"`
	GID  int    `json:"gid"`
	// FilesystemDevice, IsMountPoint and IsReadOnly are the root's filesystem facts as the scan
	// starts (D42). The core compares the first two with the last complete scan's: an unplugged
	// disk leaves the empty directory under its mount point, whose listing succeeds (D41, D208).
	// IsReadOnly is reported for what the node sees; the core does not act on it yet.
	FilesystemDevice uint64 `json:"filesystemDevice"`
	IsMountPoint     bool   `json:"isMountPoint"`
	IsReadOnly       bool   `json:"isReadOnly"`
}

// Path is a path as the filesystem gave it (D49): relative to its media root everywhere but
// Artwork.AbsolutePath, which says in its name that it is not. Text holds it exactly when it is
// valid UTF-8; otherwise Text is a readable approximation and Bytes holds the bytes. The core
// normalises Text to NFC for identity and display, and opens by the bytes.
type Path struct {
	Text  string `json:"text"`
	Bytes []byte `json:"bytes,omitempty"`
}

// Observed is a file the node enumerated, probed and hashed.
type Observed struct {
	Path      Path  `json:"path"`
	SizeBytes int64 `json:"sizeBytes"`
	MtimeMs   int64 `json:"mtimeMs"`
	// Identity is the payload hash (D179): its scheme, what it was anchored on, the SHA-256 in
	// hex and the payload's length.
	Identity Identity `json:"identity"`
	Probe    Probe    `json:"probe"`
	// OpenSubtitlesHash is OpenSubtitles' moviehash of the file, 16 hex digits, which finds
	// subtitles made for these exact bytes (D224 §3); empty for a file too small to have one.
	OpenSubtitlesHash string `json:"openSubtitlesHash,omitempty"`
}

// Identity is a file's payload identity (D179).
type Identity struct {
	Scheme       string `json:"scheme"`
	Anchor       string `json:"anchor"`
	PayloadHash  string `json:"payloadHash"`
	PayloadBytes int64  `json:"payloadBytes"`
}

// SkipRule names the rule that excluded a path (D40, D163). The set is closed.
type SkipRule string

// The skip rules.
const (
	// SkipTrash is .trash/ at a media root: previous versions of files an export replaced (D144).
	SkipTrash SkipRule = "trash"
	// SkipSample is a release sample: sample.mkv, x.sample.mkv, x-sample.mkv, a sample/ directory
	// (D162).
	SkipSample SkipRule = "sample"
	// SkipAppleDouble is a macOS AppleDouble sidecar, "._<name>": metadata another machine left,
	// never media (testing-strategy.md).
	SkipAppleDouble SkipRule = "appledouble"
	// SkipExtension is a file whose extension is not a video extension the node probes.
	SkipExtension SkipRule = "extension"
	// SkipSymlinkOutsideRoots is a symlink whose target is outside every media root (D206).
	SkipSymlinkOutsideRoots SkipRule = "symlink_outside_roots"
	// SkipNotRegular is something that is neither a file, a directory nor a symlink: a device,
	// a socket, a FIFO.
	SkipNotRegular SkipRule = "not_regular"
)

// Skip is a path an ignore rule excluded. It is not a failure, and not in the inventory.
type Skip struct {
	Path Path     `json:"path"`
	Rule SkipRule `json:"rule"`
}

// ErrorScope says how much an error leaves unknown (D40).
type ErrorScope string

// The scopes, narrowest first.
const (
	ScopeEntry   ErrorScope = "entry"
	ScopeSubtree ErrorScope = "subtree"
	ScopeRoot    ErrorScope = "root"
	ScopeRun     ErrorScope = "run"
)

// ErrorCode is the project's closed vocabulary for what went wrong (D40), not an errno.
type ErrorCode string

// The error codes.
const (
	CodePermissionDenied ErrorCode = "permission_denied"
	CodeNotFound         ErrorCode = "not_found"
	CodeIOError          ErrorCode = "io_error"
	CodeTimeout          ErrorCode = "timeout"
	CodeUnsupported      ErrorCode = "unsupported"
	CodeProbeFailed      ErrorCode = "probe_failed"
	CodeCancelled        ErrorCode = "cancelled"
)

// Error is something the node could not read. Repeats of one code and scope in one directory
// collapse into one record, which counts its occurrences (D40).
type Error struct {
	Path            Path          `json:"path"`
	Scope           ErrorScope    `json:"scope"`
	Code            ErrorCode     `json:"code"`
	OccurrenceCount int           `json:"occurrenceCount"`
	Platform        PlatformError `json:"platform"`
}

// PlatformError is the platform's truth for a person diagnosing it: the error text and, where the
// node could stat the path, its owner, group and mode (D163 §8).
type PlatformError struct {
	Text     string `json:"text"`
	OwnerUID int    `json:"ownerUid"`
	GroupGID int    `json:"groupGid"`
	ModeText string `json:"modeText"`
}

// ScanComplete closes a scan: the moment the core may apply removals in the regions no error
// left unknown (D39, D40). A scan with errors is complete, and says how many.
type ScanComplete struct {
	Root          string `json:"root"`
	ObservedCount int    `json:"observedCount"`
	SidecarCount  int    `json:"sidecarCount"`
	SkippedCount  int    `json:"skippedCount"`
	ErrorCount    int    `json:"errorCount"`
	// IsStoppedByErrors is true when the node stopped enumerating the root after too many errors.
	IsStoppedByErrors bool `json:"isStoppedByErrors"`
}
