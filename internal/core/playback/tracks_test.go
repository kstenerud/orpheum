// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"reflect"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// scopeFilm is The Big Parade as nas.example holds it: HEVC at 1920×696 and one E-AC-3 5.1 track.
func scopeFilm() nodeproto.Probe {
	return nodeproto.Probe{
		Format: nodeproto.ProbeFormat{BitRateBps: 2_600_000},
		Streams: []nodeproto.ProbeStream{
			{StreamIndex: 0, Kind: "video", Codec: "hevc", Video: nodeproto.ProbeVideo{Width: 1920, Height: 696}},
			{StreamIndex: 1, Kind: "audio", Codec: "eac3", Audio: nodeproto.ProbeAudio{ChannelCount: 6}},
		},
	}
}

// On its ladder, a playback's tracks are the film's own picture, copied, and each lower quality, H.264
// scaled to the film's shape, beside the file's own; its sounds AAC in stereo at the ladder's rate,
// the standard mix and the boosted one, each converted from the file's track (the owner, 2026-10-20:
// "show which tracks are original, and which are transcoded, and from what").
func TestTracks_OnTheLadderAreTheFilmsOwnPictureAndConvertedQualitiesAndSounds(t *testing.T) {
	t.Parallel()
	fake := &cutAfterChunks{offered: chunks.Offered{
		Pictures: []chunks.Stream{
			{Kind: nodeproto.StreamPicture},
			{Kind: nodeproto.StreamQuality, Quality: qualities.Quality{Kbps: 2000, Height: 540}},
		},
		SoundKbps: 96, OwnPictureKbps: 2205,
	}}
	service := &Service{offers: fake}
	tracks, err := service.tracksOf(t.Context(), tokenPlayback{}, scopeFilm(), Started{OnLadder: &chunks.Offer{}})
	if err != nil {
		t.Fatal(err)
	}
	wantPictures := []PictureTrack{
		{Stream: "picture", Codec: "hevc", Width: 1920, Height: 696, Kbps: 2205, IsOriginal: true},
		{Stream: "q2000k540p", Codec: "h264", Width: 1490, Height: 540, Kbps: 2000},
	}
	wantSounds := []SoundTrack{
		{AudioTrackIndex: 0, Codec: "aac", ChannelCount: 2, Kbps: 96},
		{AudioTrackIndex: 0, IsBoosted: true, Codec: "aac", ChannelCount: 2, Kbps: 96},
	}
	if !reflect.DeepEqual(tracks.Pictures, wantPictures) || !reflect.DeepEqual(tracks.Sounds, wantSounds) {
		t.Errorf("tracks on the ladder = %+v, %+v; want %+v, %+v",
			tracks.Pictures, tracks.Sounds, wantPictures, wantSounds)
	}
	if tracks.FilePicture != (FilePicture{Codec: "hevc", Width: 1920, Height: 696}) ||
		!reflect.DeepEqual(tracks.FileSounds, []FileSound{{AudioTrackIndex: 0, Codec: "eac3", ChannelCount: 6}}) {
		t.Errorf("the file's own = %+v, %+v; want HEVC 1920×696 and E-AC-3 5.1", tracks.FilePicture, tracks.FileSounds)
	}
}

// Off the ladder, a file played as it is plays every track as it is.
func TestTracks_OffTheLadderAreTheFilesOwn(t *testing.T) {
	t.Parallel()
	service := &Service{}
	direct, err := service.tracksOf(t.Context(), tokenPlayback{}, scopeFilm(),
		Started{Decision: decide.Decision{Delivery: decide.Direct}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(direct.Pictures, []PictureTrack{
		{Codec: "hevc", Width: 1920, Height: 696, Kbps: 2600, IsOriginal: true},
	}) || !reflect.DeepEqual(direct.Sounds, []SoundTrack{{Codec: "eac3", ChannelCount: 6, IsOriginal: true}}) {
		t.Errorf("tracks played as they are = %+v, %+v", direct.Pictures, direct.Sounds)
	}
}
