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

// scopeFilm is 28 Winters Later as the production NAS holds it: HEVC at 1920×696 and one E-AC-3 5.1 track.
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
		SoundKbps: 96, OwnPictureKbps: 2205, Sounds: ladderSounds(),
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
	levelled := decide.Levelling{IsMeasured: true, GainDB: -3.5}
	wantSounds := []SoundTrack{
		{
			AudioTrackIndex: 0, Codec: "aac", ChannelCount: 2, Kbps: 96, Stream: "sound0-96k",
			Levelling: levelled, CarriedGainDB: -3.5,
		},
		{
			AudioTrackIndex: 0, IsBoosted: true, Codec: "aac", ChannelCount: 2, Kbps: 96, Stream: "sound0-boost-96k",
			Levelling: levelled, CarriedGainDB: -3.5,
		},
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

// The top of the ladder, the film's own picture, names the sounds it plays where its device plays the
// track as it is: the file's own E-AC-3 5.1, copied, in the standard mix's place, and the boosted mix
// converted; the lower qualities play the ladder's (D368 §1). A device of formats plays as they are
// the tracks whose codec and channels the formats list, which the playback's chunks copy at its top.
func TestTracks_TheTopNamesTheFilmsOwnSoundItPlaysCopied(t *testing.T) {
	t.Parallel()
	fake := &cutAfterChunks{offered: chunks.Offered{
		Pictures: []chunks.Stream{
			{Kind: nodeproto.StreamPicture},
			{Kind: nodeproto.StreamQuality, Quality: qualities.Quality{Kbps: 2000, Height: 540}},
		},
		SoundKbps: 96, OwnPictureKbps: 2205, CopiedTracks: []int{0}, Sounds: ladderSounds(),
		TopSounds: []chunks.OfferedSound{
			{
				Stream: chunks.Stream{Kind: nodeproto.StreamSoundCopied},
				Level:  chunks.SoundLevel{Levelling: decide.Levelling{IsMeasured: true, GainDB: -6}},
			},
			ladderSounds()[1],
		},
	}}
	service := &Service{offers: fake}
	tracks, err := service.tracksOf(t.Context(), tokenPlayback{}, scopeFilm(), Started{OnLadder: &chunks.Offer{}})
	if err != nil {
		t.Fatal(err)
	}
	wantTop := []SoundTrack{
		{
			AudioTrackIndex: 0, Codec: "eac3", ChannelCount: 6, IsOriginal: true, Stream: "sound0-own",
			Levelling: decide.Levelling{IsMeasured: true, GainDB: -6},
		},
		{
			AudioTrackIndex: 0, IsBoosted: true, Codec: "aac", ChannelCount: 2, Kbps: 96, Stream: "sound0-boost-96k",
			Levelling: decide.Levelling{IsMeasured: true, GainDB: -3.5}, CarriedGainDB: -3.5,
		},
	}
	if len(tracks.Pictures) != 2 || !reflect.DeepEqual(tracks.Pictures[0].Sounds, wantTop) ||
		tracks.Pictures[1].Sounds != nil || len(tracks.Sounds) != 2 || tracks.Sounds[0].IsOriginal {
		t.Errorf("tracks on the ladder = %+v, %+v; want the top's own E-AC-3 copied, the others' the ladder's",
			tracks.Pictures, tracks.Sounds)
	}
	formats := decide.PlayableFormats{AudioCodecs: []decide.Audio{decide.AACStereo, decide.EAC3}}
	if got := ownSoundTracksOf(scopeFilm(), formats); !reflect.DeepEqual(got, []int{0}) {
		t.Errorf("a device of E-AC-3 plays %v as they are; want track 0", got)
	}
	formats.AudioCodecs = []decide.Audio{decide.AACStereo}
	if got := ownSoundTracksOf(scopeFilm(), formats); got != nil {
		t.Errorf("a device of stereo AAC alone plays %v as they are; want none", got)
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
	}) || !reflect.DeepEqual(direct.Sounds, []SoundTrack{
		{Codec: "eac3", ChannelCount: 6, IsOriginal: true, Stream: "sound0-own"},
	}) {
		t.Errorf("tracks played as they are = %+v, %+v", direct.Pictures, direct.Sounds)
	}
}

