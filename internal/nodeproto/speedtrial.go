// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

// SpeedTrial is one of a node's start-up trials of how fast it makes a quality of a film of one kind
// (design/plans/start-quality-by-node-speed.md §1): a clip of Codec at BitDepth bits and Height lines,
// made on the node by its own FFmpeg, made into the quality MadeKbps at MadeHeight with the arguments
// a run of chunks uses, decoded on the card first where a run would be and on the CPU where that
// failed. Its status carries every trial once all have ended (Status.SpeedTrials).
type SpeedTrial struct {
	// Codec is the clip's picture's codec, by the name FFmpeg's probe gives it: h264, hevc, vp9, av1,
	// mpeg2video, mpeg4.
	Codec    string `json:"codec"`
	BitDepth int    `json:"bitDepth"`
	Height   int    `json:"height"`
	// MadeKbps and MadeHeight are the quality the clip was made into: the top of its height's, or the
	// lowest.
	MadeKbps   int `json:"madeKbps"`
	MadeHeight int `json:"madeHeight"`
	// SpeedPercent is how fast the quality was made, the clip's length over the run's whole time, in
	// percent of real time; zero when nothing was measured.
	SpeedPercent int `json:"speedPercent,omitempty"`
	// DecodedOn is where the clip's picture was decoded in the run measured: "gpu" or "cpu".
	DecodedOn string `json:"decodedOn,omitempty"`
	// GPUDecodeFailure is FFmpeg's words when decoding on the card failed and the trial was made again
	// on the CPU, as a run of chunks is (D568); empty when it was not tried on the card or worked.
	GPUDecodeFailure string `json:"gpuDecodeFailure,omitempty"`
	// NotTried says why the kind was not tried: this FFmpeg has no encoder to make its clip.
	NotTried string `json:"notTried,omitempty"`
	// Failure is why the trial measured nothing although it was tried, in FFmpeg's words.
	Failure string `json:"failure,omitempty"`
}
