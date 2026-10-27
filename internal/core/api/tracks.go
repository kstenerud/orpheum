// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import "github.com/kstenerud/orpheum/internal/core/playback"

// TracksBody is what a playback may play and what the file holds, which the player names side by
// side: each track as the file has it, or converted, and from what (the owner, 2026-10-20).
type TracksBody struct {
	// Pictures are the pictures it may play: its film's ladder's, highest first, or the one it plays.
	Pictures []PictureTrackBody `json:"pictures"`
	// Sounds are the sounds it may play, by audio track and mix.
	Sounds      []SoundTrackBody `json:"sounds"`
	FilePicture FilePictureBody  `doc:"The file's own picture, as the server probed it" json:"filePicture"`
	FileSounds  []FileSoundBody  `doc:"The file's own audio tracks, as the server probed them" json:"fileSounds"`
}

// PictureTrackBody is a picture a playback may play.
type PictureTrackBody struct {
	Stream     string           `doc:"Its stream on the film's ladder, as the player's level names it; absent off it" json:"stream,omitempty"` //nolint:lll // one struct tag
	Codec      string           `doc:"ffprobe's name for it: hevc, h264, av1 ..." json:"codec"`
	Width      int              `doc:"0 when not known" json:"width"`
	Height     int              `doc:"0 when not known" json:"height"`
	Kbps       int64            `doc:"Its average rate; 0 when not known" json:"kbps"`
	IsOriginal bool             `doc:"Whether it is the file's own, played as it is" json:"isOriginal"`
	Sounds     []SoundTrackBody `doc:"The sounds it plays where they are not the ladder's (sounds), in their order: the top's, its tracks' own copied; absent for every other picture" json:"sounds,omitempty"` //nolint:lll // one struct tag
}

// SoundTrackBody is a sound a playback may play: one of the file's audio tracks, as it is or
// converted.
type SoundTrackBody struct {
	AudioTrackIndex int    `json:"audioTrackIndex"`
	IsBoosted       bool   `doc:"Whether it is the mix with the dialogue boost" json:"isBoosted"`
	Codec           string `doc:"ffprobe's name for it: aac, eac3, opus ..." json:"codec"`
	ChannelCount    int    `json:"channelCount"`
	Kbps            int64  `doc:"Its rate; 0 when not known, as a file's own is not" json:"kbps"`
	IsOriginal      bool   `doc:"Whether it is the file's own, played as it is" json:"isOriginal"`
}

// FilePictureBody is the file's own picture.
type FilePictureBody struct {
	Codec  string `json:"codec"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// FileSoundBody is one of the file's own audio tracks.
type FileSoundBody struct {
	AudioTrackIndex int    `json:"audioTrackIndex"`
	Codec           string `json:"codec"`
	ChannelCount    int    `json:"channelCount"`
}

// soundTrackBodies are sounds as the API says them; nil for none.
func soundTrackBodies(sounds []playback.SoundTrack) []SoundTrackBody {
	var bodies []SoundTrackBody
	for _, sound := range sounds {
		bodies = append(bodies, SoundTrackBody{
			AudioTrackIndex: sound.AudioTrackIndex, IsBoosted: sound.IsBoosted, Codec: sound.Codec,
			ChannelCount: sound.ChannelCount, Kbps: sound.Kbps, IsOriginal: sound.IsOriginal,
		})
	}
	return bodies
}

// tracksBody is a playback's tracks, as the API says them.
func tracksBody(tracks playback.Tracks) TracksBody {
	body := TracksBody{
		Pictures: make([]PictureTrackBody, 0, len(tracks.Pictures)),
		FilePicture: FilePictureBody{
			Codec: tracks.FilePicture.Codec, Width: tracks.FilePicture.Width, Height: tracks.FilePicture.Height,
		},
		FileSounds: make([]FileSoundBody, 0, len(tracks.FileSounds)),
	}
	for _, picture := range tracks.Pictures {
		body.Pictures = append(body.Pictures, PictureTrackBody{
			Stream: picture.Stream, Codec: picture.Codec, Width: picture.Width, Height: picture.Height,
			Kbps: picture.Kbps, IsOriginal: picture.IsOriginal, Sounds: soundTrackBodies(picture.Sounds),
		})
	}
	body.Sounds = soundTrackBodies(tracks.Sounds)
	if body.Sounds == nil {
		body.Sounds = []SoundTrackBody{}
	}
	for _, sound := range tracks.FileSounds {
		body.FileSounds = append(body.FileSounds, FileSoundBody{
			AudioTrackIndex: sound.AudioTrackIndex, Codec: sound.Codec, ChannelCount: sound.ChannelCount,
		})
	}
	return body
}
