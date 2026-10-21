// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

// RecordType names one kind of a listing's or an examining's record (ScanRecord). The set is closed;
// the core keys its logic on it.
type RecordType string

// The record types.
const (
	RecordScanBegin    RecordType = "scan_begin"
	RecordObserved     RecordType = "observed"
	RecordSkip         RecordType = "skip"
	RecordError        RecordType = "error"
	RecordScanComplete RecordType = "scan_complete"
	RecordMount        RecordType = "mount"
	RecordSidecar      RecordType = "sidecar"
)

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
	Path       Path        `json:"path"`
	Kind       SidecarKind `json:"kind"`
	SizeBytes  int64       `json:"sizeBytes"`
	ModifiedAt int64       `json:"modifiedAt"`
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

// Status is the node saying it is alive and what it is doing, posted to PathStatus each tick. The
// core marks a node silent when none arrives for a while (cut-a §4, liveness); silence is only ever
// read as silence (D34).
type Status struct {
	// SentAt is when the node sent it, by the node's clock, in Unix milliseconds.
	SentAt int64 `json:"sentAt"`
	// PreparedUsage is what the node's prepared directory holds against its budget, absent when the
	// node prepares nothing (D217 §5).
	PreparedUsage *PreparedUsage `json:"preparedUsage,omitempty"`
	// Disks are the filesystems the node's roots and prepared directory are on (D225 §3).
	Disks []Disk `json:"disks,omitempty"`
	// Held is every attempt the node holds, waiting or running, and every one whose result the core
	// has not yet acknowledged, which the core reconciles with the jobs it gave the node (D234 §6).
	Held []HeldAttempt `json:"held,omitempty"`
	// TakenCommandSeq is the last command the node had taken when it made the list: the core judges
	// only the attempts it sent up to it, so one still on its way is not taken for lost.
	TakenCommandSeq int64 `json:"takenCommandSeq"`
	// Stuck is the work the node gave up on that has not ended, waiting on a device (D233 §10).
	Stuck []Stuck `json:"stuck,omitempty"`
	// Watches say, for each root, whether the node watches it for changes (D27, D272 §9).
	Watches []RootWatch `json:"watches,omitempty"`
}

// Stuck is work the node answered as stalled that has not ended: a tool's process that outlived
// its kill, waiting on a device, which it does until the device answers, if ever (D233 §10). It
// holds no slot.
type Stuck struct {
	// Tool is the program, "ffmpeg" or "ffprobe", or "node" for a listing's or an examining's own
	// reading.
	Tool string `json:"tool"`
	// PID is its process; zero for the node's own reading.
	PID int `json:"pid"`
	// Paths are where it was reading: the device it waits on holds one of them.
	Paths []string `json:"paths,omitempty"`
	// Since is when the node gave up on it, in Unix milliseconds.
	Since int64 `json:"since"`
}

// HeldState is where an attempt the node holds is.
type HeldState string

// The states an attempt the node holds is in.
const (
	HeldWaiting HeldState = "waiting"
	HeldRunning HeldState = "running"
	// HeldPaused is an attempt whose run the node paused, so play-driven work can read its device
	// alone (D240 §6).
	HeldPaused HeldState = "paused"
	// HeldEnded is an attempt whose work ended, its result not yet acknowledged: sent again until it
	// is.
	HeldEnded HeldState = "ended"
)

// HeldAttempt is one attempt the node holds, by its job and token.
type HeldAttempt struct {
	JobID        int64     `json:"jobId"`
	AttemptToken string    `json:"attemptToken"`
	State        HeldState `json:"state"`
	// SinceMovedMs is how long it has not moved, its paused time apart: running work moves as its
	// FFmpeg advances or its reading reads, and waiting work as the work running does. The core holds
	// work only while it moves (D233 §5).
	SinceMovedMs int64 `json:"sinceMovedMs"`
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
	// IsNotAnswering is a failure that is its disk not answering, or failing a read as a network
	// share whose host is gone does: what is under it cannot be read now (D233 §10).
	IsNotAnswering bool `json:"isNotAnswering,omitempty"`
	// Device is the id of the filesystem a root is on, and IsMountPoint whether it differs from its
	// parent's, as a scan's facts say them (D42, D208): the core compares them with the root's last
	// complete scan to tell its disk back from the empty directory an unmounted one leaves. Zero for
	// the prepared directory, and when the measure failed.
	Device       uint64 `json:"device,omitempty"`
	IsMountPoint bool   `json:"isMountPoint,omitempty"`
}

