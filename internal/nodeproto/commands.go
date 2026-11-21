// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

// CommandType names one kind of command (cut-a §4). The set is closed; cancel comes with the
// work it commands. CommandEncodeArtwork is in artwork.go,
// CommandExtractSubtitle in subtitle.go, CommandReadThrough in readthrough.go, CommandMakeChunks in
// chunks.go.
type CommandType string

// The command types.
const (
	// CommandCancel stops the attempt the command names (Command.AttemptToken) that the node is
	// working on: its FFmpeg is stopped, and nothing more is reported for it (cut-a §5.3).
	CommandCancel CommandType = "cancel"
	// CommandReprioritize gives the attempt the command names its new priority, and says whether it is
	// still play-driven (Command.Priority, Command.IsPlayDriven): work no person waits for any more
	// falls to background priority, and pauses nothing more (D234 §3, D240 §6).
	CommandReprioritize CommandType = "reprioritize"
)

// Command is one instruction from the core, delivered by the long-poll: an envelope, naming the job
// and the attempt, around the work it asks for. CommandSeq increases by one per command for a node;
// the node asks for commands after the last one it took.
type Command struct {
	CommandSeq int64 `json:"commandSeq"`
	// JobID is the core's job the command is for: the work a node answers with a Result of the same
	// id, or the job a cancel stops.
	JobID int64 `json:"jobId,omitempty"`
	// AttemptToken names this sending of the job's work, made by the core for it alone: the node
	// holds the work by it, a cancel names it, and every result echoes it, so the core takes a result
	// only for the attempt that is waiting (D238 §2, S83, S84).
	AttemptToken string `json:"attemptToken,omitempty"`
	// Priority is the job's: the node starts what waits for a slot, the highest first and, at one
	// priority, the first to come (D234 §3). Zero for a cancel.
	Priority int64 `json:"priority,omitempty"`
	// IsPlayDriven is work a person is waiting for, to play: on its device it pauses the background
	// work reading there, and no background work starts there while it runs (D240 §6).
	IsPlayDriven bool `json:"isPlayDriven,omitempty"`
	Work
}

// Work is what a command asks for: its type, and the payload of that type, which a result echoes
// (Result.Work). It names no job: the envelope does.
type Work struct {
	Type CommandType `json:"type"`
	// EncodeArtwork is set for CommandEncodeArtwork.
	EncodeArtwork *ArtworkBatch `json:"encodeArtwork,omitempty"`
	// ExtractSubtitle is set for CommandExtractSubtitle.
	ExtractSubtitle *ExtractSubtitle `json:"extractSubtitle,omitempty"`
	// ReadThrough is set for CommandReadThrough.
	ReadThrough *ReadThrough `json:"readThrough,omitempty"`
	// MakeChunks is set for CommandMakeChunks.
	MakeChunks *MakeChunks `json:"makeChunks,omitempty"`
	// List is set for CommandList.
	List *List `json:"list,omitempty"`
	// Examine is set for CommandExamine.
	Examine *Examine `json:"examine,omitempty"`
	// HashWhole is set for CommandHashWhole.
	HashWhole *HashWhole `json:"hashWhole,omitempty"`
	// MakeSystemVideos is set for CommandMakeSystemVideos.
	MakeSystemVideos *MakeSystemVideos `json:"makeSystemVideos,omitempty"`
	// FindRepeats is set for CommandFindRepeats.
	FindRepeats *FindRepeats `json:"findRepeats,omitempty"`
}

// Commands is the long-poll's answer: the commands after the one the node named, possibly none
// when the poll's time ran out.
type Commands struct {
	Commands []Command `json:"commands"`
	// DeviceReaderLimits are how many may read each of the node's devices at once, by its key, as the
	// core's settings say now: a change reaches the node with the next answer (D240 §5).
	DeviceReaderLimits map[string]int `json:"deviceReaderLimits"`
	// StoreTiers are how strongly each chunk store is kept, by its directory, as the core's players
	// stand now, every store not named KeepUnlikely: a change reaches the node with the next answer
	// (D309 §8). Nil from a core that sends none, which leaves the node's last.
	StoreTiers map[string]KeepTier `json:"storeTiers"`
	// Tuning is how the node behaves, as the core's settings say now: a change reaches the node with
	// the next answer.
	Tuning Tuning `json:"tuning"`
	// WatchFolders are the library folders on the node its watcher watches, as the core's libraries
	// stand now (HelloReply.WatchFolders): the core answers a waiting poll at once when they change.
	WatchFolders []string `json:"watchFolders"`
}

