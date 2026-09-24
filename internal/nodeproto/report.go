// SPDX-License-Identifier: GPL-3.0-or-later

package nodeproto

// RecordType names one kind of report record (cut-a §4). The set is closed; the core keys its
// logic on it.
type RecordType string

// The record types a scan emits. Job and status records come with the node process.
const (
	RecordScanBegin    RecordType = "scan_begin"
	RecordObserved     RecordType = "observed"
	RecordSkip         RecordType = "skip"
	RecordError        RecordType = "error"
	RecordScanComplete RecordType = "scan_complete"
)

// Record is one line of a JSONL report. Type says which one field is set; Seq is the node's
// sequence number, increasing by one per record across the node's life.
type Record struct {
	Seq          int64         `json:"seq"`
	Type         RecordType    `json:"type"`
	ScanBegin    *ScanBegin    `json:"scanBegin,omitempty"`
	Observed     *Observed     `json:"observed,omitempty"`
	Skip         *Skip         `json:"skip,omitempty"`
	Error        *Error        `json:"error,omitempty"`
	ScanComplete *ScanComplete `json:"scanComplete,omitempty"`
}

// ScanBegin opens a scan of one media root. It carries the node's effective uid and gid, so a
// permission error below it can be read against who was asking (D163 §8).
type ScanBegin struct {
	Root string `json:"root"`
	UID  int    `json:"uid"`
	GID  int    `json:"gid"`
}

// Path is a path relative to its media root, as the filesystem gave it (D49). Text holds it
// exactly when it is valid UTF-8; otherwise Text is a readable approximation and Raw holds the
// bytes. The core normalises Text to NFC for identity and display, and opens by the bytes.
type Path struct {
	Text string `json:"text"`
	Raw  []byte `json:"raw,omitempty"`
}

// Observed is a file the node enumerated, probed and hashed.
type Observed struct {
	Path    Path  `json:"path"`
	Size    int64 `json:"size"`
	MtimeMs int64 `json:"mtimeMs"`
	// Identity is the payload hash (D179): its scheme, what it was anchored on, the SHA-256 in
	// hex and the payload's length.
	Identity Identity `json:"identity"`
	Probe    Probe    `json:"probe"`
}

// Identity is a file's payload identity (D179).
type Identity struct {
	Scheme        string `json:"scheme"`
	Anchor        string `json:"anchor"`
	Hash          string `json:"hash"`
	PayloadLength int64  `json:"payloadLength"`
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
// collapse into one record with a Count (D40).
type Error struct {
	Path  Path       `json:"path"`
	Scope ErrorScope `json:"scope"`
	Code  ErrorCode  `json:"code"`
	Count int        `json:"count"`
	Raw   ErrorRaw   `json:"raw"`
}

// ErrorRaw is the platform's truth for a person diagnosing it: the error text and, where the
// node could stat the path, its owner, group and mode (D163 §8).
type ErrorRaw struct {
	Text  string `json:"text"`
	Owner int    `json:"owner"`
	Group int    `json:"group"`
	Mode  string `json:"mode"`
}

// ScanComplete closes a scan: the moment the core may apply removals in the regions no error
// left unknown (D39, D40). A scan with errors is complete, and says how many.
type ScanComplete struct {
	Root     string `json:"root"`
	Observed int    `json:"observed"`
	Skipped  int    `json:"skipped"`
	Errors   int    `json:"errors"`
	// Stopped is true when the node stopped enumerating the root after too many errors.
	Stopped bool `json:"stopped"`
}
