// SPDX-License-Identifier: GPL-3.0-or-later

package nodeproto

// CommandExtractSubtitle asks the node to turn one text subtitle into ASS and WebVTT (cut-a §4,
// D217 §6).
const CommandExtractSubtitle CommandType = "extract_subtitle"

// PathSubtitle is where the node posts an extract_subtitle command's answer.
const PathSubtitle = "/node/v1/subtitle"

// ExtractSubtitle is one subtitle to extract: a track inside a video, or a file beside it.
// Exactly one of Video and Sidecar is set.
type ExtractSubtitle struct {
	// Job names the core's job the answer belongs to.
	Job int64 `json:"job"`
	// Video is the file that carries the track, named by payload hash and checked before FFmpeg
	// reads it (D100).
	Video *BytesFile `json:"video,omitempty"`
	// StreamIndex is the track's stream index in Video, as ffprobe numbers every stream.
	StreamIndex int `json:"streamIndex,omitempty"`
	// Sidecar is a subtitle file beside the media. A sidecar has no payload hash, so its Hash is
	// empty and the answer's SourceHash says which bytes were converted.
	Sidecar *BytesFile `json:"sidecar,omitempty"`
}

// SubtitleText is the node's answer: the subtitle as ASS, for JASSUB, and as WebVTT, for the
// cast receiver (cut-a §6), or why neither was made.
type SubtitleText struct {
	Job int64  `json:"job"`
	ASS []byte `json:"ass,omitempty"`
	VTT []byte `json:"vtt,omitempty"`
	// SourceHash is the SHA-256, in hex, of the sidecar file as the node read it; empty for a
	// track in a video, which the command named by payload hash.
	SourceHash string `json:"sourceHash,omitempty"`
	// Refused says why nothing was made: a source that changed, a track FFmpeg cannot convert.
	Refused string `json:"refused,omitempty"`
}
