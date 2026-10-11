// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"strconv"
	"strings"
)

// variant is one picture a master playlist offers: its stream's playlist, its peak and average bit
// rates with the sound's, the average zero when not known, and its picture's size, zero when not
// known.
type variant struct {
	uri                               string
	bandwidthBps, averageBandwidthBps int64
	width, height                     int
}

// soundMedia is one sound a master playlist offers, in the audio group every picture plays with.
type soundMedia struct {
	uri, name, language string
	// isDefault is the sound a player starts with: the playback's starting track's standard mix.
	isDefault bool
}

// masterPlaylist is the master playlist of a film cut into chunks (chunks.md §3): each sound a
// rendition of the group "audio", stereo, the default one also chosen automatically; then each
// picture, highest first, with its bit rates and size. CODECS is left out: our encodes' H.264
// profile and level and a copied picture's are not known for sure, and a CODECS naming only the
// sound would say a picture is sound alone (RFC 8216 §4.3.4.2).
func masterPlaylist(sounds []soundMedia, variants []variant) string {
	var text strings.Builder
	text.WriteString("#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-INDEPENDENT-SEGMENTS\n")
	for _, sound := range sounds {
		isDefault := "NO"
		if sound.isDefault {
			isDefault = "YES"
		}
		text.WriteString(`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="` + sound.name + `",LANGUAGE="` +
			sound.language + `",DEFAULT=` + isDefault + `,AUTOSELECT=` + isDefault + `,CHANNELS="2",URI="` +
			sound.uri + "\"\n")
	}
	for _, picture := range variants {
		text.WriteString("#EXT-X-STREAM-INF:BANDWIDTH=" + strconv.FormatInt(picture.bandwidthBps, 10))
		if picture.averageBandwidthBps > 0 {
			text.WriteString(",AVERAGE-BANDWIDTH=" + strconv.FormatInt(picture.averageBandwidthBps, 10))
		}
		if picture.width > 0 && picture.height > 0 {
			text.WriteString(",RESOLUTION=" + strconv.Itoa(picture.width) + "x" + strconv.Itoa(picture.height))
		}
		if len(sounds) > 0 {
			text.WriteString(`,AUDIO="audio"`)
		}
		text.WriteString("\n" + picture.uri + "\n")
	}
	return text.String()
}

// mediaPlaylist is the playlist of one stream of a film on its grid, the same for every stream: a
// VOD playlist of every chunk, each its exact length, after the store's init (chunks.md §3).
func mediaPlaylist(grid Grid) string {
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
		text.WriteString("#EXTINF:" + strconv.FormatFloat(float64(lengthMs)/msPerSecond, 'f', 3, 64) + ",\n" +
			chunkName(n) + "\n")
	}
	text.WriteString("#EXT-X-ENDLIST\n")
	return text.String()
}
