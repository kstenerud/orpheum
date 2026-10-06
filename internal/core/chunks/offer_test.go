// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

func TestMasterPlaylist_OffersThePictureAndEachQualityBelowTheFilmsOwnWithEverySound(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	settingsService, isService := service.settings.(*settings.Service)
	if !isService {
		t.Fatal("the fixture's settings are not the settings service")
	}
	// 6000k at 1080p is above the film's four megabits and not below its height.
	settingstest.Set(t, settingsService, settings.ChunkQualities, `"6000k@1080p,3000k@720p,1200k@480p,700k@432p"`)
	playback := Playback{FileID: 1, IsSourcePictureDecodable: true, HasStartingAudio: true, StartingAudioTrackIndex: 1}
	text, err := service.MasterPlaylist(t.Context(), playback)
	if err != nil {
		t.Fatal(err)
	}
	// The picture's peak is its busiest second, 1.2 MB, as bits with the sound's; its average is its
	// seconds' over the film's twenty minutes, with the sound's.
	const want = "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-INDEPENDENT-SEGMENTS\n" +
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="sound0",LANGUAGE="en",DEFAULT=NO,AUTOSELECT=NO,` +
		`CHANNELS="2",URI="sound0/stream.m3u8"` + "\n" +
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="sound0-boost",LANGUAGE="en",DEFAULT=NO,AUTOSELECT=NO,` +
		`CHANNELS="2",URI="sound0-boost/stream.m3u8"` + "\n" +
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="sound1",LANGUAGE="und",DEFAULT=YES,AUTOSELECT=YES,` +
		`CHANNELS="2",URI="sound1/stream.m3u8"` + "\n" +
		`#EXT-X-STREAM-INF:BANDWIDTH=9792000,AVERAGE-BANDWIDTH=4196666,RESOLUTION=1920x1080,AUDIO="audio"` + "\n" +
		"picture/stream.m3u8\n" +
		`#EXT-X-STREAM-INF:BANDWIDTH=4692000,AVERAGE-BANDWIDTH=3192000,RESOLUTION=1280x720,AUDIO="audio"` + "\n" +
		"q3000k720p/stream.m3u8\n" +
		`#EXT-X-STREAM-INF:BANDWIDTH=1992000,AVERAGE-BANDWIDTH=1392000,RESOLUTION=854x480,AUDIO="audio"` + "\n" +
		"q1200k480p/stream.m3u8\n" +
		`#EXT-X-STREAM-INF:BANDWIDTH=1242000,AVERAGE-BANDWIDTH=892000,RESOLUTION=768x432,AUDIO="audio"` + "\n" +
		"q700k432p/stream.m3u8\n"
	if text != want {
		t.Errorf("the master playlist =\n%s\nwant\n%s", text, want)
	}
	// A player that cannot decode the film's picture is offered only the qualities.
	playback.IsSourcePictureDecodable = false
	if text, err = service.MasterPlaylist(t.Context(), playback); err != nil || strings.Contains(text, "picture/") {
		t.Errorf("a player that cannot decode the picture is offered\n%s, %v\nwant no picture copied", text, err)
	}
	if _, err = service.MediaPlaylist(t.Context(), playback, "picture"); !errors.Is(err, ErrNoStream) {
		t.Errorf("its picture's playlist = %v; want ErrNoStream", err)
	}
}

func TestMasterPlaylist_AFilmWithNoGridIsReadThroughForOne(t *testing.T) {
	t.Parallel()
	service, reader := chunksFixture(t, newFakeQueue(), fakeNode{})
	_, err := service.MasterPlaylist(t.Context(), Playback{FileID: 2, IsSourcePictureDecodable: true})
	if !errors.Is(err, ErrNoGrid) || !slices.Equal(reader.asked, []int64{2}) {
		t.Errorf("film 2's master playlist = %v, read through %v; want ErrNoGrid and film 2 read through",
			err, reader.asked)
	}
}

// A film whose read-through failed is tried again for a person's play, the read-through's to judge
// whether the failure came before the play; while it is read again it has no grid yet, and a
// failure that stands is ErrCannotBeCut (D248 §4).
func TestMasterPlaylist_APlayTriesAgainAReadThroughThatFailedBeforeIt(t *testing.T) {
	t.Parallel()
	service, reader := chunksFixture(t, newFakeQueue(), fakeNode{})
	failure := "Invalid data found when processing input"
	err := service.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.UpsertVideoTimeline(t.Context(), sqlcgen.UpsertVideoTimelineParams{
			PayloadHash: []byte{0x02}, KeyframesMsJSON: "[]", BytesInEachSecondJSON: "[]", Failure: &failure,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	reader.failedBeforePlay = map[int64]bool{2: true}
	playback := Playback{FileID: 2, IsSourcePictureDecodable: true, StartedAt: time.UnixMilli(10_000)}
	if _, err = service.MasterPlaylist(t.Context(), playback); !errors.Is(err, ErrNoGrid) ||
		!slices.Equal(reader.retried, []int64{2}) {
		t.Errorf("a play after film 2's read-through failed = %v, retried %v; want it read through again",
			err, reader.retried)
	}
	if _, err = service.MasterPlaylist(t.Context(), playback); !errors.Is(err, ErrCannotBeCut) {
		t.Errorf("film 2's read-through failing on the play's attempt = %v; want ErrCannotBeCut", err)
	}
}

func TestStoreFileOf_OffersOnlyTheStreamsAndFilesThePlaybackHas(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	playback := Playback{FileID: 1, IsSourcePictureDecodable: true, PositionMs: 95_000}
	gridID := mustGrid(t, service).ID()
	for _, refused := range []struct{ stream, file string }{
		{"q6000k1080p", "00000.m4s"},  // not in the setting
		{"q1200k481p", "00000.m4s"},   // nor this
		{"sound2", "00000.m4s"},       // a track the film does not have
		{"sound1-boost", "00000.m4s"}, // a stereo track's boost
		{"picture", "00120.m4s"},      // past the last chunk, 119
		{"picture", "stream.json"},
		{"../1", "00000.m4s"},
	} {
		if file, err := service.StoreFileOf(t.Context(), playback, refused.stream, refused.file); !errors.Is(err,
			ErrNoStream) {
			t.Errorf("%s/%s = %+v, %v; want ErrNoStream", refused.stream, refused.file, file, err)
		}
	}
	file, err := service.StoreFileOf(t.Context(), playback, "sound0-boost", "000042.m4s")
	if err != nil || file.NodeID != 1 || file.Target != nodeproto.PathBytesChunks+"1/chunks-sound0-boost/"+gridID+
		"/00042.m4s" || file.ContentType != "video/iso.segment" || file.runFrom != 42 {
		t.Errorf("chunk 42 of the boosted sound = %+v, %v; want it on node 1 as the node names it", file, err)
	}
	// A player changing to a stream asks first for its init: a run starts where the player is.
	file, err = service.StoreFileOf(t.Context(), playback, "picture", "init.mp4")
	if err != nil || !strings.HasSuffix(file.Target, "/init.mp4") || file.ContentType != "video/mp4" ||
		file.runFrom != 9 {
		t.Errorf("the picture's init = %+v, %v; want it, a run from chunk 9, where the player is", file, err)
	}
}
