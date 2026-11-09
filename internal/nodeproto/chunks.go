// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

// CommandMakeChunks asks the node to make chunks of one stream of a film, on the film's grid
// (D247 §10; plans/chunks.md §2).
const CommandMakeChunks CommandType = "makeChunks"

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
	// StreamSoundCopied is one audio track as it is, copied, every channel (D368 §1).
	StreamSoundCopied ChunkStreamKind = "soundCopied"
)

// IsSound is whether the kind is a sound, made or copied: a run of either names its track (Sound) and
// starts a chunk early, which it drops (MakeChunks.LeadInMs).
func (kind ChunkStreamKind) IsSound() bool { return kind == StreamSound || kind == StreamSoundCopied }

// ChunkStream is one stream's recipe. A quality's fields are set for StreamQuality, a sound's for
// StreamSound, its track alone for StreamSoundCopied, and none for StreamPicture.
type ChunkStream struct {
	Kind ChunkStreamKind `json:"kind"`
	// BitRateKbps and Height are a quality's: its average bit rate and its picture's height, the
	// width following the source's shape.
	BitRateKbps int `json:"bitRateKbps,omitempty"`
	Height      int `json:"height,omitempty"`
	// Sound is a sound's: the track and its mix (D217 §3); a copied sound's track alone.
	Sound *SoundTrack `json:"sound,omitempty"`
	// IsBoosted is whether a sound is the dialogue boost's mix.
	IsBoosted bool `json:"isBoosted,omitempty"`
	// SoundKbps is a sound's AAC bit rate, chunkSoundKbps when the run was asked for.
	SoundKbps int `json:"soundKbps,omitempty"`
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
	// LeadInMs is, for a sound or a copied picture's run past the film's first chunk, the start of the
	// chunk before FirstChunk: the run starts there and drops that chunk. A sound's carries the
	// encoder's priming (slow-links.md §10), or, for a sound copied, what FFmpeg copies from before
	// its seek, so its first chunk starts where the run before it ended (D368 §1). A copied picture's
	// is read so FFmpeg times the pictures shown before the run's first keyframe and decoded after it,
	// as an open GOP has, as the film does: a run starting at that keyframe gave them wrong times and
	// left a hole at its join (docs/contributors/idiosyncrasies.md). Zero otherwise.
	LeadInMs int64 `json:"leadInMs,omitempty"`
	// PictureSeekMs is, for a copied picture, where FFmpeg is told to seek: past the run's first
	// keyframe (its lead-in's, when it has one), before the next, or, with
	// IsPictureSeekBeforeKeyframe, at or before it. FFmpeg lands
	// on the keyframe at or before a seek, and moves a seek into a Matroska file 130 ms earlier, so a
	// seek at the keyframe itself lands on the one before it and every chunk of the run is the one
	// before it (docs/contributors/idiosyncrasies.md).
	// Zero for other streams.
	PictureSeekMs int64 `json:"pictureSeekMs,omitempty"`
	// IsPictureSeekBeforeKeyframe says a copied picture's seek is at or before the run's first
	// keyframe, not past it: the film is MPEG-TS, which FFmpeg seeks to the frame at or before a
	// time and not to a keyframe, so a copy from past the keyframe would start at the next one
	// (docs/contributors/idiosyncrasies.md).
	IsPictureSeekBeforeKeyframe bool `json:"isPictureSeekBeforeKeyframe,omitempty"`
	// IsSoundInterleaved says a sound run's track is interleaved in super-blocks, as RealMedia
	// carries cook, ATRAC3, SIPR and RealAudio 28.8: FFmpeg's demuxer loses the super-block a seek
	// lands in, so the node seeks further before the run, and not at all from the film's start
	// (docs/contributors/idiosyncrasies.md).
	IsSoundInterleaved bool `json:"isSoundInterleaved,omitempty"`
	// NeedsPictureRetiming says a run of the picture, copied or encoded, rebuilds its display times
	// from the bitstream's picture order, as its probe says it must (ProbeVideo.NeedsRetiming), and
	// reads a little past its end, so the frames before its last bound are re-timed from the ones
	// after. False for sounds.
	NeedsPictureRetiming bool `json:"needsPictureRetiming,omitempty"`
}

// MakeChunksReport is the node's word on how a run ended.
type MakeChunksReport struct {
	State RunState `json:"state"`
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

// StoreManifest names a store: what it holds, so the node knows what its disk holds after a restart
// (D238 §2).
type StoreManifest struct {
	StoreDir string      `json:"storeDir"`
	Video    BytesFile   `json:"video"`
	Stream   ChunkStream `json:"stream"`
	// ChunkRecords is the version of the records its init and chunks carry, each its size and CRC-32C
	// beside it, written by the node as it puts them (D392 §4); zero for a store made before they were
	// recorded, which cannot be checked and is dropped at its first use.
	ChunkRecords int `json:"chunkRecords,omitempty"`
}
