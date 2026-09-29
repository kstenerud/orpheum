// SPDX-License-Identifier: GPL-3.0-or-later

package nodeproto

// CommandMeasureLoudness asks the node to measure the loudness of a video's audio tracks (cut-a
// §4, D226 §1).
const CommandMeasureLoudness CommandType = "measure_loudness"

// PathLoudness is where the node posts a measure_loudness command's answer.
const PathLoudness = "/node/v1/loudness"

// MeasureLoudness is one video whose audio tracks to measure, from one read of the file.
type MeasureLoudness struct {
	// JobID names the core's job the answer belongs to.
	JobID int64 `json:"jobId"`
	// Video is the file, named by payload hash and checked before FFmpeg reads it (D100).
	Video BytesFile `json:"video"`
	// Tracks lists the tracks to measure.
	Tracks []LoudnessTrack `json:"tracks"`
}

// LoudnessTrack is one audio track to measure.
type LoudnessTrack struct {
	// StreamIndex is the track's stream index, as ffprobe numbers every stream.
	StreamIndex int `json:"streamIndex"`
	// ChannelCount is the track's channel count: one with more than two is measured as it is and as
	// the standard stereo mix a prepare makes of it; one with two or fewer, as stereo alone.
	ChannelCount int `json:"channelCount"`
}

// LoudnessResult is the node's answer: each track's measurements, or why there are none.
type LoudnessResult struct {
	JobID  int64           `json:"jobId"`
	Tracks []TrackLoudness `json:"tracks,omitempty"`
	// Failure is FFmpeg's word that it could not measure the file: a fact about its bytes, which
	// measuring again would not change.
	Failure string `json:"failure,omitempty"`
	// Refusal says why the node did not try: a source that changed since the scan, a node that
	// does not measure. It says nothing of the bytes.
	Refusal string `json:"refusal,omitempty"`
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
