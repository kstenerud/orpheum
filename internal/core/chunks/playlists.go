// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"slices"
	"strconv"
	"strings"
)

// variant is one picture a master playlist offers: its stream's playlist, its peak and average bit
// rates with the sound's, the average zero when not known, its picture's size, zero when not known,
// and the audio group it plays.
type variant struct {
	uri                               string
	bandwidthBps, averageBandwidthBps int64
	width, height                     int
	soundGroup                        string
}

// soundGroup is one audio group a master playlist offers: its id and its sounds.
type soundGroup struct {
	id     string
	sounds []soundMedia
}

// soundMedia is one sound a master playlist offers, in an audio group.
type soundMedia struct {
	uri, name, language string
	// channels is how many channels it plays.
	channels int
	// isDefault is the sound a player starts with: the playback's starting track's standard mix.
	isDefault bool
}

// masterPlaylist is the master playlist of a film cut into chunks (chunks.md §3): each sound a
// rendition of its group, with its channels, the default one also chosen automatically; then each
// picture, highest first, with its bit rates and size, naming the group it plays (D368 §3). CODECS
// is left out: our encodes' H.264 profile and level and a copied picture's are not known for sure, and
// a CODECS naming only the sound would say a picture is sound alone (RFC 8216 §4.3.4.2).
func masterPlaylist(groups []soundGroup, variants []variant) string {
	var text strings.Builder
	text.WriteString("#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-INDEPENDENT-SEGMENTS\n")
	for _, group := range groups {
		for _, sound := range group.sounds {
			isDefault := "NO"
			if sound.isDefault {
				isDefault = "YES"
			}
			text.WriteString(`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="` + group.id + `",NAME="` + sound.name +
				`",LANGUAGE="` + sound.language + `",DEFAULT=` + isDefault + `,AUTOSELECT=` + isDefault +
				`,CHANNELS="` + strconv.Itoa(sound.channels) + `",URI="` + sound.uri + "\"\n")
		}
	}
	hasSound := slices.ContainsFunc(groups, func(group soundGroup) bool { return len(group.sounds) > 0 })
	for _, picture := range variants {
		text.WriteString("#EXT-X-STREAM-INF:BANDWIDTH=" + strconv.FormatInt(picture.bandwidthBps, 10))
		if picture.averageBandwidthBps > 0 {
			text.WriteString(",AVERAGE-BANDWIDTH=" + strconv.FormatInt(picture.averageBandwidthBps, 10))
		}
		if picture.width > 0 && picture.height > 0 {
			text.WriteString(",RESOLUTION=" + strconv.Itoa(picture.width) + "x" + strconv.Itoa(picture.height))
		}
		if hasSound {
			text.WriteString(`,AUDIO="` + picture.soundGroup + `"`)
		}
		text.WriteString("\n" + picture.uri + "\n")
	}
	return text.String()
}

// mediaPlaylist is the playlist of one stream of a film on its grid, the same for every stream: a
// VOD playlist of every chunk, each its exact length, after the store's init (chunks.md §3). Given
// each chunk's bytes (chunkBytesOf), each chunk says its bit rate in an EXT-X-BITRATE before it, so
// a player knows a heavy chunk before it asks for it (D464 §1).
func mediaPlaylist(grid Grid, chunkBytes []int64) string {
	var longestMs int64
	for n := range grid.StartsMs {
		longestMs = max(longestMs, grid.endMs(n)-grid.StartsMs[n])
	}
	const msPerSecond = 1000
	targetSeconds := (longestMs + msPerSecond - 1) / msPerSecond
	var text strings.Builder
	text.WriteString("#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:" + strconv.FormatInt(targetSeconds, 10) +
		"\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-INDEPENDENT-SEGMENTS\n#EXT-X-MAP:URI=\"" + initName + "\"\n")
	for n := range grid.StartsMs {
		lengthMs := grid.endMs(n) - grid.StartsMs[n]
		if n < len(chunkBytes) && lengthMs > 0 {
			text.WriteString("#EXT-X-BITRATE:" + strconv.FormatInt(kbpsOf(chunkBytes[n], lengthMs), 10) + "\n")
		}
		text.WriteString("#EXTINF:" + strconv.FormatFloat(float64(lengthMs)/msPerSecond, 'f', 3, 64) + ",\n" +
			chunkName(n) + "\n")
	}
	text.WriteString("#EXT-X-ENDLIST\n")
	return text.String()
}

// kbpsOf is the bit rate of bytes over lengthMs, in kilobits a second, rounded up: a chunk's
// EXT-X-BITRATE, which a player sizes the chunk by, so never below its bytes.
func kbpsOf(bytes, lengthMs int64) int64 {
	const bitsPerByte = 8
	// bits per millisecond is kilobits per second.
	return (bytes*bitsPerByte + lengthMs - 1) / lengthMs
}

// chunkBytesOf is the bytes of each chunk of a grid, from its picture's bytesInEachSecond: nil
// where they are not known.
func chunkBytesOf(grid Grid, bytesInEachSecond []int64) []int64 {
	if len(bytesInEachSecond) == 0 {
		return nil
	}
	before := bytesBeforeOf(bytesInEachSecond)
	bytes := make([]int64, len(grid.StartsMs))
	for n := range grid.StartsMs {
		bytes[n] = before(grid.endMs(n)) - before(grid.StartsMs[n])
	}
	return bytes
}
