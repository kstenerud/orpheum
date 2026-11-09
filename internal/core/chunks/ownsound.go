// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"slices"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// The top of a film's ladder, its own picture copied, plays its own sound copied, every channel,
// where its player plays that as it is; its lower qualities play the light stereo track made at
// chunkSoundKbps, and the made mixes of a track played copied are made as loud as it is (D368).

// unknownOwnSoundKbpsPerChannel is the rate a copied sound is taken to have, for each of its
// channels, where its probe says none, as a Matroska's often does not: what its master playlist adds
// to its picture's for a player sizing its link. More than a lossy sound's usual rate, so a player
// starts no higher than its link carries.
const unknownOwnSoundKbpsPerChannel = 128

// copiedTracksOf are the playback's tracks the film has whose own sound its top plays copied: its
// own picture copied there, and the tracks its player plays as they are (Playback.OwnSoundTracks),
// in order.
func copiedTracksOf(film cutFilm, playback Playback, top []Stream) []int {
	if len(top) != 1 || top[0].Kind != nodeproto.StreamPicture {
		return nil
	}
	count := len(decide.AudioStreams(film.probe))
	var copied []int
	for _, track := range playback.OwnSoundTracks {
		if track >= 0 && track < count && !slices.Contains(copied, track) {
			copied = append(copied, track)
		}
	}
	slices.Sort(copied)
	return copied
}

// isCopied is whether the offer's top plays the track's own sound copied.
func (offered offer) isCopied(audioTrackIndex int) bool {
	return slices.Contains(offered.copied, audioTrackIndex)
}

// madeSound is the stream of one of the film's sounds as the server makes it, at chunkSoundKbps: as
// loud as the track's own sound where the top plays that copied and the film's loudness was measured,
// whether its person levels or not, so a player stepping off the top hears no change of level and
// takes nothing back out through Web Audio (D368 §1, D226 §4); else unlevelled while the loudness is
// not measured or its person plays without levelling; else levelled.
func (offered offer) madeSound(sound Sound) Stream {
	isAtOwnLevel := offered.isMeasured && offered.isCopied(sound.AudioTrackIndex)
	return Stream{
		Kind: nodeproto.StreamSound, AudioTrackIndex: sound.AudioTrackIndex, IsBoosted: sound.IsBoosted,
		SoundKbps: offered.soundKbps, IsUnlevelled: offered.isUnlevelled && !isAtOwnLevel, IsAtOwnLevel: isAtOwnLevel,
	}
}

// soundOf is the stream of one of the film's sounds that picture plays: the track's own sound
// copied, for the track's standard sound on the top where its player plays it as it is; else the
// one the server makes, which a feature that needs it made, the dialogue boost, plays at every
// quality (D368 §4).
func (offered offer) soundOf(picture Stream, sound Sound) Stream {
	if picture.Kind == nodeproto.StreamPicture && !sound.IsBoosted && offered.isCopied(sound.AudioTrackIndex) {
		return Stream{Kind: nodeproto.StreamSoundCopied, AudioTrackIndex: sound.AudioTrackIndex}
	}
	return offered.madeSound(sound)
}

// playingSoundOf is the stream of the sound the playback plays on picture, as picture plays it
// (soundOf): the sound its player last asked for or said it plays (Playback.PlayingSound), where it
// is one the offer has; else its starting track's standard sound. A track chosen as the film plays
// is switched in place, so a stall's offer, and the master playlist a player moving to it reads,
// start the sound playing, not the one the playback started with
// (audio-switch-and-retiming-2026-11-01.md §1.2).
func (offered offer) playingSoundOf(playback Playback, picture Stream) Stream {
	if stream, isStream := StreamNamed(playback.PlayingSound); isStream {
		playing := Sound{AudioTrackIndex: stream.AudioTrackIndex, IsBoosted: stream.IsBoosted}
		isSound := stream.Kind == nodeproto.StreamSound || stream.Kind == nodeproto.StreamSoundCopied
		if isSound && slices.ContainsFunc(offered.sounds, func(sound Sound) bool {
			return sound.AudioTrackIndex == playing.AudioTrackIndex && sound.IsBoosted == playing.IsBoosted
		}) {
			return offered.soundOf(picture, playing)
		}
	}
	return offered.soundOf(picture, Sound{AudioTrackIndex: playback.StartingAudioTrackIndex})
}

