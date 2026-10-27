// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// playsOwnSound is film 1's playback by a player that decodes its picture and plays its 5.1 AC-3
// track, track 0, as it is, starting on that track.
func playsOwnSound() Playback {
	return Playback{
		FileID: 1, IsSourcePictureDecodable: true, HasStartingAudio: true, StartingAudioTrackIndex: 0,
		OwnSoundTracks: []int{0}, PositionMs: 95_000,
	}
}

// The top of the ladder, the film's own picture copied, plays in an audio group of its own the
// film's own 5.1 sound copied, its six channels said, where its player plays that as it is, and the
// boosted mix made, which a feature that needs it made plays at every quality; the lower qualities
// play the light stereo track. Each slot of either group has one name, in one order, so a player
// keeps its sound as it moves between them. The track the top plays copied is made as loud as it is
// at every quality, the other levelled; the top's rate adds its own sound's, six channels at 128 kb/s
// where its probe says none (D368 §1–§4).
func TestMasterPlaylist_TheTopPlaysTheTracksItsPlayerPlaysAsTheyAreCopied(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	settingsService, isService := service.settings.(*settings.Service)
	if !isService {
		t.Fatal("the fixture's settings are not the settings service")
	}
	settingstest.Set(t, settingsService, settings.ChunkQualities, `"1200k@480p"`)
	text, _, err := service.MasterPlaylist(t.Context(), playsOwnSound())
	if err != nil {
		t.Fatal(err)
	}
	const want = "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-INDEPENDENT-SEGMENTS\n" +
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="own",NAME="sound0-96k-ownlevel",LANGUAGE="en",DEFAULT=YES,` +
		`AUTOSELECT=YES,CHANNELS="6",URI="sound0-own/stream.m3u8"` + "\n" +
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="own",NAME="sound0-boost-96k-ownlevel",LANGUAGE="en",DEFAULT=NO,` +
		`AUTOSELECT=NO,CHANNELS="2",URI="sound0-boost-96k-ownlevel/stream.m3u8"` + "\n" +
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="own",NAME="sound1-96k",LANGUAGE="und",DEFAULT=NO,AUTOSELECT=NO,` +
		`CHANNELS="2",URI="sound1-96k/stream.m3u8"` + "\n" +
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="sound0-96k-ownlevel",LANGUAGE="en",DEFAULT=YES,` +
		`AUTOSELECT=YES,CHANNELS="2",URI="sound0-96k-ownlevel/stream.m3u8"` + "\n" +
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="sound0-boost-96k-ownlevel",LANGUAGE="en",DEFAULT=NO,` +
		`AUTOSELECT=NO,CHANNELS="2",URI="sound0-boost-96k-ownlevel/stream.m3u8"` + "\n" +
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="sound1-96k",LANGUAGE="und",DEFAULT=NO,AUTOSELECT=NO,` +
		`CHANNELS="2",URI="sound1-96k/stream.m3u8"` + "\n" +
		`#EXT-X-STREAM-INF:BANDWIDTH=10368000,AVERAGE-BANDWIDTH=4772666,RESOLUTION=1920x1080,AUDIO="own"` + "\n" +
		"picture/stream.m3u8\n" +
		`#EXT-X-STREAM-INF:BANDWIDTH=1896000,AVERAGE-BANDWIDTH=1296000,RESOLUTION=854x480,AUDIO="audio"` + "\n" +
		"q1200k480p/stream.m3u8\n"
	if text != want {
		t.Errorf("the master playlist =\n%s\nwant\n%s", text, want)
	}
	// A player that cannot decode the picture starts at its twin, which plays the light track: no
	// group of its own.
	twin := playsOwnSound()
	twin.IsSourcePictureDecodable = false
	if text, _, err = service.MasterPlaylist(t.Context(), twin); err != nil || strings.Contains(text, "-own") {
		t.Errorf("a player at the twin is offered\n%s, %v; want no sound copied", text, err)
	}
}

// A playback whose person plays without levelling has the mixes of a track its top plays copied made
// as loud as that track, carrying no levelling, and its other tracks' mixes unlevelled; one whose first
// master playlist came before the film's loudness was measured keeps its mixes unlevelled, none at a
// track's level, which needs the measurement (D368 §1, D259 §2).
func TestMasterPlaylist_ATrackPlayedCopiedIsMadeAtItsOwnLevelLevellingOrNot(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	unlevelled := playsOwnSound()
	unlevelled.IsSoundUnlevelled = true
	text, kept, err := service.MasterPlaylist(t.Context(), unlevelled)
	if err != nil || !strings.Contains(text, `URI="sound0-96k-ownlevel/stream.m3u8"`) ||
		!strings.Contains(text, `URI="sound1-96k-raw/stream.m3u8"`) || strings.Contains(text, `URI="sound1-96k/`) ||
		!kept.IsSoundUnlevelled || kept.IsSoundUnmeasured {
		t.Errorf("without levelling =\n%s\n%+v, %v; want track 0's mixes at its own level, track 1's unlevelled",
			text, kept, err)
	}
	unmeasured := playsOwnSound()
	unmeasured.IsSoundUnlevelled, unmeasured.IsSoundUnmeasured = true, true
	text, kept, err = service.MasterPlaylist(t.Context(), unmeasured)
	if err != nil || !strings.Contains(text, `URI="sound0-96k-raw/stream.m3u8"`) ||
		strings.Contains(text, "ownlevel") ||
		!strings.Contains(text, `URI="sound0-own/stream.m3u8"`) || !kept.IsSoundUnmeasured {
		t.Errorf("first offered unmeasured =\n%s\n%+v, %v; want its mixes unlevelled to its end, its top copied",
			text, kept, err)
	}
}

