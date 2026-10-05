// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
	"github.com/kstenerud/orpheum/internal/core/renditions"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/langtag"
)

// ErrNoStream is a stream a playback is not offered, or a file its store does not have: a quality
// not in chunk_qualities or not below the film's own, a sound of a track the film does not have,
// the picture copied to a player that cannot decode it, or a name of no store's file.
var ErrNoStream = errors.New("no such stream of chunks")

// Playback is a playback whose film is played as chunks, as the playback service knows it.
type Playback struct {
	FileID int64
	// IsSourcePictureDecodable is whether the player decodes the film's own picture: false when the
	// playback's decision re-encoded it (profile h264-8m), so the picture copied is not offered.
	IsSourcePictureDecodable bool
	// HasStartingAudio is whether the playback started with an audio track, and
	// StartingAudioTrackIndex is that track's place among the film's audio tracks.
	HasStartingAudio        bool
	StartingAudioTrackIndex int
	// PositionMs is where its player last reported it was, in the film (D217 §7).
	PositionMs int64
}

// soundBitRateBps is the bit rate a sound's chunks are encoded at, as a prepare's sound is: AAC
// stereo at 192 kb/s (internal/node/chunks, internal/node/prepare). Every picture plays with it.
const soundBitRateBps = 192_000

// qualityPeakPerAverage is how far over its average a quality's bit rate may rise: the node caps
// its encode's peaks at one and a half times it (internal/node/prepare's QualityArguments).
const qualityPeakPerAverage = 1.5

// offer is what a film cut into chunks offers a playback: its pictures, highest first, and its
// sounds, each track's standard mix and then its boosted one, as a prepare carries them.
type offer struct {
	pictures []Stream
	sounds   []renditions.AudioRendition
}

// has is whether the offer has the stream.
func (offered offer) has(stream Stream) bool {
	if stream.Kind == nodeproto.StreamSound {
		return hasSound(offered.sounds, stream)
	}
	return slices.Contains(offered.pictures, stream)
}

// soundStream is the stream of one of a film's sounds.
func soundStream(sound renditions.AudioRendition) Stream {
	return Stream{Kind: nodeproto.StreamSound, AudioTrackIndex: sound.AudioTrackIndex, IsBoosted: sound.IsBoosted}
}

// offerOf is what the film offers the playback (chunks.md §3): its picture copied, unless the
// player cannot decode it; then each quality of chunk_qualities below the film's own picture, in
// bit rate and in height, where each is known; and every sound a prepare would make.
func (service *Service) offerOf(ctx context.Context, film cutFilm, playback Playback) (offer, error) {
	text, err := service.settings.String(ctx, settings.ChunkQualities)
	if err != nil {
		return offer{}, fmt.Errorf("read chunk_qualities: %w", err)
	}
	all, err := qualities.Parse(text)
	if err != nil {
		return offer{}, fmt.Errorf("read chunk_qualities: %w", err)
	}
	offered := offer{sounds: renditions.AudioRenditionsOf(renditions.PreparedAudio(film.probe))}
	if playback.IsSourcePictureDecodable {
		offered.pictures = append(offered.pictures, Stream{Kind: nodeproto.StreamPicture})
	}
	averageBps, height := film.pictureAverageBps(), film.picture.Video.Height
	for _, quality := range all {
		if (averageBps <= 0 || int64(quality.Kbps)*1000 < averageBps) && (height <= 0 || quality.Height < height) {
			offered.pictures = append(offered.pictures, Stream{Kind: nodeproto.StreamQuality, Quality: quality})
		}
	}
	return offered, nil
}

// pictureAverageBps is the film's own picture's average bit rate: from its bytes in each second, or,
// when its timeline has none, the file's as its probe said; zero when neither is known.
func (film cutFilm) pictureAverageBps() int64 {
	if len(film.bytesInEachSecond) == 0 {
		return film.probe.Format.BitRateBps
	}
	var total int64
	for _, bytes := range film.bytesInEachSecond {
		total += bytes
	}
	return total * 8 / int64(len(film.bytesInEachSecond))
}

// picturePeakBps is the film's own picture's peak bit rate, by which a link is sized (slow-links.md
// §3): its busiest second, or, when its timeline has none, twice its average.
func (film cutFilm) picturePeakBps() int64 {
	if len(film.bytesInEachSecond) == 0 {
		return 2 * film.probe.Format.BitRateBps
	}
	return slices.Max(film.bytesInEachSecond) * 8
}

