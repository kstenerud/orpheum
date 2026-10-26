// SPDX-License-Identifier: AGPL-3.0-or-later

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
	text, _, err := service.MasterPlaylist(t.Context(), playback)
	if err != nil {
		t.Fatal(err)
	}
	// The picture's peak is its busiest second, 1.2 MB, as bits with the sound's; its average is its
	// seconds' over the film's twenty minutes, with the sound's.
	const want = "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-INDEPENDENT-SEGMENTS\n" +
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="sound0-96k",LANGUAGE="en",DEFAULT=NO,AUTOSELECT=NO,` +
		`CHANNELS="2",URI="sound0-96k/stream.m3u8"` + "\n" +
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="sound0-boost-96k",LANGUAGE="en",DEFAULT=NO,AUTOSELECT=NO,` +
		`CHANNELS="2",URI="sound0-boost-96k/stream.m3u8"` + "\n" +
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="sound1-96k",LANGUAGE="und",DEFAULT=YES,AUTOSELECT=YES,` +
		`CHANNELS="2",URI="sound1-96k/stream.m3u8"` + "\n" +
		`#EXT-X-STREAM-INF:BANDWIDTH=9696000,AVERAGE-BANDWIDTH=4100666,RESOLUTION=1920x1080,AUDIO="audio"` + "\n" +
		"picture/stream.m3u8\n" +
		`#EXT-X-STREAM-INF:BANDWIDTH=4596000,AVERAGE-BANDWIDTH=3096000,RESOLUTION=1280x720,AUDIO="audio"` + "\n" +
		"q3000k720p/stream.m3u8\n" +
		`#EXT-X-STREAM-INF:BANDWIDTH=1896000,AVERAGE-BANDWIDTH=1296000,RESOLUTION=854x480,AUDIO="audio"` + "\n" +
		"q1200k480p/stream.m3u8\n" +
		`#EXT-X-STREAM-INF:BANDWIDTH=1146000,AVERAGE-BANDWIDTH=796000,RESOLUTION=768x432,AUDIO="audio"` + "\n" +
		"q700k432p/stream.m3u8\n"
	if text != want {
		t.Errorf("the master playlist =\n%s\nwant\n%s", text, want)
	}
	// A player that cannot decode the film's picture is offered only the qualities.
	playback.IsSourcePictureDecodable = false
	if text, _, err = service.MasterPlaylist(t.Context(), playback); err != nil || strings.Contains(text, "picture/") {
		t.Errorf("a player that cannot decode the picture is offered\n%s, %v\nwant no picture copied", text, err)
	}
	if _, err = service.MediaPlaylist(t.Context(), playback, "picture"); !errors.Is(err, ErrNoStream) {
		t.Errorf("its picture's playlist = %v; want ErrNoStream", err)
	}
}

func TestMasterPlaylist_AFilmWithNoGridIsReadThroughForOne(t *testing.T) {
	t.Parallel()
	service, reader := chunksFixture(t, newFakeQueue(), fakeNode{})
	_, _, err := service.MasterPlaylist(t.Context(), Playback{FileID: 2, IsSourcePictureDecodable: true})
	if !errors.Is(err, ErrNoGrid) || !slices.Equal(reader.asked, []int64{2}) {
		t.Errorf("film 2's master playlist = %v, read through %v; want ErrNoGrid and film 2 read through",
			err, reader.asked)
	}
}

// A film whose read-through failed is tried again for a person's play, the read-through's to judge
// whether the failure came before the play; while it is read again it has no grid yet, and on a
// failure that stands it is cut on time, which needs no keyframes (D248 §4, D344 §2).
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
	if _, _, err = service.MasterPlaylist(t.Context(), playback); !errors.Is(err, ErrNoGrid) ||
		!slices.Equal(reader.retried, []int64{2}) {
		t.Errorf("a play after film 2's read-through failed = %v, retried %v; want it read through again",
			err, reader.retried)
	}
	text, kept, err := service.MasterPlaylist(t.Context(), playback)
	if err != nil || !kept.IsOnTime || strings.Contains(text, "picture/") || !strings.Contains(text, "q4000k1080p/") {
		t.Errorf("film 2's read-through failing on the play's attempt =\n%s\n%+v, %v; want it cut on time, its "+
			"twin at the top", text, kept, err)
	}
}

// A quality offered by the file's bit rate, before the picture's bytes in each second were known, is
// still the playback's once they are, though they put the picture's own below it (the M4 closing
// audit).
func TestMediaPlaylist_AQualityOfferedBeforeThePicturesBytesWereKnownIsServedAfter(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	settingsService, isService := service.settings.(*settings.Service)
	if !isService {
		t.Fatal("the fixture's settings are not the settings service")
	}
	settingstest.Set(t, settingsService, settings.ChunkQualities, `"3000k@720p,1200k@480p"`)
	// Film 2's file runs at four megabits; its picture, once read, at two.
	err := service.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.UpsertVideoTimeline(t.Context(), sqlcgen.UpsertVideoTimelineParams{
			PayloadHash: []byte{0x02}, KeyframesMsJSON: "[0,10000,20000]",
			BytesInEachSecondJSON: "[" + strings.Repeat("250000,", 29) + "250000]",
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	playback := Playback{FileID: 2, IsSourcePictureDecodable: true}
	text, _, err := service.MasterPlaylist(t.Context(), playback)
	if err != nil || strings.Contains(text, "q3000k720p") {
		t.Fatalf("film 2 once its picture is read is offered\n%s, %v; want no 3000k quality", text, err)
	}
	if _, err = service.MediaPlaylist(t.Context(), playback, "q3000k720p"); err != nil {
		t.Errorf("the 3000k quality a playback was offered before the read = %v; want it served still", err)
	}
	if _, err = service.MediaPlaylist(t.Context(), playback, "q6000k1080p"); !errors.Is(err, ErrNoStream) {
		t.Errorf("a quality never offered = %v; want ErrNoStream", err)
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
	file, err := service.StoreFileOf(t.Context(), playback, "sound0-boost-96k", "000042.m4s")
	if err != nil || file.NodeID != 1 || file.Target != nodeproto.PathBytesChunks+"1/chunks-sound0-boost-96k/"+gridID+
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
