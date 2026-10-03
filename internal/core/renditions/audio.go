// SPDX-License-Identifier: GPL-3.0-or-later

package renditions

import (
	"context"
	"encoding/json"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// AudioRendition is one audio rendition of a prepared playback: a source track's standard mix,
// or its boosted one (D217 §3).
type AudioRendition struct {
	// AudioTrackIndex is the source audio track's place among its audio tracks.
	AudioTrackIndex int
	Language        string
	IsBoosted       bool
	// Levelling is the gain that brings the track's stereo sound to the level; the boosted mix
	// plays with the standard one's (D226 §3).
	Levelling decide.Levelling
	// CarriedGainDB is the gain the rendition's sound already carries, which the player takes
	// from the levelling it applies (D226 §4).
	CarriedGainDB float64
}

// levelAudio gives each audio rendition of a prepared playback its levelling, which is its track's
// stereo sound's, and the gain its sound carries, by its track's place in carried: what the
// rendition's making put in (D226 §4).
func levelAudio(renditions []AudioRendition, levels decide.Levels, probe nodeproto.Probe, carriedGainsJSON string) {
	var gains []float64
	_ = json.Unmarshal([]byte(carriedGainsJSON), &gains) //nolint:errcheck // its own record; unreadable is none
	for index := range renditions {
		rendition := &renditions[index]
		rendition.Levelling = decide.StereoLevelling(levels, probe, rendition.AudioTrackIndex)
		if rendition.AudioTrackIndex < len(gains) {
			rendition.CarriedGainDB = gains[rendition.AudioTrackIndex]
		}
	}
}

// audioRenditions lists a prepare's audio renditions in the order the master playlist has them.
func audioRenditions(command nodeproto.Prepare) []AudioRendition {
	var renditions []AudioRendition
	for _, track := range command.AudioTracks {
		standard := AudioRendition{AudioTrackIndex: track.AudioTrackIndex, Language: track.Language}
		renditions = append(renditions, standard)
		if track.ChannelCount > 2 {
			boosted := AudioRendition{AudioTrackIndex: track.AudioTrackIndex, Language: track.Language, IsBoosted: true}
			renditions = append(renditions, boosted)
		}
	}
	return renditions
}

// Wanted is a rendition a playback wants: whether it is still being made, and its audio
// renditions, levelled.
type Wanted struct {
	IsPreparing bool
	Audio       []AudioRendition
}

// Want makes sure the rendition of file in profile is complete or on its way, at the highest
// priority (want), and says how far it is and its audio renditions, each with the gain that levels
// it by levels and the gain its making put in.
func (service *Service) Want(
	ctx context.Context, file sqlcgen.GetPlayableFileRow, probe nodeproto.Probe, levels decide.Levels,
	profile nodeproto.PrepareProfile,
) (Wanted, error) {
	rendition, err := service.want(ctx, file, profile)
	if err != nil {
		return Wanted{}, err
	}
	audio := audioRenditions(prepareCommand(nodeproto.BytesFile{}, file.ID, profile, "", probe))
	levelAudio(audio, levels, probe, rendition.CarriedGainsDBJSON)
	return Wanted{IsPreparing: rendition.State != renditionComplete, Audio: audio}, nil
}
