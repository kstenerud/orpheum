// SPDX-License-Identifier: AGPL-3.0-or-later

package decide

import (
	"math"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// peakCeilingDBTP is the highest true peak a gain may bring a sound to (D226 §3): fixed, not a
// setting, because above it a sound clips when it is decoded or resampled. The level a sound is
// brought towards and the most it is raised are the settings loudness_target_lufs and
// loudness_max_raise_db.
const peakCeilingDBTP = -1.0

// streamKindAudio is an audio track's kind, as a probe names it.
const streamKindAudio = "audio"

// The mixes of a track a measurement names: the multichannel sound as it is, which a receiver
// plays, and the stereo a stereo device hears (D226 §1).
const (
	MixMultichannel = "multichannel"
	MixStereo       = "stereo"
)

// LoudnessAim is the level a sound is brought towards and the most it is raised (D226 §3).
type LoudnessAim struct {
	TargetLUFS, MaxRaiseDB float64
}

// Levelling is what brings a sound to the level: the gain, once the sound is measured. A sound not
// measured yet plays as it is (D226 §6).
type Levelling struct {
	IsMeasured bool
	GainDB     float64
}

// SoundKey names one measured sound of a file: a track, as its multichannel sound or as stereo.
type SoundKey struct {
	StreamIndex int64
	Mix         string
}

// Levels are the gains that level each measured sound of a file.
type Levels map[SoundKey]Levelling

// Measured is what a measurement found of one sound: its integrated loudness and its true peak,
// nil for silence, which has no peak; or that it could not be measured.
type Measured struct {
	IntegratedLUFS *float64
	TruePeakDBTP   *float64
	IsFailure      bool
}

// LevellingOf is the gain that brings a measured sound to the level, within its limits; a sound that
// could not be measured has none, and silence, which has no peak, is left as it is.
func LevellingOf(measured Measured, aim LoudnessAim) (Levelling, bool) {
	if measured.IsFailure || measured.IntegratedLUFS == nil {
		return Levelling{}, false
	}
	if measured.TruePeakDBTP == nil {
		return Levelling{IsMeasured: true}, true
	}
	gain := min(aim.TargetLUFS-*measured.IntegratedLUFS, peakCeilingDBTP-*measured.TruePeakDBTP, aim.MaxRaiseDB)
	return Levelling{IsMeasured: true, GainDB: math.Round(gain*10) / 10}, true
}

// DirectLevelling is the gain for a file played as it is: its main track's, as it is when it has
// more than two channels, which the device mixes down itself (D336 §1), else as stereo.
func DirectLevelling(levels Levels, probe nodeproto.Probe) Levelling {
	main, found := MainAudio(probe)
	if !found {
		return Levelling{}
	}
	return asItIs(levels, main)
}

// AsItIsLevelling is the levelling of a file's audio track, by its place among its audio tracks,
// played as it is: what a track copied, or a mix made as loud as it (D368 §1), is brought to by its
// player; none for a place it lacks. A playback that starts on a track other than the main one is
// levelled by that track's own, never the main track's (audio-switch-and-retiming-2026-11-01.md §1.2).
func AsItIsLevelling(levels Levels, probe nodeproto.Probe, audioTrackIndex int) Levelling {
	tracks := AudioStreams(probe)
	if audioTrackIndex < 0 || audioTrackIndex >= len(tracks) {
		return Levelling{}
	}
	return asItIs(levels, tracks[audioTrackIndex])
}

// asItIs is the levelling of an audio track played as it is: as its multichannel sound when it has
// more than two channels, which the device mixes down itself, else as stereo.
func asItIs(levels Levels, track nodeproto.ProbeStream) Levelling {
	mix := MixStereo
	if track.Audio.ChannelCount > 2 {
		mix = MixMultichannel
	}
	return levels[SoundKey{int64(track.StreamIndex), mix}]
}

// OwnLevelGainDB is the gain that makes a file's audio track's stereo mix, by its place among the
// tracks, as loud as the track played as it is: the stereo mix's levelling less the track's own, so
// a player stepping from the track copied to the mix hears one level (D368 §1); zero for a place it
// lacks, or a sound not measured.
func OwnLevelGainDB(levels Levels, probe nodeproto.Probe, audioTrackIndex int) float64 {
	tracks := AudioStreams(probe)
	if audioTrackIndex >= len(tracks) {
		return 0
	}
	own := asItIs(levels, tracks[audioTrackIndex])
	stereo := StereoLevelling(levels, probe, audioTrackIndex)
	if !own.IsMeasured || !stereo.IsMeasured {
		return 0
	}
	return math.Round((stereo.GainDB-own.GainDB)*10) / 10
}

// StereoLevelling is the levelling of a file's audio track, by its place among its audio tracks, as
// stereo: what a converted sound is brought to (D226 §4); none for a place it lacks.
func StereoLevelling(levels Levels, probe nodeproto.Probe, audioTrackIndex int) Levelling {
	indexes := AudioStreamIndexes(probe)
	if audioTrackIndex >= len(indexes) {
		return Levelling{}
	}
	return levels[SoundKey{int64(indexes[audioTrackIndex]), MixStereo}]
}

// AudioStreams are a file's audio tracks, in order: a track's place among them is how a run of its
// sound's chunks and a playback's sounds name it.
func AudioStreams(probe nodeproto.Probe) []nodeproto.ProbeStream {
	var tracks []nodeproto.ProbeStream
	for _, stream := range probe.Streams {
		if stream.Kind == streamKindAudio {
			tracks = append(tracks, stream)
		}
	}
	return tracks
}

// AudioStreamIndexes are the stream indexes of a file's audio tracks, by their place among them.
func AudioStreamIndexes(probe nodeproto.Probe) []int {
	var indexes []int
	for _, stream := range AudioStreams(probe) {
		indexes = append(indexes, stream.StreamIndex)
	}
	return indexes
}

// LevelSounds puts each audio track's levelling gain into the track, the gain its sound's chunks
// carry (D226 §4). A track not measured yet carries none.
func LevelSounds(tracks []nodeproto.SoundTrack, levels Levels, probe nodeproto.Probe) {
	for index := range tracks {
		track := &tracks[index]
		track.CarriedGainDB = StereoLevelling(levels, probe, track.AudioTrackIndex).GainDB
	}
}
