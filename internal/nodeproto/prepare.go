// SPDX-License-Identifier: GPL-3.0-or-later

package nodeproto

// CommandPrepare asks the node to prepare an HLS rendition of one of its files (cut-a §4, §6).
const CommandPrepare CommandType = "prepare"

// PathPrepare is where the node reports a prepare's progress and its end.
const PathPrepare = "/node/v1/prepare"

// PathBytesPrepared is the byte socket's route for a rendition's files:
// PathBytesPrepared + "<file_id>/<profile>/<run_id>/<name>" (D217 §5).
const PathBytesPrepared = "/bytes/v1/prepared/"

// The segment lengths a prepare may ask for, and the one the node makes when a command asks for
// none within them (D217 §3): the bounds and starting value of the core's setting
// hls_segment_length_seconds, defined once here for the core's registry and the node. Past ten
// seconds, switching to the dialogue boost's mix, which waits for the next segment, takes too long.
const (
	MinSegmentLengthSeconds      = 1
	MaxSegmentLengthSeconds      = 10
	StartingSegmentLengthSeconds = 2
)

// PrepareProfile is a rendition's recipe (cut-a §6).
type PrepareProfile string

// The profiles: the picture copied, or re-encoded to H.264 at about 8 Mb/s.
const (
	ProfileRemux    PrepareProfile = "remux"
	ProfileH264At8M PrepareProfile = "h264-8m"
)

// Prepare is one prepare job: the video named by identity (D100), the profile, the audio
// tracks to carry, and where under the node's prepared directory the rendition goes.
type Prepare struct {
	JobID   int64          `json:"jobId"`
	Video   BytesFile      `json:"video"`
	Profile PrepareProfile `json:"profile"`
	// RenditionDir is "<file_id>/<profile>/<run_id>", this making's directory under the node's prepared
	// directory and its byte path: a new run for each making, so no request reaches a copy an
	// earlier making left.
	RenditionDir string `json:"renditionDir"`
	// VideoStreamIndex is the source picture's stream index, as ffprobe numbers every stream: the
	// picture, never cover art that happens to come first.
	VideoStreamIndex int `json:"videoStreamIndex"`
	// VideoCodec is the source picture's codec as the probe named it: a copied HEVC picture is
	// tagged hvc1, which Safari requires.
	VideoCodec string `json:"videoCodec"`
	// SourceDurationMs is the source's length as the probe gave it, from which the node sets the
	// prepare's deadline; zero when unknown.
	SourceDurationMs int64 `json:"sourceDurationMs,omitempty"`
	// SegmentLengthSeconds is each HLS segment's target length (D217 §3).
	SegmentLengthSeconds int `json:"segmentLengthSeconds"`
	// AudioTracks lists the source's audio tracks, in order: every one is carried, so switching
	// language needs no new job (D217 §3).
	AudioTracks []PrepareAudio `json:"audioTracks"`
}

// PrepareAudio is one source audio track. A track with more than two channels has a centre and
// gets a boosted rendition beside its standard one; one without has no centre to raise.
type PrepareAudio struct {
	// AudioTrackIndex is the track's place among the source's audio tracks, as FFmpeg's 0:a:<index>.
	AudioTrackIndex int    `json:"audioTrackIndex"`
	Language        string `json:"language,omitempty"`
	ChannelCount    int    `json:"channelCount"`
	// CarriedGainDB is put in each of the track's mixes, so a player that cannot apply a gain itself, as
	// Safari's own HLS cannot, plays it levelled (D226 §4); 0 for a track not measured yet.
	CarriedGainDB float64 `json:"carriedGainDb,omitempty"`
}

// PrepareState is where a prepare is.
type PrepareState string

// The states a report names.
const (
	// PrepareWaiting is a prepare the node holds, waiting for a slot of its kind (cut-a §5.3):
	// sent when it starts to wait and at each status tick while it does, so the core, which
	// gives up on a node silent about a job, keeps waiting for it.
	PrepareWaiting PrepareState = "waiting"
	PrepareRunning PrepareState = "running"
	PrepareDone    PrepareState = "done"
	// PrepareFailed is FFmpeg failing on the source: Failure says how.
	PrepareFailed PrepareState = "failed"
	// PrepareRefused is a prepare the node did not try: Refusal says why.
	PrepareRefused PrepareState = "refused"
	// PrepareStalled is a prepare whose FFmpeg the node stopped because it stalled: Stall says
	// what was seen.
	PrepareStalled PrepareState = "stalled"
)

// IsEnd is whether a report in this state is the prepare's last: the node is done with it, made
// or not.
func (state PrepareState) IsEnd() bool {
	switch state {
	case PrepareDone, PrepareFailed, PrepareRefused, PrepareStalled:
		return true
	case PrepareWaiting, PrepareRunning:
	}
	return false
}

// RefusalCode names a refusal the core acts on, rather than only shows.
type RefusalCode string

// RefusedNoRoom is a prepare refused because the prepared directory's disk is past its limit and
// nothing can be evicted: the core waits for the node's status to show room (D231 §5).
const RefusedNoRoom RefusalCode = "no_room"

// PrepareReport is the node's word on a prepare: how far it is, or how it ended.
type PrepareReport struct {
	JobID int64        `json:"jobId"`
	State PrepareState `json:"state"`
	// PreparedThroughMs is how much of the source is prepared, in milliseconds of its time.
	PreparedThroughMs int64 `json:"preparedThroughMs"`
	// SizeBytes is the rendition's size once done.
	SizeBytes int64 `json:"sizeBytes,omitempty"`
	// Failure is FFmpeg's exit and the tail of its stderr: a fact about the source's bytes, which
	// preparing again would not change.
	Failure string `json:"failure,omitempty"`
	// Refusal says why the node did not try: a node that does not prepare, a file outside its roots
	// or whose payload hash changed since the scan, a disk with no room. It says nothing of the
	// bytes, so the prepare may be asked for again.
	Refusal string `json:"refusal,omitempty"`
	// RefusalCode names a refusal the core acts on: RefusedNoRoom. Empty for any other.
	RefusalCode RefusalCode `json:"refusalCode,omitempty"`
	// Stall is what the node saw of an FFmpeg it stopped because it stalled: it says nothing of
	// the source's bytes the first time.
	Stall *Stall `json:"stall,omitempty"`
	// VideoEncoder is what made the video once done: h264_qsv, h264_nvenc, x264, or copy for a
	// remux (D225 §3).
	VideoEncoder string `json:"videoEncoder,omitempty"`
	// RealtimeFactor is seconds of the source prepared per second of running, as FFmpeg last said, once
	// done; zero when it never said.
	RealtimeFactor float64 `json:"realtimeFactor,omitempty"`
}
