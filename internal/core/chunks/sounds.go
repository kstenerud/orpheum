// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"cmp"
	"path"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/langtag"
)

// Sound is one sound a film's chunks offer: a source track's standard stereo mix, or its boosted one
// (D217 §3).
type Sound struct {
	// AudioTrackIndex is the source audio track's place among its audio tracks.
	AudioTrackIndex int
	Language        string
	IsBoosted       bool
}

// SoundTracksOf are a film's audio tracks as a run of their sound's chunks makes them: each in its
// place, tagged with its language, langtag.Undetermined when the probe gave none (D247 §10). The gain
// each carries is decide.LevelSounds' to put in.
func SoundTracksOf(probe nodeproto.Probe) []nodeproto.SoundTrack {
	var tracks []nodeproto.SoundTrack
	for index, stream := range decide.AudioStreams(probe) {
		language := cmp.Or(stream.Language, langtag.Undetermined)
		tracks = append(tracks, nodeproto.SoundTrack{
			AudioTrackIndex: index, Language: language, ChannelCount: stream.Audio.ChannelCount,
		})
	}
	return tracks
}

// SoundsOf are the sounds of a film's audio tracks, in the order its master playlist has them: each
// track's standard mix, then its boosted one when it has more than two channels, a centre to raise
// (D217 §3).
func SoundsOf(tracks []nodeproto.SoundTrack) []Sound {
	var sounds []Sound
	for _, track := range tracks {
		sounds = append(sounds, Sound{AudioTrackIndex: track.AudioTrackIndex, Language: track.Language})
		if track.ChannelCount > 2 {
			boosted := Sound{AudioTrackIndex: track.AudioTrackIndex, Language: track.Language, IsBoosted: true}
			sounds = append(sounds, boosted)
		}
	}
	return sounds
}

// contentType is a store's file's media type, by its name: a chunk's or its init's.
func contentType(name string) string {
	if path.Ext(name) == ".m4s" {
		return "video/iso.segment"
	}
	return "video/mp4"
}