// A sound copied is offered only of a track the top plays copied: its player plays it as it is, and
// the top is the picture copied. Its rate is its own (D368 §1).
func TestStoreFileOf_OffersASoundCopiedOnlyOfATrackTheTopPlaysCopied(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	playback := playsOwnSound()
	file, err := service.StoreFileOf(t.Context(), playback, "sound0-own", "00009.m4s")
	if err != nil || !file.IsSound() || file.AverageKbps() != 768 ||
		!strings.Contains(file.Target, "/1/chunks-sound0-own/") {
		t.Errorf("chunk 9 of track 0 copied = %+v, %v; want a sound of 768 kb/s, six channels at 128", file, err)
	}
	if _, err = service.StoreFileOf(t.Context(), playback, "sound1-own", "00009.m4s"); !errors.Is(err, ErrNoStream) {
		t.Errorf("track 1 copied, which its player was not said to play = %v; want ErrNoStream", err)
	}
	playback.OwnSoundTracks = nil
	if _, err = service.StoreFileOf(t.Context(), playback, "sound0-own", "00009.m4s"); !errors.Is(err, ErrNoStream) {
		t.Errorf("track 0 copied to a player that cannot play it = %v; want ErrNoStream", err)
	}
}

// A playback that starts at the top starts its sound copied, which carries no gain for its player to
// take back out; one its link starts lower starts the light track at the copy's level, and carries
// nothing of the levelling either: its player levels it as it levels the track played as it is
// (D368 §1, D226 §4).
func TestStartOnLadder_StartsTheTopOnTheFilmsOwnSoundCopied(t *testing.T) {
	t.Parallel()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	offer, err := service.StartOnLadder(t.Context(), playsOwnSound(), 95_000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if offer.StreamName != "picture" || offer.SoundStreamName != "sound0-own" || offer.SoundCarriedGainDB != 0 {
		t.Errorf("the offer = %+v; want the picture with its own sound copied, carrying no gain", offer)
	}
	want := []askedRun{{"picture", 9, 38}, {"sound0-own", 9, 38}}
	if runs := queue.runs(t, service); !slices.Equal(runs, want) {
		t.Errorf("the runs asked for = %+v; want the picture's and its own sound's, from chunk 9", runs)
	}
	lower, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	offer, err = lower.StartOnLadder(t.Context(), playsOwnSound(), 95_000, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if offer.StreamName != "q1200k480p" || offer.SoundStreamName != "sound0-96k-ownlevel" ||
		offer.SoundCarriedGainDB != 0 {
		t.Errorf("the offer on a slow link = %+v; want 1200k with the light track at the copy's level", offer)
	}
}

// A run of a sound copied copies its track, led in from the chunk before, carrying no gain; a run of a
// mix made as loud as its track's own sound carries the gain that brings it there, the stereo mix's
// levelling less the track's own, 2.5 dB (D368 §1).
func TestHandleMakeChunks_ACopiedSoundIsSentAsItsTrackAndAMixAtItsLevelCarriesTheDifference(t *testing.T) {
	t.Parallel()
	fixture := runsOnAFakeNode(t)
	gridID := mustGrid(t, fixture.service).ID()
	fixture.start(t, 8, testRun{"sound0-own", gridID, runArgs{FirstChunk: 2, LastChunk: 2}})
	sent := testutil.Receive(t, fixture.node.sent, "the copy's command").MakeChunks
	want := nodeproto.SoundTrack{AudioTrackIndex: 0, Language: "en", ChannelCount: 6}
	if sent == nil || sent.Stream.Kind != nodeproto.StreamSoundCopied || sent.Stream.Sound == nil ||
		*sent.Stream.Sound != want || sent.Stream.SoundKbps != 0 || sent.SoundLeadInMs != 10_000 ||
		!slices.Equal(sent.BoundsMs, []int64{20_000, 30_000}) {
		t.Fatalf("the node was sent %+v; want the English 5.1 track copied, led in from chunk 1's start", sent)
	}
	fixture.start(t, 9, testRun{"sound0-boost-96k-ownlevel", gridID, runArgs{FirstChunk: 2, LastChunk: 2}})
	sent = testutil.Receive(t, fixture.node.sent, "the mix's command").MakeChunks
	if sent == nil || sent.Stream.Kind != nodeproto.StreamSound || sent.Stream.Sound == nil ||
		sent.Stream.Sound.CarriedGainDB != 2.5 || !sent.Stream.IsBoosted {
		t.Errorf("the node was sent %+v; want the boosted mix carrying 2.5 dB, -3.5 less -6", sent)
	}
}

// Each stream's name reads back as the stream: a sound copied, and a mix as loud as its track's own;
// a mix both unlevelled and at its track's level is no stream's.
func TestStreamNamed_ReadsTheSoundsCopiedAndAtTheirOwnLevelBack(t *testing.T) {
	t.Parallel()
	for _, stream := range []Stream{
		{Kind: nodeproto.StreamSoundCopied, AudioTrackIndex: 3},
		{Kind: nodeproto.StreamSound, AudioTrackIndex: 1, IsBoosted: true, SoundKbps: 96, IsAtOwnLevel: true},
	} {
		if named, isStream := StreamNamed(stream.Name()); !isStream || named != stream {
			t.Errorf("%q reads back as %+v, %t; want %+v", stream.Name(), named, isStream, stream)
		}
	}
	for _, name := range []string{"sound0-96k-raw-ownlevel", "sound0-own-96k", "sound0-boost-own"} {
		if stream, isStream := StreamNamed(name); isStream {
			t.Errorf("%q reads as %+v; want no stream", name, stream)
		}
	}
}
