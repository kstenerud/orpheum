// SPDX-License-Identifier: GPL-3.0-or-later

package nodeproto

// CommandReadThrough asks the node to read a video through once and say what only a read of the
// whole file tells: how loud its audio tracks are (cut-a §4, D226 §1), and its picture's timeline,
// the keyframes it can be cut at and the bytes each second of it takes (D247 §10). One FFmpeg reads
// the file once for all of it.
const CommandReadThrough CommandType = "read_through"

// ReadThrough is one video to read through, and what to learn from the read: the loudness of the
// tracks it names, its picture's timeline, or both. A read asks for at least one.
type ReadThrough struct {
	// Video is the file, named by payload hash and checked before FFmpeg reads it (D100).
	Video BytesFile `json:"video"`
	// LoudnessTracks are the audio tracks whose loudness to measure; none asks for no loudness.
	LoudnessTracks []LoudnessTrack `json:"loudnessTracks,omitempty"`
	// VideoTimeline, when set, asks for the picture's timeline; nil asks for none.
	VideoTimeline *VideoTimelineAsk `json:"videoTimeline,omitempty"`
	// DurationMs is the film's length, as its probe says: measuring loudness decodes every track, so
	// a long film takes long whatever the disk, and the node's watchdog allows for it. 0 when unknown.
	DurationMs int64 `json:"durationMs,omitempty"`
}

// LoudnessTrack is one audio track to measure.
type LoudnessTrack struct {
	// StreamIndex is the track's stream index, as ffprobe numbers every stream.
	StreamIndex int `json:"streamIndex"`
	// ChannelCount is the track's channel count: one with more than two is measured as it is and as
	// the standard stereo mix a prepare makes of it; one with two or fewer, as stereo alone.
	ChannelCount int `json:"channelCount"`
}

// VideoTimelineAsk asks for the timeline of one picture stream.
type VideoTimelineAsk struct {
	// VideoStreamIndex is the picture's stream index, as ffprobe numbers every stream.
	VideoStreamIndex int `json:"videoStreamIndex"`
	// IsFromIndexOnly asks for its keyframes from the file's own index alone, never by reading the
	// file through, for a player waiting on them: a file with none answers HasNoIndex at once
	// (D259 §2, D269 §2). False reads the file through.
	IsFromIndexOnly bool `json:"isFromIndexOnly,omitempty"`
}

// ReadThroughResult is the node's answer: what it learned of the video, each part it was asked
// for, or why there is nothing.
type ReadThroughResult struct {
	// Loudness is each asked track's measurements; empty when no loudness was asked for.
	Loudness []TrackLoudness `json:"loudness,omitempty"`
	// VideoTimeline is the picture's timeline; nil when none was asked for.
	VideoTimeline *VideoTimeline `json:"videoTimeline,omitempty"`
	// Failure is FFmpeg's word that it could not read the file through: a fact about its bytes,
	// which reading again would not change.
	Failure string `json:"failure,omitempty"`
	// Refusal says why the node did not try: a source that changed since the scan, a node that
	// does not read files through. It says nothing of the bytes.
	Refusal string `json:"refusal,omitempty"`
	// RefusalCode names a refusal the core says apart: RefusedChanged. Empty for any other.
	RefusalCode RefusalCode `json:"refusalCode,omitempty"`
	// Stall is what the node saw of an FFmpeg it stopped because it stalled: it says nothing of
	// the bytes the first time.
	Stall *Stall `json:"stall,omitempty"`
	// HasNoIndex answers a timeline asked for from the index alone of a file that has none: its
	// film is cut on time until it is read through (D269 §2).
	HasNoIndex bool `json:"hasNoIndex,omitempty"`
}

// TrackLoudness is one track's measurements.
type TrackLoudness struct {
	StreamIndex int `json:"streamIndex"`
	// Multichannel is the track as it is, measured only when it has more than two channels.
	Multichannel *Measurement `json:"multichannel,omitempty"`
	// Stereo is the track as a stereo device hears it: the standard stereo mix of a track with
	// more than two channels, else the track itself.
	Stereo Measurement `json:"stereo"`
}

// Measurement is one sound's loudness, by EBU R128 (ITU-R BS.1770).
type Measurement struct {
	// IntegratedLUFS is the programme's loudness, -70 for silence.
	IntegratedLUFS float64 `json:"integratedLufs"`
	// RangeLU is the loudness range: how far its quiet and loud parts are apart.
	RangeLU float64 `json:"rangeLu"`
	// TruePeakDBTP is the highest true peak; absent for silence, which has none.
	TruePeakDBTP *float64 `json:"truePeakDbtp,omitempty"`
}

// VideoTimeline is what a picture's packets say along the film, in the film's own time (its first
// packet's time is the film's start, as FFmpeg reads it): where it can be cut, and how many bytes
// each stretch of it takes.
type VideoTimeline struct {
	// KeyframesMs are the times of its keyframes, in milliseconds, ascending, each once: the only
	// places a copy of the picture can begin (D247 §10).
	KeyframesMs []int64 `json:"keyframesMs"`
	// BytesInEachSecond are the picture's bytes in each whole second of the film, by the second
	// they are shown in: entry n is second n. Its peaks, not its average, say how fast a link must
	// be to play it without stalling.
	BytesInEachSecond []int64 `json:"bytesInEachSecond"`
}