// PreparedUsage is what a node's prepared directory holds, against its budget and its disk's
// limit (D217 §5).
type PreparedUsage struct {
	UsedBytes            int64 `json:"usedBytes"`
	BudgetBytes          int64 `json:"budgetBytes"`
	DiskUsedPercent      int   `json:"diskUsedPercent"`
	DiskUsedLimitPercent int   `json:"diskUsedLimitPercent"`
	// DiskAvailableBytes is how much of the disk is free, and DiskMinFreeBytes the floor the node
	// keeps free on it, whatever its percentage (node.prepared_disk_min_free_mib).
	DiskAvailableBytes int64 `json:"diskAvailableBytes"`
	DiskMinFreeBytes   int64 `json:"diskMinFreeBytes"`
}

// ScanBegin opens a scan of one media root. It carries the node's effective uid and gid, so a
// permission error below it can be read against who was asking (D163 §8).
type ScanBegin struct {
	RootPath string `json:"rootPath"`
	UID      int    `json:"uid"`
	GID      int    `json:"gid"`
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
// valid UTF-8; otherwise Text is a readable approximation and Raw holds the path's bytes as the
// filesystem gave them. The core normalises Text to NFC for identity and display, and opens by
// the raw bytes.
type Path struct {
	Text string `json:"text"`
	Raw  []byte `json:"raw,omitempty"`
}

// Bytes is the path as the filesystem gave it: Raw when it is set, else Text.
func (path Path) Bytes() []byte {
	if path.Raw != nil {
		return path.Raw
	}
	return []byte(path.Text)
}

// Observed is a file the node enumerated, probed and hashed.
type Observed struct {
	Path       Path  `json:"path"`
	SizeBytes  int64 `json:"sizeBytes"`
	ModifiedAt int64 `json:"modifiedAt"`
	// Identity is the payload hash (D179): its scheme, what it was anchored on, the SHA-256 in
	// hex and the payload's length.
	Identity Identity `json:"identity"`
	Probe    Probe    `json:"probe"`
	// OpensubtitlesHash is OpenSubtitles' moviehash of the file, 16 hex digits, which finds
	// subtitles made for these exact bytes (D224 §3); empty for a file too small to have one.
	OpensubtitlesHash string `json:"opensubtitlesHash,omitempty"`
	// FilesystemDevice and Inode are where the file was read, as a listing names them (Listed),
	// which the core keeps to tell the file moved by them later (D304 §5); 0 where unknown.
	FilesystemDevice uint64 `json:"filesystemDevice,omitempty"`
	Inode            uint64 `json:"inode,omitempty"`
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
	// SkipSystem is a folder a NAS or an operating system keeps for itself beside the media: a
	// recycle bin, snapshots, thumbnails, an index (design/research/library-scanner-practice-
	// 2026-10-16.md §5.3: Synology's "#recycle" was read as a film's folder, a series' in a series
	// library).
	SkipSystem SkipRule = "system"
	// SkipDiscImage is a disc's image, an .iso, or a disc's folder, VIDEO_TS or BDMV, which the
	// beta does not read (D306 §5): listed as skipped, so a film is not missing in silence.
	SkipDiscImage SkipRule = "disc_image_not_supported_yet"
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

// ScanError is something the node could not read. Repeats of one code and scope in one directory
// collapse into one record, which counts its occurrences (D40).
type ScanError struct {
	Path            Path          `json:"path"`
	Scope           ErrorScope    `json:"scope"`
	Code            ErrorCode     `json:"code"`
	OccurrenceCount int           `json:"occurrenceCount"`
	PlatformError   PlatformError `json:"platformError"`
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
	RootPath     string `json:"rootPath"`
	SkippedCount int    `json:"skippedCount"`
	// ListedCount is how many entries a listing listed.
	ListedCount int `json:"listedCount"`
	ErrorCount  int `json:"errorCount"`
	// IsStoppedByErrors is true when the node stopped enumerating the root after too many errors.
	IsStoppedByErrors bool `json:"isStoppedByErrors"`
}