// ladderSounds are scopeFilm's sounds on its ladder, its 5.1 track's standard mix and boosted one,
// made levelled, carrying the stereo mix's -3.5 dB.
func ladderSounds() []chunks.OfferedSound {
	level := chunks.SoundLevel{Levelling: decide.Levelling{IsMeasured: true, GainDB: -3.5}, CarriedGainDB: -3.5}
	return []chunks.OfferedSound{
		{Stream: chunks.Stream{Kind: nodeproto.StreamSound, SoundKbps: 96}, Level: level},
		{Stream: chunks.Stream{Kind: nodeproto.StreamSound, IsBoosted: true, SoundKbps: 96}, Level: level},
	}
}

// twoTrackFilm is a film of two audio tracks, a 5.1 one first, the main, and a stereo one, each
// measured: the first louder as it is than the second by 4 dB.
func twoTrackFilm() (nodeproto.Probe, decide.Levels) {
	probe := scopeFilm()
	stereo := nodeproto.ProbeAudio{ChannelCount: 2}
	probe.Streams = append(probe.Streams,
		nodeproto.ProbeStream{StreamIndex: 2, Kind: "audio", Codec: "aac", Audio: stereo})
	return probe, decide.Levels{
		{StreamIndex: 1, Mix: decide.MixMultichannel}: {IsMeasured: true, GainDB: -6},
		{StreamIndex: 1, Mix: decide.MixStereo}:       {IsMeasured: true, GainDB: -3.5},
		{StreamIndex: 2, Mix: decide.MixStereo}:       {IsMeasured: true, GainDB: -2},
	}
}

// A playback starting on a track other than the main one is levelled by that track's own gain, as it
// is, never the main track's, wrong by the difference between the two tracks' loudness; one naming
// no track, by the main one's (audio-switch-and-retiming-2026-11-01.md §1.1).
func TestStartingLevelling_IsTheStartingTracksOwnNotTheMainTracks(t *testing.T) {
	t.Parallel()
	probe, levels := twoTrackFilm()
	second := &AudioTrack{AudioTrackIndex: 1, StreamIndex: 2}
	if got := startingLevelling(levels, probe, second); got != (decide.Levelling{IsMeasured: true, GainDB: -2}) {
		t.Errorf("starting on the second track: %+v; want its own -2 dB, not the main track's -6", got)
	}
	if got := startingLevelling(levels, probe, nil); got != (decide.Levelling{IsMeasured: true, GainDB: -6}) {
		t.Errorf("starting on no track named: %+v; want the main 5.1 track's -6 dB as it is", got)
	}
}

// Off the ladder, each of the file's own tracks says its stream, as its sound copied is named, and its
// own levelling as it is, so a track switched to in place by the element is levelled by its own
// (audio-switch-and-retiming-2026-11-01.md §1.2, the owner's answers of 2026-10-31).
func TestTracks_OffTheLadderEachTrackIsLevelledByItsOwn(t *testing.T) {
	t.Parallel()
	probe, levels := twoTrackFilm()
	service := &Service{}
	direct, err := service.tracksOf(t.Context(), tokenPlayback{}, probe,
		Started{Decision: decide.Decision{Delivery: decide.Direct}, levels: levels})
	if err != nil {
		t.Fatal(err)
	}
	want := []SoundTrack{
		{
			Codec: "eac3", ChannelCount: 6, IsOriginal: true, Stream: "sound0-own",
			Levelling: decide.Levelling{IsMeasured: true, GainDB: -6},
		},
		{
			AudioTrackIndex: 1, Codec: "aac", ChannelCount: 2, IsOriginal: true, Stream: "sound1-own",
			Levelling: decide.Levelling{IsMeasured: true, GainDB: -2},
		},
	}
	if !reflect.DeepEqual(direct.Sounds, want) {
		t.Errorf("the file's own tracks = %+v; want each levelled by its own", direct.Sounds)
	}
}
