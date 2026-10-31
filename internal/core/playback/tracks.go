// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"

	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Tracks are what a playback may play and what the file holds, which its player names side by side:
// each track as the file has it, or converted, and from what (the owner, 2026-10-20: "show which
// tracks are original, and which are transcoded, and from what").
type Tracks struct {
	// Pictures are the pictures the playback may play: its film's ladder's, highest first, or the one
	// it plays off it.
	Pictures []PictureTrack
	// Sounds are the sounds it may play: each of the file's audio tracks, as it is or converted, and a
	// converted one's mix with the dialogue boost too.
	Sounds []SoundTrack
	// FilePicture and FileSounds are the file's own, as the probe found them.
	FilePicture FilePicture
	FileSounds  []FileSound
}

// PictureTrack is a picture a playback may play.
type PictureTrack struct {
	// Stream is its stream on the film's ladder, as the player's level names it; empty off it.
	Stream string
	// Codec is ffprobe's name for it ("hevc", "h264"); Width and Height are 0 when not known.
	Codec         string
	Width, Height int
	// Kbps is its average rate, 0 when not known.
	Kbps       int64
	IsOriginal bool
	// Sounds are the sounds it plays where they are not the ladder's light ones (Tracks.Sounds), in
	// their order: the top's, its tracks' own sounds copied where its device plays them (D368 §1);
	// nil for every other picture.
	Sounds []SoundTrack
}

// SoundTrack is a sound a playback may play: one of the file's audio tracks, as it is or converted.
type SoundTrack struct {
	AudioTrackIndex int
	IsBoosted       bool
	Codec           string
	ChannelCount    int
	// Kbps is its rate, 0 when not known, as the file's own is not.
	Kbps       int64
	IsOriginal bool
	// Stream is its stream, as its master playlist names it; off the ladder, the file's own track as
	// its sound copied is named, which its player says it plays once it switches to it in place.
	Stream string
	// Levelling is what its player wants to bring it to the level, and CarriedGainDB the gain its
	// chunks carry already (D226 §4): its own track's, so a sound switched to in place, as the
	// dialogue boost is, plays at its own level (audio-switch-and-retiming-2026-11-01.md §1.2).
	Levelling     decide.Levelling
	CarriedGainDB float64
}

// FilePicture is the file's own picture, as the probe found it.
type FilePicture struct {
	Codec         string
	Width, Height int
}

// FileSound is one of the file's own audio tracks, as the probe found it.
type FileSound struct {
	AudioTrackIndex int
	Codec           string
	ChannelCount    int
}

// Every converted sound is AAC in stereo, and every converted picture H.264, as a chunk makes them
// (internal/node).
const (
	convertedSoundCodec    = "aac"
	convertedSoundChannels = 2
	convertedPictureCodec  = "h264"
)

// Offers says what a film's ladder offers a playback: chunks.Service.
type Offers interface {
	Offered(ctx context.Context, playback chunks.Playback) (chunks.Offered, error)
}

// tracksOf are the playback's tracks: on its film's ladder, offered, when it starts on it; else
// played as the file is.
func (service *Service) tracksOf(
	ctx context.Context, playback tokenPlayback, probe nodeproto.Probe, started Started,
) (Tracks, error) {
	tracks := fileTracksOf(probe)
	switch {
	case started.OnLadder != nil && service.offers != nil:
		offered, err := service.offers.Offered(ctx, chunksPlaybackOf(playback))
		if err != nil {
			return Tracks{}, err
		}
		tracks.Pictures = ladderPictures(offered, tracks.FilePicture)
		tracks.Sounds = convertedSounds(offered.Sounds, int64(offered.SoundKbps))
		if offered.TopSounds != nil && len(tracks.Pictures) > 0 && tracks.Pictures[0].IsOriginal {
			tracks.Pictures[0].Sounds = withOwnSounds(tracks.Sounds, offered.TopSounds, probe)
		}
	default:
		tracks.Pictures = []PictureTrack{originalPicture(tracks.FilePicture, probe.Format.BitRateBps/1000)}
		for _, sound := range tracks.FileSounds {
			own := chunks.Stream{Kind: nodeproto.StreamSoundCopied, AudioTrackIndex: sound.AudioTrackIndex}
			tracks.Sounds = append(tracks.Sounds, SoundTrack{
				AudioTrackIndex: sound.AudioTrackIndex, Codec: sound.Codec, ChannelCount: sound.ChannelCount,
				IsOriginal: true, Stream: own.Name(),
				Levelling: decide.AsItIsLevelling(started.levels, probe, sound.AudioTrackIndex),
			})
		}
	}
	return tracks, nil
}