// MediaFile is the file of the node's media a command reads, and whether it reads one: the device it
// is on is the one the work holds a reader of (D240 §5). It answers by the work's type, so a type
// added without an answer here fails the exhaustive lint.
func (work Work) MediaFile() (BytesFile, bool) {
	switch work.Type {
	case CommandExtractSubtitle:
		return work.ExtractSubtitle.mediaFile()
	case CommandReadThrough:
		if work.ReadThrough != nil {
			return work.ReadThrough.Video, true
		}
	case CommandMakeChunks:
		return work.MakeChunks.mediaFile()
	case CommandHashWhole:
		return work.HashWhole.mediaFile()
	case CommandFindRepeats:
		return work.FindRepeats.mediaFile()
	case CommandList:
		return work.List.mediaFile()
	case CommandExamine:
		return work.Examine.mediaFile()
	case CommandEncodeArtwork, CommandMakeSystemVideos, CommandCancel, CommandReprioritize:
		// An artwork batch carries its images, and the system videos are made from nothing on a disk;
		// a cancel and a reprioritizing carry no work.
	}
	return BytesFile{}, false
}

// mediaFile is the video a subtitle is extracted from, or the sidecar converted; a fetched
// subtitle's conversion reads neither. Each payload's mediaFile answers none for a work that carries
// no payload.
func (command *ExtractSubtitle) mediaFile() (BytesFile, bool) {
	switch {
	case command == nil:
	case command.Video != nil:
		return *command.Video, true
	case command.Sidecar != nil:
		return *command.Sidecar, true
	}
	return BytesFile{}, false
}

// mediaFile is the video the chunks are made from.
func (command *MakeChunks) mediaFile() (BytesFile, bool) {
	if command == nil {
		return BytesFile{}, false
	}
	return command.Video, true
}

// mediaFile is the file hashed.
func (command *HashWhole) mediaFile() (BytesFile, bool) {
	if command == nil {
		return BytesFile{}, false
	}
	return command.File, true
}

// mediaFile is the set's first video: a season's episodes are on one disk, as a library folder's
// files mostly are.
func (command *FindRepeats) mediaFile() (BytesFile, bool) {
	if command == nil || len(command.Videos) == 0 {
		return BytesFile{}, false
	}
	return command.Videos[0].Video, true
}

// mediaFile is the folder listed.
func (command *List) mediaFile() (BytesFile, bool) {
	if command == nil {
		return BytesFile{}, false
	}
	return BytesFile{RootPath: command.RootPath, Path: []byte(command.Path)}, true
}

// mediaFile is the first file examined, by its raw bytes.
func (command *Examine) mediaFile() (BytesFile, bool) {
	if command == nil || len(command.Files) == 0 {
		return BytesFile{}, false
	}
	return BytesFile{RootPath: command.RootPath, Path: command.Files[0].Path.Bytes()}, true
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

// Slot is the kind of slot a node runs work in (cut-a §5.3): the encodes and remuxes its config
// says (Capacity), and one each of subtitle extraction and reading a video through, each of which
// reads a whole video, and of artwork re-encoding. The core holds a node to its slots, as the node does
// (D240 §5).
type Slot string

// The slots.
const (
	SlotEncode      Slot = "encode"
	SlotRemux       Slot = "remux"
	SlotExtract     Slot = "extract"
	SlotReadThrough Slot = "readThrough"
	SlotArtwork     Slot = "artwork"
)

// Slot is the slot the work runs in, or "" for work that holds none: a listing, an examining or a
// read of a video's index alone, which hold only a reader of their device, a scan, a cancel. It
// answers by the work's type, so a type added without an answer here fails the exhaustive lint.
func (work Work) Slot() Slot {
	switch work.Type {
	case CommandExtractSubtitle:
		return SlotExtract
	case CommandReadThrough:
		// A read of a video's index alone holds no slot.
		if work.ReadThrough != nil && !work.ReadThrough.IsFromIndexOnly() {
			return SlotReadThrough
		}
	case CommandHashWhole, CommandFindRepeats:
		// A whole-file hash reads a whole video too, as a read-through does (D116, D363 §2), and finding
		// repeats decodes the sound of a season's videos, one after another.
		return SlotReadThrough
	case CommandMakeChunks:
		return work.MakeChunks.slot()
	case CommandEncodeArtwork, CommandMakeSystemVideos:
		// The system videos are a few seconds' small encodes, made once, as an image's are.
		return SlotArtwork
	case CommandList, CommandExamine, CommandCancel, CommandReprioritize:
	}
	return ""
}

// slot is the encode slot for a run that encodes a quality, else the remux slot.
func (command *MakeChunks) slot() Slot {
	switch {
	case command == nil:
		return ""
	case command.Stream.Kind == StreamQuality:
		return SlotEncode
	default:
		return SlotRemux
	}
}

// Slots are every kind of slot a node runs work in.
func (Capacity) Slots() []Slot {
	return []Slot{SlotEncode, SlotRemux, SlotExtract, SlotReadThrough, SlotArtwork}
}

// SlotCount is how many of a slot's work run at once on a node of this capacity.
func (capacity Capacity) SlotCount(slot Slot) int {
	switch slot {
	case SlotEncode:
		return capacity.EncodeSlotCount
	case SlotRemux:
		return capacity.RemuxSlotCount
	case SlotExtract, SlotReadThrough, SlotArtwork:
	}
	return 1
}
