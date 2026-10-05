// SPDX-License-Identifier: GPL-3.0-or-later

package nodeproto

// CommandMakeChunks asks the node to make chunks of one stream of a film, on the film's grid
// (D247 §10; plans/chunks.md §2).
const CommandMakeChunks CommandType = "make_chunks"

// PathBytesChunks is the byte socket's route for a store's files:
// PathBytesChunks + "<file_id>/chunks-<stream>/<grid_id>/<name>", where name is init.mp4 or a
// chunk's <n>.m4s.
const PathBytesChunks = "/bytes/v1/chunks/"

// ChunkStreamKind is what a stream cut on the grid carries.
type ChunkStreamKind string

// The kinds of stream.
const (
	// StreamPicture is the source's picture, copied as it is.
	StreamPicture ChunkStreamKind = "picture"
	// StreamQuality is the picture encoded to H.264 at a bit rate and a height.
	StreamQuality ChunkStreamKind = "quality"
	// StreamSound is one audio track's stereo mix, standard or with the dialogue boost.
	StreamSound ChunkStreamKind = "sound"
)

// ChunkStream is one stream's recipe. A quality's fields are set for StreamQuality, a sound's for
// StreamSound, and none for StreamPicture.
type ChunkStream struct {
	Kind ChunkStreamKind `json:"kind"`
	// BitRateKbps and Height are a quality's: its average bit rate and its picture's height, the
	// width following the source's shape.
	BitRateKbps int `json:"bitRateKbps,omitempty"`
	Height      int `json:"height,omitempty"`
	// Sound is a sound's: the track and its mix, as a prepare makes them (D217 §3).
	Sound *PrepareAudio `json:"sound,omitempty"`
	// IsBoosted is whether a sound is the dialogue boost's mix.
	IsBoosted bool `json:"isBoosted,omitempty"`
}

// MakeChunks is one run: chunks FirstChunk to FirstChunk+len(BoundsMs)-2 of one stream, made in
// order, from the source named by identity (D100), into the store at StoreDir.
type MakeChunks struct {
	Video BytesFile `json:"video"`
	// VideoStreamIndex is the source picture's stream index, as ffprobe numbers every stream.
	VideoStreamIndex int `json:"videoStreamIndex"`
	// VideoCodec is the source picture's codec as the probe named it: a copied HEVC picture is
	// tagged hvc1, which Safari requires.
	VideoCodec string      `json:"videoCodec"`
	Stream     ChunkStream `json:"stream"`
	// StoreDir is "<file_id>/chunks-<stream>/<grid_id>" under the node's prepared directory, and
	// its byte path.
	StoreDir   string `json:"storeDir"`
	FirstChunk int    `json:"firstChunk"`
	// BoundsMs are the run's chunks' starts in the film's time, then the end of its last chunk.
	BoundsMs []int64 `json:"boundsMs"`
	// SoundLeadInMs is, for a sound run past the film's first chunk, the start of the chunk before
	// FirstChunk: the run starts its sound there and drops that chunk, which carries the encoder's
	// priming (slow-links.md §10). Zero otherwise.
	SoundLeadInMs int64 `json:"soundLeadInMs,omitempty"`
}

// MakeChunksReport is the node's word on how a run ended: its states are a prepare's.
type MakeChunksReport struct {
	State PrepareState `json:"state"`
	// LastChunk is the last chunk the run made; -1 when it made none.
	LastChunk   int         `json:"lastChunk"`
	Failure     string      `json:"failure,omitempty"`
	Refusal     string      `json:"refusal,omitempty"`
	RefusalCode RefusalCode `json:"refusalCode,omitempty"`
	Stall       *Stall      `json:"stall,omitempty"`
}

// StoreManifestName is the file in a store's directory that names it, written when the store is
// made: a store directory without one is evicted as a making that never finished.
const StoreManifestName = "store.json"

// StoreManifest names a store: what it holds, so the node's budget keeps it as it keeps a
// rendition.
type StoreManifest struct {
	StoreDir string      `json:"storeDir"`
	Video    BytesFile   `json:"video"`
	Stream   ChunkStream `json:"stream"`
}
