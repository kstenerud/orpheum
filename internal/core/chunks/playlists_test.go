// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"testing"
)

func TestMasterPlaylist_ListsEachSoundThenEachPictureHighestFirst(t *testing.T) {
	t.Parallel()
	sounds := []soundMedia{
		{uri: "sound0/stream.m3u8", name: "sound0", language: "en", isDefault: true},
		{uri: "sound0-boost/stream.m3u8", name: "sound0-boost", language: "en"},
		{uri: "sound1/stream.m3u8", name: "sound1", language: "und"},
	}
	variants := []variant{
		{
			uri: "picture/stream.m3u8", bandwidthBps: 9_792_000, averageBandwidthBps: 4_192_000,
			width: 1920, height: 1080,
		},
		{
			uri: "q1200k480p/stream.m3u8", bandwidthBps: 1_992_000, averageBandwidthBps: 1_392_000,
			width: 854, height: 480,
		},
		// A picture whose average and size are not known says neither.
		{uri: "q700k432p/stream.m3u8", bandwidthBps: 1_242_000},
	}
	const want = "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-INDEPENDENT-SEGMENTS\n" +
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="sound0",LANGUAGE="en",DEFAULT=YES,AUTOSELECT=YES,` +
		`CHANNELS="2",URI="sound0/stream.m3u8"` + "\n" +
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="sound0-boost",LANGUAGE="en",DEFAULT=NO,AUTOSELECT=NO,` +
		`CHANNELS="2",URI="sound0-boost/stream.m3u8"` + "\n" +
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="sound1",LANGUAGE="und",DEFAULT=NO,AUTOSELECT=NO,` +
		`CHANNELS="2",URI="sound1/stream.m3u8"` + "\n" +
		`#EXT-X-STREAM-INF:BANDWIDTH=9792000,AVERAGE-BANDWIDTH=4192000,RESOLUTION=1920x1080,AUDIO="audio"` + "\n" +
		"picture/stream.m3u8\n" +
		`#EXT-X-STREAM-INF:BANDWIDTH=1992000,AVERAGE-BANDWIDTH=1392000,RESOLUTION=854x480,AUDIO="audio"` + "\n" +
		"q1200k480p/stream.m3u8\n" +
		`#EXT-X-STREAM-INF:BANDWIDTH=1242000,AUDIO="audio"` + "\n" +
		"q700k432p/stream.m3u8\n"
	if got := masterPlaylist(sounds, variants); got != want {
		t.Errorf("the master playlist =\n%s\nwant\n%s", got, want)
	}
	// A film with no sound lists its pictures in no audio group.
	const silent = "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-INDEPENDENT-SEGMENTS\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=1242000\nq700k432p/stream.m3u8\n"
	if got := masterPlaylist(nil, variants[2:]); got != silent {
		t.Errorf("a silent film's master playlist =\n%s\nwant\n%s", got, silent)
	}
}

func TestMediaPlaylist_ListsEveryChunkAtItsExactLengthAfterTheInit(t *testing.T) {
	t.Parallel()
	// The first keyframe is 80 ms in; the longest chunk is 10.01 s, which rounds the target up to 11.
	grid := Grid{StartsMs: []int64{80, 2080, 12_090}, FilmDurationMs: 14_500}
	const want = "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:11\n#EXT-X-PLAYLIST-TYPE:VOD\n" +
		"#EXT-X-INDEPENDENT-SEGMENTS\n#EXT-X-MAP:URI=\"init.mp4\"\n" +
		"#EXTINF:2.000,\n00000.m4s\n" +
		"#EXTINF:10.010,\n00001.m4s\n" +
		"#EXTINF:2.410,\n00002.m4s\n" +
		"#EXT-X-ENDLIST\n"
	if got := mediaPlaylist(grid); got != want {
		t.Errorf("the media playlist =\n%s\nwant\n%s", got, want)
	}
}
