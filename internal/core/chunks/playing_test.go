// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// twoTracksLoudness levels film 1's two audio tracks: its 5.1 track, stream 1, as stereo by -3.5 dB
// and as it is by -6; its stereo track, stream 2, by -2.
type twoTracksLoudness struct{}

func (twoTracksLoudness) Levels(context.Context, []byte) (decide.Levels, error) {
	return decide.Levels{
		{StreamIndex: 1, Mix: decide.MixStereo}:       {IsMeasured: true, GainDB: -3.5},
		{StreamIndex: 1, Mix: decide.MixMultichannel}: {IsMeasured: true, GainDB: -6},
		{StreamIndex: 2, Mix: decide.MixStereo}:       {IsMeasured: true, GainDB: -2},
	}, nil
}

// Every sound a playback is offered says what levels it, its own track's: a track copied at the top,
// and its mixes made as loud as it, want the track's own levelling as it is and carry none (D368 §1);
// another track's mix, made levelled, carries its own stereo levelling. Not the main track's, which
// levelled a playback started on another track wrong by the difference between the two tracks'
// loudness (audio-switch-and-retiming-2026-11-01.md §1.1).
func TestOffered_EachSoundIsLevelledByItsOwnTrack(t *testing.T) {
	t.Parallel()
	service, _ := chunksFixture(t, newFakeQueue(), fakeNode{})
	service.loudness = twoTracksLoudness{}
	playback := playsOwnSound()
	playback.StartingAudioTrackIndex = 1
	offered, err := service.Offered(t.Context(), playback)
	if err != nil {
		t.Fatal(err)
	}
	asItIs := SoundLevel{Levelling: decide.Levelling{IsMeasured: true, GainDB: -6}}
	second := SoundLevel{Levelling: decide.Levelling{IsMeasured: true, GainDB: -2}, CarriedGainDB: -2}
	wantLight := []OfferedSound{
		{Stream: Stream{Kind: nodeproto.StreamSound, SoundKbps: 96, IsAtOwnLevel: true}, Level: asItIs},
		{
			Stream: Stream{Kind: nodeproto.StreamSound, IsBoosted: true, SoundKbps: 96, IsAtOwnLevel: true},
			Level:  asItIs,
		},
		{Stream: Stream{Kind: nodeproto.StreamSound, AudioTrackIndex: 1, SoundKbps: 96}, Level: second},
	}
	if !slices.Equal(offered.Sounds, wantLight) {
		t.Errorf("the lower qualities' sounds = %+v; want %+v", offered.Sounds, wantLight)
	}
	if len(offered.TopSounds) != 3 || offered.TopSounds[0].Stream.Name() != "sound0-own" ||
		offered.TopSounds[0].Level != asItIs || offered.TopSounds[2] != wantLight[2] {
		t.Errorf("the top's sounds = %+v; want the 5.1 track copied, levelled as it is, carrying nothing",
			offered.TopSounds)
	}
}

// A stall's offer starts the sound the player plays now, the one it last asked for or said, a track
// switched to in place, and carries that sound's own gain; and the master playlist a player moving to
// it reads starts that track. A player that said nothing starts the playback's starting track
// (audio-switch-and-retiming-2026-11-01.md §1.2, the owner's answers of 2026-10-31).
func TestStalled_StartsTheSoundThePlayerPlaysNotTheOneItStartedWith(t *testing.T) {
	t.Parallel()
	queue := newFakeQueue()
	service, _ := chunksFixture(t, queue, fakeNode{})
	service.loudness = twoTracksLoudness{}
	playback := Playback{FileID: 1, IsSourcePictureDecodable: true, HasStartingAudio: true}
	for _, said := range []string{"sound1-own", "sound1-96k"} {
		playback.PlayingSound = said
		offer, err := service.Stalled(t.Context(), playback, 95_000, 2000)
		if err != nil {
			t.Fatal(err)
		}
		if offer.SoundStreamName != "sound1-96k" || offer.SoundCarriedGainDB != -2 {
			t.Errorf("playing %s, the offer = %+v; want the second track's mix, carrying its own -2 dB",
				said, offer)
		}
		text, _, err := service.MasterPlaylist(t.Context(), playback)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(text, `NAME="sound1-96k",LANGUAGE="und",DEFAULT=YES`) {
			t.Errorf("playing %s, the master playlist =\n%s\nwant the second track its default", said, text)
		}
	}
	playback.PlayingSound = ""
	offer, err := service.Stalled(t.Context(), playback, 95_000, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if offer.SoundStreamName != "sound0-96k" || offer.SoundCarriedGainDB != -3.5 {
		t.Errorf("a player that said nothing: the offer = %+v; want the starting track's mix", offer)
	}
}
