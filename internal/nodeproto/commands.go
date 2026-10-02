// SPDX-License-Identifier: GPL-3.0-or-later

package nodeproto

// CommandType names one kind of command (cut-a §4). The set is closed; cancel comes with the
// work it commands. CommandEncodeArtwork is in artwork.go, CommandPrepare in prepare.go,
// CommandExtractSubtitle in subtitle.go, CommandMeasureLoudness in loudness.go.
type CommandType string

// The command types.
const (
	CommandScan CommandType = "scan"
	// CommandCancel stops the attempt the command names (Command.AttemptToken) that the node is
	// working on: its FFmpeg is stopped, and nothing more is reported for it (cut-a §5.3).
	CommandCancel CommandType = "cancel"
)

// Command is one instruction from the core, delivered by the long-poll: an envelope, naming the job
// and the attempt, around the work it asks for. CommandSeq increases by one per command for a node;
// the node asks for commands after the last one it took.
type Command struct {
	CommandSeq int64 `json:"commandSeq"`
	// JobID is the core's job the command is for: the work a node answers with a Result of the same
	// id, or the job a cancel stops. Zero for a scan.
	JobID int64 `json:"jobId,omitempty"`
	// AttemptToken names this sending of the job's work, made by the core for it alone: the node
	// holds the work by it, a cancel names it, and every result echoes it, so the core takes a result
	// only for the attempt that is waiting (D238 §2, S83, S84). Empty for a scan.
	AttemptToken string `json:"attemptToken,omitempty"`
	// Priority is the job's: the node starts what waits for a slot, the highest first and, at one
	// priority, the first to come (D234 §3). Zero for a scan and a cancel.
	Priority int64 `json:"priority,omitempty"`
	Work
}

// Work is what a command asks for: its type, and the payload of that type, which a result echoes
// (Result.Work). It names no job: the envelope does.
type Work struct {
	Type CommandType `json:"type"`
	Scan *Scan       `json:"scan,omitempty"`
	// EncodeArtwork is set for CommandEncodeArtwork.
	EncodeArtwork *ArtworkBatch `json:"encodeArtwork,omitempty"`
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

// WithoutBytes is the work as a result echoes it, and the core keeps it: without the bytes it
// carried, an image or a fetched text, which their hashes stand for.
func (work Work) WithoutBytes() Work {
	if work.EncodeArtwork != nil {
		images := make([]Artwork, len(work.EncodeArtwork.Images))
		for index, image := range work.EncodeArtwork.Images {
			image.SourceImage = nil
			images[index] = image
		}
		work.EncodeArtwork = &ArtworkBatch{Images: images}
	}
	if work.ExtractSubtitle != nil {
		extract := *work.ExtractSubtitle
		extract.FetchedSubRip = nil
		work.ExtractSubtitle = &extract
	}
	return work
}
