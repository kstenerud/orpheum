// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

// CommandPrepare asks the node to prepare an HLS rendition of one of its files (cut-a §4, §6).
const CommandPrepare CommandType = "prepare"

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
	ProfileRemux PrepareProfile = "remux"
	ProfileH264  PrepareProfile = "h264"
)

// Prepare is one prepare job: the video named by identity (D100), the profile, the audio
// tracks to carry, and where under the node's prepared directory the rendition goes.
type Prepare struct {
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
	// VideoKbps is an encode's average rate for its picture (D256): set for h264, by the source
	// picture's height; zero, for a node or a command from before it, is the profile's 8 Mb/s.
	VideoKbps int `json:"videoKbps,omitempty"`
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

// PrepareState is how a prepare ended. Where one is while it waits or runs is its progress
// (Progress).
type PrepareState string

// The states a report names.
const (
	PrepareDone PrepareState = "done"
	// PrepareFailed is FFmpeg failing on the source: Failure says how.
	PrepareFailed PrepareState = "failed"
	// PrepareRefused is a prepare the node did not try: Refusal says why.
	PrepareRefused PrepareState = "refused"
	// PrepareStalled is a prepare whose FFmpeg the node stopped because it stalled: Stall says
	// what was seen.
	PrepareStalled PrepareState = "stalled"
)

// RefusalCode names a refusal the core acts on or says apart, rather than only shows.
type RefusalCode string

// RefusedChanged is work refused because its file's payload hash is not the one named: the file
// changed since the scan (D100). The core tries the job again on its back-off, as for any refusal,
// and says why; the scan that sees the change ends the old file's jobs (S74).
const RefusedChanged RefusalCode = "changed"

// RefusedNoRoom is a prepare refused because the prepared directory's disk is past its limit and
// nothing can be evicted: the core waits for the node's status to show room (D231 §5).
const RefusedNoRoom RefusalCode = "no_room"

// PrepareReport is the node's word on how a prepare ended.
type PrepareReport struct {
	State PrepareState `json:"state"`
	// SizeBytes is the rendition's size once done.
	SizeBytes int64 `json:"sizeBytes,omitempty"`
	// Failure is FFmpeg's exit and the tail of its stderr: a fact about the source's bytes, which
	// preparing again would not change.
	Failure string `json:"failure,omitempty"`
	// Refusal says why the node did not try: a node that does not prepare, a file outside its roots
	// or whose payload hash changed since the scan, a disk with no room. It says nothing of the
	// bytes, so the prepare may be asked for again.
	Refusal string `json:"refusal,omitempty"`
	// RefusalCode names a refusal the core acts on or says apart: RefusedNoRoom, RefusedChanged.
	// Empty for any other.
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

// ManifestName is the file in a rendition's run directory that holds its manifest. A run directory
// without one is a making that never finished.
const ManifestName = "manifest.json"

// RenditionManifest is what a node writes beside a rendition once it is made, so that, holding no
// database, it knows what its disk holds after a restart (D238 §2), and the core can rebuild its
// index from it: the command that made it, which names the source by identity, the profile, the
// run, the segment length and the audio tracks with their gains and languages, and how it was made.
type RenditionManifest struct {
	Prepare Prepare `json:"prepare"`
	// MadeAtMs is when the making ended, by the node's clock, in Unix milliseconds.
	MadeAtMs       int64   `json:"madeAtMs"`
	SizeBytes      int64   `json:"sizeBytes"`
	VideoEncoder   string  `json:"videoEncoder"`
	RealtimeFactor float64 `json:"realtimeFactor,omitempty"`
}
