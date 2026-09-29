// SPDX-License-Identifier: GPL-3.0-or-later

package nodeproto

// CommandType names one kind of command (cut-a §4). The set is closed; cancel comes with the
// work it commands. CommandArtwork is in artwork.go, CommandPrepare in prepare.go,
// CommandExtractSubtitle in subtitle.go, CommandMeasureLoudness in loudness.go.
type CommandType string

// The command types.
const (
	CommandScan CommandType = "scan"
	// CommandCancel stops a job the node is working on: its FFmpeg is stopped, and nothing more
	// is reported for it (cut-a §5.3).
	CommandCancel CommandType = "cancel"
)

// Command is one instruction from the core, delivered by the long-poll. Seq increases by one per
// command for a node; the node asks for commands after the last one it took.
type Command struct {
	Seq  int64       `json:"seq"`
	Type CommandType `json:"type"`
	Scan *Scan       `json:"scan,omitempty"`
	// Artwork is set for CommandArtwork.
	Artwork *Artwork `json:"artwork,omitempty"`
	// Prepare is set for CommandPrepare.
	Prepare *Prepare `json:"prepare,omitempty"`
	// ExtractSubtitle is set for CommandExtractSubtitle.
	ExtractSubtitle *ExtractSubtitle `json:"extractSubtitle,omitempty"`
	// MeasureLoudness is set for CommandMeasureLoudness.
	MeasureLoudness *MeasureLoudness `json:"measureLoudness,omitempty"`
	// Cancel is set for CommandCancel.
	Cancel *Cancel `json:"cancel,omitempty"`
}

// Cancel names the job to stop, by the core's job id.
type Cancel struct {
	JobID int64 `json:"jobId"`
}

// Scan asks the node to scan one of its roots.
type Scan struct {
	Root string `json:"root"`
}

// Commands is the long-poll's answer: the commands after the one the node named, possibly none
// when the poll's time ran out.
type Commands struct {
	Commands []Command `json:"commands"`
}
