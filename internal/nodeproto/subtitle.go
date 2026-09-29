// SPDX-License-Identifier: GPL-3.0-or-later

package nodeproto

// CommandExtractSubtitle asks the node to turn one text subtitle into ASS and WebVTT (cut-a §4,
// D217 §6).
const CommandExtractSubtitle CommandType = "extract_subtitle"

// PathSubtitle is where the node posts an extract_subtitle command's answer.
const PathSubtitle = "/node/v1/subtitle"

// ExtractSubtitle is one subtitle to extract: a track inside a video, a file beside it, or a
// subtitle fetched on request. Exactly one of Video, Sidecar and FetchedSubRip is set.
type ExtractSubtitle struct {
	// JobID names the core's job the answer belongs to.
	JobID int64 `json:"jobId"`
	// Video is the file that carries the track, named by payload hash and checked before FFmpeg
	// reads it (D100).
	Video *BytesFile `json:"video,omitempty"`
	// StreamIndex is the track's stream index in Video, as ffprobe numbers every stream.
	StreamIndex int `json:"streamIndex,omitempty"`
	// Sidecar is a subtitle file beside the media, converted whole, named by payload hash as a
	// video is and checked the same way (D100).
	Sidecar *BytesFile `json:"sidecar,omitempty"`
	// FetchedSubRip is the text of a subtitle fetched on request, SubRip as the provider gave it,
	// converted here because hostile input is parsed on the node (D224 §5).
	FetchedSubRip []byte `json:"fetchedSubRip,omitempty"`
}

// SubtitleText is the node's answer: the subtitle as ASS, for JASSUB, and as WebVTT, for the
// cast receiver (cut-a §6), or why neither was made.
type SubtitleText struct {
	JobID int64  `json:"jobId"`
	ASS   []byte `json:"ass,omitempty"`
	VTT   []byte `json:"vtt,omitempty"`
	// Refusal says why nothing was made: a source that changed, a track FFmpeg cannot convert.
	Refusal string `json:"refusal,omitempty"`
}
