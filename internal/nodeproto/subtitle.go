// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

// CommandExtractSubtitle asks the node to turn text subtitles into ASS and WebVTT (cut-a §4,
// D217 §6): the text tracks of a video, every one asked for from one read of it (D340), or one
// subtitle file or fetched text.
const CommandExtractSubtitle CommandType = "extractSubtitle"

// ExtractSubtitle is what to extract: text tracks inside a video, a file beside it, or a subtitle
// fetched on request. Exactly one of Video, Sidecar and FetchedSubRip is set.
type ExtractSubtitle struct {
	// Video is the file that carries the tracks, named by payload hash and checked before FFmpeg
	// reads it (D100).
	Video *BytesFile `json:"video,omitempty"`
	// StreamIndexes are, with Video, the tracks to extract from one read of it, at least one, each
	// by its stream index as ffprobe numbers every stream (D340).
	StreamIndexes []int `json:"streamIndexes,omitempty"`
	// Sidecar is a subtitle file beside the media, converted whole, named by payload hash as a
	// video is and checked the same way (D100).
	Sidecar *BytesFile `json:"sidecar,omitempty"`
	// FetchedSubRip is the text of a subtitle fetched on request, SubRip as the provider gave it,
	// converted here because hostile input is parsed on the node (D224 §5).
	FetchedSubRip []byte `json:"fetchedSubRip,omitempty"`
	// FetchedSha256 is, with FetchedSubRip, the hex SHA-256 of the fetched subtitle's body as the
	// core keeps it, by which the core keeps its text: what an answer echoes in FetchedSubRip's
	// place (Result.Command).
	FetchedSha256 string `json:"fetchedSha256,omitempty"`
}

// SubtitleText is the node's answer: the subtitle as ASS, for JASSUB, and as WebVTT, for the
// cast receiver (cut-a §6), or why neither was made. A video's answer carries each track's text, or
// its failure, in Tracks; a refusal or a stall is the whole command's (D340).
type SubtitleText struct {
	ASS []byte `json:"ass,omitempty"`
	VTT []byte `json:"vtt,omitempty"`
	// Failure is the converter's word that it could not make the text: a fact about the source's
	// bytes, such as a track FFmpeg cannot convert, which asking again would not change.
	Failure string `json:"failure,omitempty"`
	// Tracks are, for a video, every track the command named (ExtractSubtitle.StreamIndexes), each
	// with its own text or its own failure.
	Tracks []TrackText `json:"tracks,omitempty"`
	// Refusal says why the node did not try: a node that does not convert subtitles, a source
	// outside its roots or changed since the scan. It says nothing of the bytes.
	Refusal string `json:"refusal,omitempty"`
	// RefusalCode names a refusal the core says apart: RefusedChanged. Empty for any other.
	RefusalCode RefusalCode `json:"refusalCode,omitempty"`
	// Stall is what the node saw of an FFmpeg it stopped because it stalled: it says nothing of
	// the bytes the first time.
	Stall *Stall `json:"stall,omitempty"`
}

// TrackText is one track of a video as ASS and WebVTT, or the converter's word that this track
// could not be made, as SubtitleText.Failure is a whole subtitle's.
type TrackText struct {
	// StreamIndex is the track's, as the command named it.
	StreamIndex int    `json:"streamIndex"`
	ASS         []byte `json:"ass,omitempty"`
	VTT         []byte `json:"vtt,omitempty"`
	Failure     string `json:"failure,omitempty"`
}
