// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

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
	// playback's decision re-encoded it (profile h264), so the picture copied is not offered.
	IsSourcePictureDecodable bool
	// HasStartingAudio is whether the playback started with an audio track, and
	// StartingAudioTrackIndex is that track's place among the film's audio tracks.
	HasStartingAudio        bool
	StartingAudioTrackIndex int
	// PositionMs is where its player last reported it was, in the film (D217 §7).
	PositionMs int64
	// StartedAt is when its person pressed play: what failed before then is tried again for them.
	StartedAt time.Time
	// IsOnTime is whether it began on its film cut on time, its file having no index of its
	// keyframes: it stays so to its end, though a read-through finds them meanwhile (D269 §3).
	IsOnTime bool
	// IsSoundUnlevelled is whether its first master playlist named its film's sounds unlevelled, the
	// film's loudness not measured yet: they stay so to its end, though the loudness is measured
	// meanwhile, so a stall's offer starts and waits on the sound its player plays.
	IsSoundUnlevelled bool
}

// Kept is what a playback keeps, to its end, of the first master playlist it was offered, though
// its film changes meanwhile: whether the film was cut on time, and whether its sounds were named
// unlevelled.
type Kept struct {
	IsOnTime          bool
	IsSoundUnlevelled bool
}

// Kept is what the playback keeps already.
func (playback Playback) Kept() Kept {
	return Kept{IsOnTime: playback.IsOnTime, IsSoundUnlevelled: playback.IsSoundUnlevelled}
}

// gridKind is what the playback's film is cut at: on time, once it began so, else on keyframes.
func (playback Playback) gridKind() GridKind {
	if playback.IsOnTime {
		return OnTime
	}
	return OnKeyframes
}

// qualityPeakPerAverage is how far over its average a quality's bit rate may rise: the node caps
// its encode's peaks at one and a half times it (internal/node/prepare's QualityArguments).
const qualityPeakPerAverage = 1.5

// offer is what a film cut into chunks offers a playback: its pictures, highest first, and its
// sounds, each track's standard mix and then its boosted one, as a prepare carries them.
type offer struct {
	pictures []Stream
	sounds   []renditions.AudioRendition
	// soundKbps is chunk_sound_kbps: every sound's bit rate, which every picture plays with.
	soundKbps int
	// isUnlevelled is whether the film's loudness is not measured yet: its sounds are offered
	// unlevelled (Stream.IsUnlevelled).
	isUnlevelled bool
}

// has is whether the offer has the stream. A sound is had levelled or not, whichever the film's
// loudness now says, so a playback goes on with the sounds its playlist named.
func (offered offer) has(stream Stream) bool {
	if stream.Kind == nodeproto.StreamSound {
		return stream.SoundKbps == offered.soundKbps && hasSound(offered.sounds, stream)
	}
	return slices.Contains(offered.pictures, stream)
}

// soundStream is the stream of one of a film's sounds, at kbps, unlevelled or not.
func soundStream(sound renditions.AudioRendition, kbps int, isUnlevelled bool) Stream {
	return Stream{
		Kind: nodeproto.StreamSound, AudioTrackIndex: sound.AudioTrackIndex, IsBoosted: sound.IsBoosted,
		SoundKbps: kbps, IsUnlevelled: isUnlevelled,
	}
}

// offerOf is what the film offers the playback (chunks.md §3): its picture copied, unless the
// player cannot decode it; then each quality of chunk_qualities below the film's own picture, in
// bit rate and in height, where each is known; and every sound a prepare would make, unlevelled
// while the film's loudness is not measured (D259 §2).
func (service *Service) offerOf(ctx context.Context, film cutFilm, playback Playback) (offer, error) {
	text, err := service.settings.String(ctx, settings.ChunkQualities)
	if err != nil {
		return offer{}, fmt.Errorf("read chunk_qualities: %w", err)
	}
	all, err := qualities.Parse(text)
	if err != nil {
		return offer{}, fmt.Errorf("read chunk_qualities: %w", err)
	}
	soundKbps, err := service.settings.Int(ctx, settings.ChunkSoundKbps)
	if err != nil {
		return offer{}, fmt.Errorf("read chunk_sound_kbps: %w", err)
	}
	isUnlevelled, err := service.isSoundUnlevelled(ctx, film, playback)
	if err != nil {
		return offer{}, err
	}
	offered := offer{
		sounds: renditions.AudioRenditionsOf(renditions.PreparedAudio(film.probe)), soundKbps: int(soundKbps),
		isUnlevelled: isUnlevelled,
	}
	// A picture copied begins each chunk on a keyframe: a film cut on time has none there (D269 §1).
	if playback.IsSourcePictureDecodable && film.kind == OnKeyframes {
		offered.pictures = append(offered.pictures, Stream{Kind: nodeproto.StreamPicture})
	}
	averageBps, height := film.pictureAverageBps(), film.picture.Video.Height
	for _, quality := range all {
		if (averageBps <= 0 || int64(quality.Kbps)*1000 < averageBps) && (height <= 0 || quality.Height < height) {
			offered.pictures = append(offered.pictures, Stream{Kind: nodeproto.StreamQuality, Quality: quality})
		}
	}
	if len(offered.pictures) == 0 && film.kind == OnTime && len(all) > 0 {
		only := onlyQuality(all, height)
		offered.pictures = append(offered.pictures, Stream{Kind: nodeproto.StreamQuality, Quality: only})
	}
	return offered, nil
}