// fileTracksOf are the file's own picture and audio tracks, as probe found them.
func fileTracksOf(probe nodeproto.Probe) Tracks {
	var tracks Tracks
	if video, isVideo := decide.MainVideo(probe); isVideo {
		tracks.FilePicture = FilePicture{Codec: video.Codec, Width: video.Video.Width, Height: video.Video.Height}
	}
	for index, stream := range decide.AudioStreams(probe) {
		tracks.FileSounds = append(tracks.FileSounds, FileSound{
			AudioTrackIndex: index, Codec: stream.Codec, ChannelCount: stream.Audio.ChannelCount,
		})
	}
	return tracks
}

// originalPicture is the file's own picture, played as it is, at kbps.
func originalPicture(file FilePicture, kbps int64) PictureTrack {
	return PictureTrack{Codec: file.Codec, Width: file.Width, Height: file.Height, Kbps: kbps, IsOriginal: true}
}

// ladderPictures are the pictures a film's ladder offers: its own, copied, and each lower quality,
// H.264 scaled to its height with the film's shape, as a chunk makes it (scale=-2:<height>).
func ladderPictures(offered chunks.Offered, file FilePicture) []PictureTrack {
	pictures := make([]PictureTrack, 0, len(offered.Pictures))
	for _, stream := range offered.Pictures {
		if stream.Kind != nodeproto.StreamQuality {
			picture := originalPicture(file, offered.OwnPictureKbps)
			picture.Stream = stream.Name()
			pictures = append(pictures, picture)
			continue
		}
		pictures = append(pictures, PictureTrack{
			Stream: stream.Name(), Codec: convertedPictureCodec,
			Width: widthAt(file, stream.Quality.Height), Height: stream.Quality.Height,
			Kbps: int64(stream.Quality.Kbps),
		})
	}
	return pictures
}

// widthAt is the width of file's picture scaled to height with its shape, rounded and then down to
// even, as FFmpeg's -2 makes it (libavfilter's scale_eval.c); 0 when the file's size is not known.
func widthAt(file FilePicture, height int) int {
	if file.Width <= 0 || file.Height <= 0 {
		return 0
	}
	width := (file.Width*height + file.Height/2) / file.Height
	return width - width%2
}

// withOwnSounds are sounds as the top plays them, top in their order: each standard mix of a track it
// plays copied replaced by the track's own sound, as it is (D368 §1), each with its stream and what
// levels it there.
func withOwnSounds(sounds []SoundTrack, top []chunks.OfferedSound, probe nodeproto.Probe) []SoundTrack {
	own := make([]SoundTrack, 0, len(sounds))
	streams := decide.AudioStreams(probe)
	for index, sound := range sounds {
		if index >= len(top) {
			break
		}
		played := top[index]
		if played.Stream.Kind == nodeproto.StreamSoundCopied && sound.AudioTrackIndex < len(streams) {
			stream := streams[sound.AudioTrackIndex]
			sound = SoundTrack{
				AudioTrackIndex: sound.AudioTrackIndex, Codec: stream.Codec, ChannelCount: stream.Audio.ChannelCount,
				Kbps: stream.BitRateBps / 1000, IsOriginal: true,
			}
		}
		sound.Stream, sound.Levelling, sound.CarriedGainDB = played.Stream.Name(), played.Level.Levelling,
			played.Level.CarriedGainDB
		own = append(own, sound)
	}
	return own
}

// convertedSounds are the sounds a conversion makes of each sound offered, AAC in stereo at kbps,
// each with its stream and what levels it.
func convertedSounds(made []chunks.OfferedSound, kbps int64) []SoundTrack {
	sounds := make([]SoundTrack, 0, len(made))
	for _, sound := range made {
		sounds = append(sounds, SoundTrack{
			AudioTrackIndex: sound.Stream.AudioTrackIndex, IsBoosted: sound.Stream.IsBoosted,
			Codec: convertedSoundCodec, ChannelCount: convertedSoundChannels, Kbps: kbps, Stream: sound.Stream.Name(),
			Levelling: sound.Level.Levelling, CarriedGainDB: sound.Level.CarriedGainDB,
		})
	}
	return sounds
}
