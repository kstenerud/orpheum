// SPDX-License-Identifier: GPL-3.0-or-later

package nodeproto

// CommandPrepare asks the node to prepare an HLS rendition of one of its files (cut-a §4, §6).
const CommandPrepare CommandType = "prepare"

// PathPrepare is where the node reports a prepare's progress and its end.
const PathPrepare = "/node/v1/prepare"

// PathBytesPrepared is the byte socket's route for a rendition's files:
// PathBytesPrepared + "<file_id>/<profile>/<name>" (D217 §5).
const PathBytesPrepared = "/bytes/v1/prepared/"

// PrepareProfile is a rendition's recipe (cut-a §6).
type PrepareProfile string

// The profiles: the picture copied, or re-encoded to H.264 at about 8 Mb/s.
const (
	ProfileRemux    PrepareProfile = "remux"
	ProfileH264At8M PrepareProfile = "h264-8m"
)

// Prepare is one prepare job: the source named by identity (D100), the profile, the audio
// tracks to carry, and where under the node's prepared directory the rendition goes.
type Prepare struct {
	Job     int64          `json:"job"`
	Source  BytesFile      `json:"source"`
	Profile PrepareProfile `json:"profile"`
	// Dir is "<file_id>/<profile>", the rendition's directory under the node's prepared
	// directory and its byte path.
	Dir string `json:"dir"`
	// VideoCodec is the source picture's codec as the probe named it: a copied HEVC picture is
	// tagged hvc1, which Safari requires.
	VideoCodec string `json:"videoCodec"`
	// Audio lists the source's audio tracks, in order: every one is carried, so switching
	// language needs no new job (D217 §3).
	Audio []PrepareAudio `json:"audio"`
}

// PrepareAudio is one source audio track. A track with more than two channels has a centre and
// gets a boosted rendition beside its standard one; one without has no centre to raise.
type PrepareAudio struct {
	// Index is the track's place among the source's audio tracks, as FFmpeg's 0:a:<index>.
	Index    int    `json:"index"`
	Language string `json:"language,omitempty"`
	Channels int    `json:"channels"`
}

// PrepareState is where a prepare is.
type PrepareState string

// The states a report names.
const (
	PrepareRunning PrepareState = "running"
	PrepareDone    PrepareState = "done"
	PrepareFailed  PrepareState = "failed"
)

// PrepareReport is the node's word on a prepare: how far it is, or how it ended.
type PrepareReport struct {
	Job   int64        `json:"job"`
	State PrepareState `json:"state"`
	// DoneMs is how much of the source is prepared, in milliseconds of its time.
	DoneMs int64 `json:"doneMs"`
	// Bytes is the rendition's size once done.
	Bytes int64 `json:"bytes,omitempty"`
	// Error is FFmpeg's exit and the tail of its stderr, or why the node refused.
	Error string `json:"error,omitempty"`
}
