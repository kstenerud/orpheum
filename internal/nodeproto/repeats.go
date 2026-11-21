// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

// CommandFindRepeats asks the node to fingerprint the sound of the opening and closing minutes of each
// of a set of videos the core names, a season's episodes, and say which stretch of each the others
// share: a measurement across files, so the set is the core's and the node never chooses it (D101).
// The core decides what a repeat is, an intro or the credits, and how sure it is (D82, D115).
const CommandFindRepeats CommandType = "findRepeats"

// FindRepeats is a set of videos to compare, and how much of each one's start and end is compared.
type FindRepeats struct {
	// Videos are the set, in the order the answer's are; each named by payload hash and checked
	// before FFmpeg reads it (D100).
	Videos []RepeatVideo `json:"videos"`
	// OpeningMs is how much of each video's start is fingerprinted, and ClosingMs of its end; a video
	// shorter than both together has its opening and closing overlap.
	OpeningMs int64 `json:"openingMs"`
	ClosingMs int64 `json:"closingMs"`
}

// RepeatVideo is one video of the set: its file, the audio track heard and its length.
type RepeatVideo struct {
	// FileID is the core's own id of the video, which its answer says again: the node does not read it.
	FileID int64     `json:"fileId"`
	Video  BytesFile `json:"video"`
	// AudioStreamIndex is the track's stream index, as ffprobe numbers every stream. It is mixed to
	// stereo before it is fingerprinted, whatever its channels: chromaprint refuses 5.1 (D115).
	AudioStreamIndex int `json:"audioStreamIndex"`
	// DurationMs is the video's length, as its probe says, where its closing minutes start from.
	DurationMs int64 `json:"durationMs"`
}

// Repeats is the node's answer: each video's repeats, in the command's order, or, for the whole set,
// why there are none.
type Repeats struct {
	Videos []VideoRepeats `json:"videos,omitempty"`
	// Refusal says why the node compared nothing: a node that does not fingerprint sound.
	Refusal string `json:"refusal,omitempty"`
	// Stall is set when a video's read stalled on its disk: nothing of the set is answered.
	Stall *Stall `json:"stall,omitempty"`
}

// VideoRepeats is what the comparison found of one video: the stretch of its opening and of its
// closing the set's others share, either nil when none does, or why it was not fingerprinted.
type VideoRepeats struct {
	// FileID is the video's, as the command named it.
	FileID  int64         `json:"fileId"`
	Opening *RepeatedSpan `json:"opening,omitempty"`
	Closing *RepeatedSpan `json:"closing,omitempty"`
	// Failure is FFmpeg's word that it could not fingerprint the video's sound: a fact about its
	// bytes, which reading again would not change.
	Failure string `json:"failure,omitempty"`
	// Refusal says why the node did not read it: a file that changed since the scan or is gone
	// (RefusalCode). It is compared with none of the others.
	Refusal     string      `json:"refusal,omitempty"`
	RefusalCode RefusalCode `json:"refusalCode,omitempty"`
}

// RepeatedSpan is a stretch of one video's sound that others of the set share.
type RepeatedSpan struct {
	// StartMs and EndMs are where it is in the video, from its start.
	StartMs int64 `json:"startMs"`
	EndMs   int64 `json:"endMs"`
	// HeardIn is how many of the set's other videos share it, and ComparedWith how many it was
	// compared with: those fingerprinted.
	HeardIn      int `json:"heardIn"`
	ComparedWith int `json:"comparedWith"`
}
