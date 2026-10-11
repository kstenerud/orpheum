// SPDX-License-Identifier: AGPL-3.0-or-later

package decide

import (
	"encoding/json"
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
// more than two channels, which only a receiver is sent as it is (D222), else as stereo.
func DirectLevelling(levels Levels, probe nodeproto.Probe) Levelling {
	main, found := MainAudio(probe)
	if !found {
		return Levelling{}
	}
	mix := MixStereo
	if main.Audio.ChannelCount > 2 {
		mix = MixMultichannel
	}
	return levels[SoundKey{int64(main.StreamIndex), mix}]
}

// StereoLevelling is the levelling of a file's audio track, by its place among its audio tracks, as
// stereo: what a prepared rendition's sound is brought to (D226 §4); none for a place it lacks.
func StereoLevelling(levels Levels, probe nodeproto.Probe, audioTrackIndex int) Levelling {
	indexes := AudioStreamIndexes(probe)
	if audioTrackIndex >= len(indexes) {
		return Levelling{}
	}
	return levels[SoundKey{int64(indexes[audioTrackIndex]), MixStereo}]
}

// AudioStreams are a file's audio tracks, in order: a track's place among them is how a prepare and
// a playback's audio renditions name it.
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

// LevelPrepare puts each audio track's levelling gain into a prepare's command, and returns them as
// its rendition records them (D226 §4). A track not measured yet carries none.
func LevelPrepare(command *nodeproto.Prepare, levels Levels, probe nodeproto.Probe) string {
	for index := range command.AudioTracks {
		track := &command.AudioTracks[index]
		track.CarriedGainDB = StereoLevelling(levels, probe, track.AudioTrackIndex).GainDB
	}
	return CarriedGainsJSON(command.AudioTracks)
}

// CarriedGainsJSON are the gains a prepare puts in its audio tracks, by their place, as a rendition
// records them (D226 §4).
func CarriedGainsJSON(tracks []nodeproto.PrepareAudio) string {
	gains := make([]float64, 0, len(tracks))
	for _, track := range tracks {
		gains = append(gains, track.CarriedGainDB)
	}
	encoded, err := json.Marshal(gains)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}