// variantOf is a picture as the master playlist offers it, the sound's bit rate added to its own.
func (film cutFilm) variantOf(stream Stream) variant {
	picture := variant{uri: stream.Name() + "/stream.m3u8"}
	sourceWidth, sourceHeight := film.picture.Video.Width, film.picture.Video.Height
	if stream.Kind == nodeproto.StreamQuality {
		averageBps := int64(stream.Quality.Kbps) * 1000
		picture.bandwidthBps = int64(float64(averageBps)*qualityPeakPerAverage) + soundBitRateBps
		picture.averageBandwidthBps = averageBps + soundBitRateBps
		if sourceWidth > 0 && sourceHeight > 0 {
			// As FFmpeg's scale=-2:<height> makes it: the source's shape, rounded to an even width.
			halfWidth := (sourceWidth*stream.Quality.Height + sourceHeight) / (2 * sourceHeight)
			picture.width, picture.height = 2*halfWidth, stream.Quality.Height
		}
		return picture
	}
	picture.bandwidthBps = film.picturePeakBps() + soundBitRateBps
	if average := film.pictureAverageBps(); average > 0 {
		picture.averageBandwidthBps = average + soundBitRateBps
	}
	picture.width, picture.height = sourceWidth, sourceHeight
	return picture
}

// cutForPlay is the playback's film as it is cut into chunks, for a player waiting on it. A film
// with no grid yet is read through at play priority (D247 §2), and is ErrNoGrid until it has one.
func (service *Service) cutForPlay(ctx context.Context, fileID int64) (cutFilm, error) {
	film, isCut, err := service.cut(ctx, fileID)
	if err != nil || isCut {
		return film, err
	}
	if _, err = service.readThrough.ReadForPlay(ctx, fileID); err != nil {
		return film, err
	}
	return film, fmt.Errorf("file %d: %w", fileID, ErrNoGrid)
}

// MasterPlaylist is the master playlist of the playback's film cut into chunks (chunks.md §3): every
// sound a prepare would make, the playback's starting track's standard mix the default, and every
// picture it is offered, highest first. A film with no grid yet is ErrNoGrid, and is being read
// through for one; one that cannot be cut is ErrCannotBeCut.
func (service *Service) MasterPlaylist(ctx context.Context, playback Playback) (string, error) {
	film, err := service.cutForPlay(ctx, playback.FileID)
	if err != nil {
		return "", err
	}
	offered, err := service.offerOf(ctx, film, playback)
	if err != nil {
		return "", err
	}
	sounds := make([]soundMedia, 0, len(offered.sounds))
	for _, sound := range offered.sounds {
		name := soundStream(sound).Name()
		language := sound.Language
		if !langtag.IsWellFormed(language) {
			language = langtag.Undetermined
		}
		sounds = append(sounds, soundMedia{
			uri: name + "/stream.m3u8", name: name, language: language,
			isDefault: !sound.IsBoosted && playback.HasStartingAudio &&
				sound.AudioTrackIndex == playback.StartingAudioTrackIndex,
		})
	}
	// A playback that started with no track the film still has starts with its first sound.
	if len(sounds) > 0 && !slices.ContainsFunc(sounds, func(sound soundMedia) bool { return sound.isDefault }) {
		sounds[0].isDefault = true
	}
	variants := make([]variant, 0, len(offered.pictures))
	for _, picture := range offered.pictures {
		variants = append(variants, film.variantOf(picture))
	}
	return masterPlaylist(sounds, variants), nil
}

// MediaPlaylist is the playlist of one stream the playback is offered, every chunk of the film's
// grid: ErrNoStream for a stream it is not offered, and ErrNoGrid as MasterPlaylist says.
func (service *Service) MediaPlaylist(ctx context.Context, playback Playback, streamName string) (string, error) {
	film, _, err := service.offeredStream(ctx, playback, streamName)
	if err != nil {
		return "", err
	}
	return mediaPlaylist(film.grid), nil
}

// offeredStream is the playback's film, cut, and the stream a name names, when the playback is
// offered it.
func (service *Service) offeredStream(
	ctx context.Context, playback Playback, streamName string,
) (cutFilm, Stream, error) {
	stream, isStream := StreamNamed(streamName)
	if !isStream {
		return cutFilm{}, Stream{}, fmt.Errorf("%w: %q", ErrNoStream, streamName)
	}
	film, err := service.cutForPlay(ctx, playback.FileID)
	if err != nil {
		return film, stream, err
	}
	offered, err := service.offerOf(ctx, film, playback)
	if err != nil {
		return film, stream, err
	}
	if !offered.has(stream) {
		return film, stream, fmt.Errorf("%w: file %d's %s", ErrNoStream, playback.FileID, streamName)
	}
	return film, stream, nil
}