// isPlayingTrack is whether sound is the standard sound of the track the playback plays now
// (playingSoundOf), its master playlist's default.
func (offered offer) isPlayingTrack(playback Playback, sound Sound) bool {
	if sound.IsBoosted || !playback.HasStartingAudio {
		return false
	}
	playing := offered.playingSoundOf(playback, Stream{Kind: nodeproto.StreamQuality})
	return playing.AudioTrackIndex == sound.AudioTrackIndex
}

// SoundLevel is what levels one of a playback's sounds as its player plays it (D226 §4): the
// levelling its player wants for it, and the gain its chunks carry already, which the player does not
// add again.
type SoundLevel struct {
	Levelling     decide.Levelling
	CarriedGainDB float64
}

// OfferedSound is one of the sounds a playback is offered in an audio group: its stream, and what
// levels it.
type OfferedSound struct {
	Stream Stream
	Level  SoundLevel
}

// soundLevelOf is what levels a sound stream of the film (SoundLevel): a track copied, or a mix made
// as loud as it, wants the track's own levelling and carries none (D368 §1); a mix made unlevelled
// wants its stereo levelling and carries none; a mix made levelled carries its stereo levelling
// (D226 §4). Each sound its own track's, never the main track's, so a track switched to in place plays
// at its own level.
func soundLevelOf(levels decide.Levels, probe nodeproto.Probe, stream Stream) SoundLevel {
	switch {
	case stream.Kind == nodeproto.StreamSoundCopied || stream.IsAtOwnLevel:
		return SoundLevel{Levelling: decide.AsItIsLevelling(levels, probe, stream.AudioTrackIndex)}
	case stream.IsUnlevelled:
		return SoundLevel{Levelling: decide.StereoLevelling(levels, probe, stream.AudioTrackIndex)}
	default:
		stereo := decide.StereoLevelling(levels, probe, stream.AudioTrackIndex)
		return SoundLevel{Levelling: stereo, CarriedGainDB: stereo.GainDB}
	}
}

// offeredSoundsOn are the film's sounds picture plays, in SoundsOf's order, each with what levels it.
func (offered offer) offeredSoundsOn(levels decide.Levels, probe nodeproto.Probe, picture Stream) []OfferedSound {
	sounds := make([]OfferedSound, 0, len(offered.sounds))
	for _, sound := range offered.sounds {
		stream := offered.soundOf(picture, sound)
		sounds = append(sounds, OfferedSound{Stream: stream, Level: soundLevelOf(levels, probe, stream)})
	}
	return sounds
}

// hasPicturesOwnGroup is whether picture plays sounds of its own, the top playing a track copied,
// and so names an audio group apart from the lower qualities'.
func (offered offer) hasPicturesOwnGroup(picture Stream) bool {
	return picture.Kind == nodeproto.StreamPicture && len(offered.copied) > 0
}

// soundKbpsOf is a sound stream's rate, in kbit/s: a made one's, or a copied track's own as its probe
// says it, else unknownOwnSoundKbpsPerChannel for each of its channels.
func (film cutFilm) soundKbpsOf(stream Stream) int {
	if stream.Kind != nodeproto.StreamSoundCopied {
		return stream.SoundKbps
	}
	tracks := decide.AudioStreams(film.probe)
	if stream.AudioTrackIndex >= len(tracks) {
		return 0
	}
	track := tracks[stream.AudioTrackIndex]
	if track.BitRateBps > 0 {
		return int((track.BitRateBps + 999) / 1000)
	}
	return unknownOwnSoundKbpsPerChannel * max(1, track.Audio.ChannelCount)
}

// soundKbpsWith is the rate of the sound picture plays, the default of its audio group, which
// its master playlist adds to its own: chunkSoundKbps for a film with no sound, as for any made.
func (film cutFilm) soundKbpsWith(offered offer, playback Playback, picture Stream) int {
	if !playback.HasStartingAudio {
		return offered.soundKbps
	}
	return film.soundKbpsOf(offered.playingSoundOf(playback, picture))
}

// channelsOf is how many channels a sound stream plays: a copied track's own, a made mix's two.
func (film cutFilm) channelsOf(stream Stream) int {
	tracks := decide.AudioStreams(film.probe)
	if stream.Kind == nodeproto.StreamSoundCopied && stream.AudioTrackIndex < len(tracks) {
		return max(1, tracks[stream.AudioTrackIndex].Audio.ChannelCount)
	}
	return madeSoundChannels
}

// madeSoundChannels is how many channels every sound the server makes has: stereo.
const madeSoundChannels = 2