// onlyQuality is the one quality a film cut on time is offered when none is below its own picture,
// as a picture of low bit rate has none: the highest no taller than it, else the lowest, so its
// playback has a picture (D269 §1). The qualities are chunk_qualities, highest first.
func onlyQuality(all []qualities.Quality, height int) qualities.Quality {
	for _, quality := range all {
		if height <= 0 || quality.Height <= height {
			return quality
		}
	}
	return all[len(all)-1]
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

// variantOf is a picture as the master playlist offers it, the sound's bit rate, soundKbps, added
// to its own.
func (film cutFilm) variantOf(stream Stream, soundKbps int) variant {
	soundBitRateBps := int64(soundKbps) * 1000
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
// with no grid yet is read through at play priority (D247 §2), from its file's index, and is
// ErrNoGrid until it has one; a film whose file has no index is cut on time, and read through
// ahead of the sweep for its keyframes, which a later playback is cut at (D269). A read-through
// that failed before its person pressed play is tried again for them, its failure forgotten: a
// person's play makes every attempt, and only a failure on that attempt is theirs to see (the
// owner, 2026-10-06, D248).
func (service *Service) cutForPlay(ctx context.Context, playback Playback) (cutFilm, error) {
	fileID := playback.FileID
	film, isCut, err := service.cut(ctx, fileID, playback.gridKind())
	if errors.Is(err, ErrCannotBeCut) && film.picture.Codec != "" {
		isRetried, retryErr := service.readThrough.RetryFailedForPlay(ctx, fileID, film.file.PayloadHash,
			film.picture.StreamIndex, playback.StartedAt)
		if retryErr != nil {
			return film, retryErr
		}
		if isRetried {
			return film, fmt.Errorf("file %d: %w", fileID, ErrNoGrid)
		}
	}
	if err != nil || isCut {
		return film, err
	}
	if service.readThrough.HasNoIndex(film.file.PayloadHash, film.picture.StreamIndex) {
		if _, err = service.readThrough.ReadFirst(ctx, fileID); err != nil {
			return film, err
		}
		film, _, err = service.cut(ctx, fileID, OnTime)
		return film, err
	}
	if _, err = service.readThrough.ReadForPlay(ctx, fileID); err != nil {
		return film, err
	}
	return film, fmt.Errorf("file %d: %w", fileID, ErrNoGrid)
}

// isSoundUnlevelled is whether the playback is offered its film's sounds unlevelled: when it kept
// them so, or while the film's loudness is not measured (D259 §2).
func (service *Service) isSoundUnlevelled(ctx context.Context, film cutFilm, playback Playback) (bool, error) {
	if playback.IsSoundUnlevelled {
		return true, nil
	}
	levels, err := service.loudness.Levels(ctx, film.file.PayloadHash)
	if err != nil {
		return false, err
	}
	return len(levels) == 0, nil
}

// MasterPlaylist is the master playlist of the playback's film cut into chunks (chunks.md §3): every
// sound a prepare would make, the playback's starting track's standard mix the default, and every
// picture it is offered, highest first; and what the playback keeps of it from then on: whether the
// film is cut on time (D269 §3), and whether its sounds are named unlevelled. A film with no grid
// yet is ErrNoGrid, and is being read through for one; one that cannot be cut is ErrCannotBeCut.
func (service *Service) MasterPlaylist(ctx context.Context, playback Playback) (string, Kept, error) {
	film, err := service.cutForPlay(ctx, playback)
	if err != nil {
		return "", Kept{}, err
	}
	offered, err := service.offerOf(ctx, film, playback)
	if err != nil {
		return "", Kept{}, err
	}
	sounds := make([]soundMedia, 0, len(offered.sounds))
	for _, sound := range offered.sounds {
		name := soundStream(sound, offered.soundKbps, offered.isUnlevelled).Name()
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
		variants = append(variants, film.variantOf(picture, offered.soundKbps))
	}
	kept := Kept{IsOnTime: film.kind == OnTime, IsSoundUnlevelled: offered.isUnlevelled}
	return masterPlaylist(sounds, variants), kept, nil
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
	film, err := service.cutForPlay(ctx, playback)
	if err != nil {
		return film, stream, err
	}
	offered, err := service.offerOf(ctx, film, playback)
	if err != nil {
		return film, stream, err
	}
	if offered.has(stream) {
		return film, stream, nil
	}
	// A playback offered its film before its picture's bytes in each second were known was offered
	// by the file's bit rate, sound and all: a quality between the two is the playback's still,
	// which once it came was refused mid-film (the M4 closing audit, 2026-10-08).
	before := film
	before.bytesInEachSecond = nil
	offeredBefore, err := service.offerOf(ctx, before, playback)
	if err != nil {
		return film, stream, err
	}
	if !offeredBefore.has(stream) {
		return film, stream, fmt.Errorf("%w: file %d's %s", ErrNoStream, playback.FileID, streamName)
	}
	return film, stream, nil
}
