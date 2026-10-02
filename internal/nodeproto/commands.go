// SPDX-License-Identifier: GPL-3.0-or-later

package nodeproto

// CommandType names one kind of command (cut-a §4). The set is closed; cancel comes with the
// work it commands. CommandEncodeArtwork is in artwork.go, CommandPrepare in prepare.go,
// CommandExtractSubtitle in subtitle.go, CommandMeasureLoudness in loudness.go.
type CommandType string

// The command types.
const (
	CommandScan CommandType = "scan"
	// CommandCancel stops the job the command names (Command.JobID) that the node is working
	// on: its FFmpeg is stopped, and nothing more is reported for it (cut-a §5.3).
	CommandCancel CommandType = "cancel"
)

// Command is one instruction from the core, delivered by the long-poll. CommandSeq increases by one per
// command for a node; the node asks for commands after the last one it took.
type Command struct {
	CommandSeq int64       `json:"commandSeq"`
	Type       CommandType `json:"type"`
	// JobID is the core's job the command is for: the work a node answers with a Result of the same
	// id, or the job a cancel stops. Zero for a scan.
	JobID int64 `json:"jobId,omitempty"`
	// Priority is the job's: the node starts what waits for a slot, the highest first and, at one
	// priority, the first to come (D234 §3). Zero for a scan and a cancel.
	Priority int64 `json:"priority,omitempty"`
	Scan     *Scan `json:"scan,omitempty"`
	// EncodeArtwork is set for CommandEncodeArtwork.
	EncodeArtwork *Artwork `json:"encodeArtwork,omitempty"`
	// Prepare is set for CommandPrepare.
	Prepare *Prepare `json:"prepare,omitempty"`
	// ExtractSubtitle is set for CommandExtractSubtitle.
	ExtractSubtitle *ExtractSubtitle `json:"extractSubtitle,omitempty"`
	// MeasureLoudness is set for CommandMeasureLoudness.
	MeasureLoudness *MeasureLoudness `json:"measureLoudness,omitempty"`
}

// Scan asks the node to scan one of its roots.
type Scan struct {
	RootPath string `json:"rootPath"`
}

// Commands is the long-poll's answer: the commands after the one the node named, possibly none
// when the poll's time ran out.
type Commands struct {
	Commands []Command `json:"commands"`
}
