// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Stream is one thing a film is cut into on its grid (chunks.md §1): its picture copied, its
// picture encoded at one of the qualities, or one audio track's standard or boosted stereo mix.
type Stream struct {
	Kind nodeproto.ChunkStreamKind
	// Quality is a StreamQuality's bit rate and height.
	Quality qualities.Quality
	// AudioTrackIndex is a StreamSound's track: its place among the film's audio tracks.
	AudioTrackIndex int
	// IsBoosted is whether a StreamSound is the track's mix with the dialogue boost (D217 §3).
	IsBoosted bool
	// SoundKbps is a StreamSound's AAC bit rate: chunk_sound_kbps, part of its name so a change makes
	// a store of its own.
	SoundKbps int
	// IsUnlevelled is whether a StreamSound was offered before its film's loudness was measured, as a
	// film cut from its file's index is (D259 §2): it plays as it is, from a store of its own, so a
	// levelled chunk never follows an unlevelled one in a store, and a later playback gets the levelled.
	IsUnlevelled bool
}

// pictureStreamName is the name of the stream that is the source's picture, copied.
const pictureStreamName = "picture"

// Name is the stream's name in its playlist's path and its store's directory: "picture";
// "q<kbps>k<height>p" for a quality, such as q1200k480p; "sound<track>-<kbps>k" for a track's
// standard mix and "sound<track>-boost-<kbps>k" for its boosted one, such as sound0-boost-96k, each
// ending "-raw" when unlevelled.
func (stream Stream) Name() string {
	switch stream.Kind {
	case nodeproto.StreamQuality:
		return "q" + strconv.Itoa(stream.Quality.Kbps) + "k" + strconv.Itoa(stream.Quality.Height) + "p"
	case nodeproto.StreamSound:
		name := "sound" + strconv.Itoa(stream.AudioTrackIndex)
		if stream.IsBoosted {
			name += "-boost"
		}
		name += "-" + strconv.Itoa(stream.SoundKbps) + "k"
		if stream.IsUnlevelled {
			name += "-raw"
		}
		return name
	case nodeproto.StreamPicture:
	}
	return pictureStreamName
}

// streamNamePattern is the shape of every stream's name: the picture, a quality, or a sound.
var streamNamePattern = regexp.MustCompile(`^(?:picture|q([1-9]\d{0,5})k([1-9]\d{0,4})p|` +
	`sound(0|[1-9]\d{0,2})(-boost)?-([1-9]\d{1,2})k(-raw)?)$`)

// StreamNamed is the stream a name names, or false for a name not of a stream's shape. Whether the
// film has that stream, a quality in the setting or a track it has, is not its to say.
func StreamNamed(name string) (Stream, bool) {
	match := streamNamePattern.FindStringSubmatch(name)
	switch {
	case match == nil:
		return Stream{}, false
	case match[1] != "":
		kbps, kbpsErr := strconv.Atoi(match[1])
		height, heightErr := strconv.Atoi(match[2])
		if kbpsErr != nil || heightErr != nil {
			return Stream{}, false
		}
		return Stream{Kind: nodeproto.StreamQuality, Quality: qualities.Quality{Kbps: kbps, Height: height}}, true
	case match[3] != "":
		track, trackErr := strconv.Atoi(match[3])
		kbps, kbpsErr := strconv.Atoi(match[5])
		if trackErr != nil || kbpsErr != nil {
			return Stream{}, false
		}
		return Stream{
			Kind: nodeproto.StreamSound, AudioTrackIndex: track, IsBoosted: match[4] != "", SoundKbps: kbps,
			IsUnlevelled: match[6] != "",
		}, true
	}
	return Stream{Kind: nodeproto.StreamPicture}, true
}

// gridIDHexLength is how many hex characters of the hash of a grid name it: 64 bits, which two of
// one film's grids never share by chance.
const gridIDHexLength = 16

// ID names the grid: the first 16 hex characters of the SHA-256 of its starts and the film's length,
// written as "durationMs=<ms>;startsMs=<ms>,<ms>,...". A film whose grid changes, its keyframes read
// again or the segment length changed, has its chunks made into new stores.
func (grid Grid) ID() string {
	starts := make([]string, 0, len(grid.StartsMs))
	for _, start := range grid.StartsMs {
		starts = append(starts, strconv.FormatInt(start, 10))
	}
	written := "durationMs=" + strconv.FormatInt(grid.FilmDurationMs, 10) + ";startsMs=" + strings.Join(starts, ",")
	hash := sha256.Sum256([]byte(written))
	return hex.EncodeToString(hash[:])[:gridIDHexLength]
}

// StoreDir is the directory, under the node's prepared directory, of the store of one stream of a
// film on its grid: "<file_id>/chunks-<stream>/<grid_id>" (nodeproto.MakeChunks.StoreDir).
func StoreDir(fileID int64, stream Stream, gridID string) string {
	return fmt.Sprintf("%d/chunks-%s/%s", fileID, stream.Name(), gridID)
}

// The names of a store's files: its init, and each chunk's, five digits or more.
const initName = "init.mp4"

// chunkName is the name of chunk n's file in its store: 00007.m4s.
func chunkName(n int) string { return fmt.Sprintf("%05d.m4s", n) }

// storeFileNamePattern is the shape of a store's file a player may ask for: its init or a chunk.
var storeFileNamePattern = regexp.MustCompile(`^(?:init\.mp4|(\d{5,9})\.m4s)$`)
