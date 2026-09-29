// SPDX-License-Identifier: GPL-3.0-or-later

package nodeproto

// CommandPrepare asks the node to prepare an HLS rendition of one of its files (cut-a §4, §6).
const CommandPrepare CommandType = "prepare"

// PathPrepare is where the node reports a prepare's progress and its end.
const PathPrepare = "/node/v1/prepare"

// PathBytesPrepared is the byte socket's route for a rendition's files:
// PathBytesPrepared + "<file_id>/<profile>/<run_id>/<name>" (D217 §5).
const PathBytesPrepared = "/bytes/v1/prepared/"

// A node's prepare slots (cut-a §5.3): one encode, which the processor bounds, and two remuxes,
// which the disk does. The core runs as many prepares at once as these add up to, and the node
// holds any beyond its slots, saying so (PrepareWaiting).
const (
	EncodeSlots = 1
	RemuxSlots  = 2
)

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
	JobID   int64          `json:"jobId"`
	Source  BytesFile      `json:"source"`
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
	// SegmentLengthSeconds is each HLS segment's target length (D217 §3).
	SegmentLengthSeconds int `json:"segmentLengthSeconds"`
	// Audio lists the source's audio tracks, in order: every one is carried, so switching
	// language needs no new job (D217 §3).
	Audio []PrepareAudio `json:"audio"`
}

// PrepareAudio is one source audio track. A track with more than two channels has a centre and
// gets a boosted rendition beside its standard one; one without has no centre to raise.
type PrepareAudio struct {
	// AudioTrackIndex is the track's place among the source's audio tracks, as FFmpeg's 0:a:<index>.
	AudioTrackIndex int    `json:"audioTrackIndex"`
	LanguageTag     string `json:"languageTag,omitempty"`
	ChannelCount    int    `json:"channelCount"`
	// GainDB is put in each of the track's mixes, so a player that cannot apply a gain itself, as
	// Safari's own HLS cannot, plays it levelled (D226 §4); 0 for a track not measured yet.
	GainDB float64 `json:"gainDb,omitempty"`
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
	PrepareFailed  PrepareState = "failed"
)

// PrepareReport is the node's word on a prepare: how far it is, or how it ended.
type PrepareReport struct {
	JobID int64        `json:"jobId"`
	State PrepareState `json:"state"`
	// PreparedThroughMs is how much of the source is prepared, in milliseconds of its time.
	PreparedThroughMs int64 `json:"preparedThroughMs"`
	// SizeBytes is the rendition's size once done.
	SizeBytes int64 `json:"sizeBytes,omitempty"`
	// Failure is FFmpeg's exit and the tail of its stderr, or why the node refused.
	Failure string `json:"failure,omitempty"`
	// VideoEncoder is what made the video once done: h264_qsv, h264_nvenc, x264, or copy for a
	// remux (D225 §3).
	VideoEncoder string `json:"videoEncoder,omitempty"`
	// RealtimeFactor is seconds of the source prepared per second of running, as FFmpeg last said, once
	// done; zero when it never said.
	RealtimeFactor float64 `json:"realtimeFactor,omitempty"`
}
