// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

// The chunk lengths a film may be cut at, and the one the node makes when a command asks for none
// within them: the bounds and starting value of the core's setting hls_segment_length_seconds,
// defined once here for the core's registry and the node. Past ten seconds, switching to the
// dialogue boost's mix, which waits for the next chunk, takes too long (D217 §3).
const (
	MinSegmentLengthSeconds      = 1
	MaxSegmentLengthSeconds      = 10
	StartingSegmentLengthSeconds = 2
)

// SoundTrack is one source audio track, as a run of its sound's chunks makes it. A track with more
// than two channels has a centre and gets a boosted mix beside its standard one; one without has no
// centre to raise (D217 §3).
type SoundTrack struct {
	// AudioTrackIndex is the track's place among the source's audio tracks, as FFmpeg's 0:a:<index>.
	AudioTrackIndex int    `json:"audioTrackIndex"`
	Language        string `json:"language,omitempty"`
	ChannelCount    int    `json:"channelCount"`
	// CarriedGainDB is put in each of the track's mixes, so a player that cannot apply a gain itself, as
	// Safari's own HLS cannot, plays it levelled (D226 §4); 0 for a track not measured yet.
	CarriedGainDB float64 `json:"carriedGainDb,omitempty"`
}

// RunState is how a run of work the node was given ended. Where one is while it waits or runs is
// its progress (Progress).
type RunState string

// The states a report names.
const (
	RunDone RunState = "done"
	// RunFailed is FFmpeg failing on the source: Failure says how.
	RunFailed RunState = "failed"
	// RunRefused is a run the node did not try: Refusal says why.
	RunRefused RunState = "refused"
	// RunStalled is a run whose FFmpeg the node stopped because it stalled: Stall says what was
	// seen.
	RunStalled RunState = "stalled"
)

// RefusalCode names a refusal the core acts on or says apart, rather than only shows.
type RefusalCode string

// RefusedChanged is work refused because its file's payload hash is not the one named: the file
// changed since the scan (D100). The core tries the job again on its back-off, as for any refusal,
// and says why; the scan that sees the change ends the old file's jobs (S74).
const RefusedChanged RefusalCode = "changed"

// RefusedGone is work refused because no file is at its path under its root: renamed or deleted
// since the scan, or its library folder gone. The core reads another place of the file, where it has
// one (D363 §8), until a scan sees it there again.
const RefusedGone RefusalCode = "gone"

// RefusedNoRoom is a run refused because the prepared directory's disk is past its limit and
// nothing can be evicted: the core waits for the node's status to show room (D231 §5).
const RefusedNoRoom RefusalCode = "noRoom"
