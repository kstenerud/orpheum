// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
	"github.com/kstenerud/orpheum/internal/core/decide"
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
	// IsSoundUnmeasured is whether its first master playlist was made before its film's loudness was
	// measured: no mix of it is made as loud as a track's own sound, which needs the measurement, to
	// its end, so its sounds stay as they were named (D259 §2, D368 §1).
	IsSoundUnmeasured bool
	// StartAtMostKbps is the highest rate, a picture's average with its sound's, its start on its
	// ladder may take: what is left of the server's upload for films (D286 §5); zero for no bound.
	StartAtMostKbps int64
	// OwnSoundTracks are the film's audio tracks, by their place among them, its player plays as they
	// are (decide.PlaysAsItIs): the top of its ladder, the film's own picture copied, plays each of
	// them copied (D368 §1).
	OwnSoundTracks []int
}

// Kept is what a playback keeps, to its end, of the first master playlist it was offered, though
// its film changes meanwhile: whether the film was cut on time, whether its sounds were named
// unlevelled, and whether before the film's loudness was measured.
type Kept struct {
	IsOnTime          bool
	IsSoundUnlevelled bool
	IsSoundUnmeasured bool
}

// Kept is what the playback keeps already.
func (playback Playback) Kept() Kept {
	return Kept{
		IsOnTime: playback.IsOnTime, IsSoundUnlevelled: playback.IsSoundUnlevelled,
		IsSoundUnmeasured: playback.IsSoundUnmeasured,
	}
}

// gridKind is what the playback's film is cut at: on time, once it began so, else on keyframes.
func (playback Playback) gridKind() GridKind {
	if playback.IsOnTime {
		return OnTime
	}
	return OnKeyframes
}

// qualityPeakPerAverage is how far over its average a quality's bit rate may rise: the node caps
// its encode's peaks at one and a half times it (internal/node/encode's QualityArguments).
const qualityPeakPerAverage = 1.5

// offer is what a film cut into chunks offers a playback: its pictures, highest first, and its
// sounds, each track's standard mix and then its boosted one (SoundsOf), made, or, on the top, a
// track's own copied (soundOf).
type offer struct {
	pictures []Stream
	sounds   []Sound
	// soundKbps is chunk_sound_kbps: the bit rate of every sound the server makes.
	soundKbps int
	// isUnlevelled is whether the film's loudness is not measured yet, or its person plays without
	// levelling: its sounds are offered unlevelled (Stream.IsUnlevelled); and isMeasured whether its
	// loudness was measured before the playback's first master playlist, so a mix can be made as
	// loud as its track's own sound (Stream.IsAtOwnLevel).
	isUnlevelled, isMeasured bool
	// copied are the tracks, by place, whose own sound the top, the film's own picture copied, plays
	// copied (D368 §1); none where the top is not the picture copied.
	copied []int
}

// has is whether the offer has the stream. A sound made is had levelled or not, whichever the film's
// loudness now says, so a playback goes on with the sounds its playlist named; a sound copied only of
// a track the top plays copied.
func (offered offer) has(stream Stream) bool {
	switch stream.Kind {
	case nodeproto.StreamSound:
		return stream.SoundKbps == offered.soundKbps && hasSound(offered.sounds, stream)
	case nodeproto.StreamSoundCopied:
		return offered.isCopied(stream.AudioTrackIndex)
	case nodeproto.StreamPicture, nodeproto.StreamQuality:
	}
	return slices.Contains(offered.pictures, stream)
}

// offerOf is what the film offers the playback (chunks.md §3): its picture copied, unless the
// player cannot decode it, when its twin, H.264 at its own height, takes its place (D323 §1); then each
// quality of the ladder below the film's own picture, in bit rate and in height, where each is known,
// the ladder chunk_qualities_hardware where the film's node encodes on a GPU, else chunk_qualities
// (D323 §3); and every sound of its tracks (SoundsOf), unlevelled while the film's loudness is not
// measured (D259 §2), each track the player plays as it is copied on the picture copied (D368 §1),
// but a track whose copy FFmpeg failed on twice at one chunk (hasGivenUp), which plays the sound the
// server makes instead, as the lower qualities do, until the film is forgotten: a copy that cannot be
// cut leaves the film playable (A Woman of Paris, staging, 2026-10-28).
func (service *Service) offerOf(ctx context.Context, film cutFilm, playback Playback) (offer, error) {
	all, err := service.ladderOf(ctx, film)
	if err != nil {
		return offer{}, err
	}
	soundKbps, err := service.settings.Int(ctx, settings.ChunkSoundKbps)
	if err != nil {
		return offer{}, fmt.Errorf("read chunk_sound_kbps: %w", err)
	}
	levels, err := service.loudness.Levels(ctx, film.file.PayloadHash)
	if err != nil {
		return offer{}, err
	}
	offered := offer{
		sounds: SoundsOf(SoundTracksOf(film.probe)), soundKbps: int(soundKbps),
		isUnlevelled: playback.IsSoundUnlevelled || len(levels) == 0,
		isMeasured:   !playback.IsSoundUnmeasured && len(levels) > 0,
	}
	height := film.picture.Video.Height
	top, averageBps := topOf(film, playback)
	offered.pictures = append(offered.pictures, top...)
	offered.copied = slices.DeleteFunc(copiedTracksOf(film, playback, top), func(track int) bool {
		return service.runs.hasGivenUp(StoreDir(film.file.ID, Stream{
			Kind: nodeproto.StreamSoundCopied, AudioTrackIndex: track,
		}, film.grid.ID()))
	})
	for _, quality := range all {
		if (averageBps <= 0 || int64(quality.Kbps)*1000 < averageBps) && (height <= 0 || quality.Height < height) {
			offered.pictures = append(offered.pictures, Stream{Kind: nodeproto.StreamQuality, Quality: quality})
		}
	}
	// A picture too small for a twin, which the player cannot decode, has nothing above or below it:
	// it is offered the one quality, as a film cut on time is, so its playback has a picture (D344 §4).
	if len(offered.pictures) == 0 && len(all) > 0 {
		only := onlyQuality(all, height)
		offered.pictures = append(offered.pictures, Stream{Kind: nodeproto.StreamQuality, Quality: only})
	}
	return offered, nil
}

// topOf is the top of the film's ladder for the playback, and the rate the qualities below it step
// down from: its own picture copied, where the player decodes it and the film is cut on its keyframes,
// which a copy needs (D269 §1); its twin where the player does not decode it, or where the film is
// cut on time, so a playback on time is not held below the film's own height (D344 §3), from whose
// rate the qualities below step down, since they are H.264 as the twin is and the original packs
// more into a bit (D323 §1, the owner, 2026-10-20); else none, below its own picture's rate.
func topOf(film cutFilm, playback Playback) ([]Stream, int64) {
	switch {
	case playback.IsSourcePictureDecodable && film.kind == OnKeyframes:
		return []Stream{{Kind: nodeproto.StreamPicture}}, film.pictureAverageBps()
	case film.picture.Video.Height >= qualities.MinHeight:
		twin := twinOf(film)
		return []Stream{twin}, int64(twin.Quality.Kbps) * 1000
	default:
		return nil, film.pictureAverageBps()
	}
}

// twinOf is the film's twin: its picture in H.264 at its own height, even, as H.264's 4:2:0 picture
// needs, and at most qualities.MaxHeight, at decide.TwinKbps (D323 §1).
func twinOf(film cutFilm) Stream {
	height := min(film.picture.Video.Height, qualities.MaxHeight)
	return Stream{Kind: nodeproto.StreamQuality, Quality: qualities.Quality{
		Kbps: decide.TwinKbps(film.probe), Height: height - height%2,
	}}
}

// ladderOf are the qualities the film may be cut at: chunk_qualities_hardware where its node encodes
// on a GPU, chunk_qualities where it does not (D323 §3).
func (service *Service) ladderOf(ctx context.Context, film cutFilm) ([]qualities.Quality, error) {
	key := settings.ChunkQualities
	if service.encoders != nil && service.encoders.EncodesOnAGPU(film.file.NodeID) {
		key = settings.ChunkQualitiesHardware
	}
	text, err := service.settings.String(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", key, err)
	}
	all, err := qualities.Parse(text)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", key, err)
	}
	return all, nil
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
// owner, 2026-10-06, D248). A picture its keyframes cannot cut, FFmpeg having failed to read it
// through or found no keyframe before its end, is cut on time (D344 §2).
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
		if film.kind == OnKeyframes {
			film, _, err = service.cut(ctx, fileID, OnTime)
			return film, err
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

// MasterPlaylist is the master playlist of the playback's film cut into chunks (chunks.md §3): every
// sound of its tracks (SoundsOf), the playback's starting track's standard mix the default, in the
// audio group the lower qualities play, and, where the top plays a track's own sound copied, again in
// a group of the top's own (D368 §3); every picture it is offered, highest first, each naming its
// group; and what the playback keeps of it from then on: whether the film is cut on time (D269 §3),
// and whether its sounds are named unlevelled. A film with no grid yet is ErrNoGrid, and is being
// read through for one; one that cannot be cut is ErrCannotBeCut.
func (service *Service) MasterPlaylist(ctx context.Context, playback Playback) (string, Kept, error) {
	film, err := service.cutForPlay(ctx, playback)
	if err != nil {
		return "", Kept{}, err
	}
	offered, err := service.offerOf(ctx, film, playback)
	if err != nil {
		return "", Kept{}, err
	}
	var groups []soundGroup
	variants := make([]variant, 0, len(offered.pictures))
	for _, picture := range offered.pictures {
		shown := film.variantOf(picture, film.soundKbpsWith(offered, playback, picture))
		shown.soundGroup = lightSoundGroup
		if offered.hasPicturesOwnGroup(picture) {
			shown.soundGroup = ownSoundGroup
		}
		if !slices.ContainsFunc(groups, func(group soundGroup) bool { return group.id == shown.soundGroup }) {
			sounds := film.soundMediaOf(offered, playback, picture)
			groups = append(groups, soundGroup{id: shown.soundGroup, sounds: sounds})
		}
		variants = append(variants, shown)
	}
	kept := Kept{
		IsOnTime: film.kind == OnTime, IsSoundUnlevelled: offered.isUnlevelled, IsSoundUnmeasured: !offered.isMeasured,
	}
	return masterPlaylist(groups, variants), kept, nil
}

// The audio groups of a master playlist: the lower qualities' sounds, which every picture plays
// where the top plays no sound of its own, and the top's, its tracks' own sounds copied (D368 §3).
const (
	lightSoundGroup = "audio"
	ownSoundGroup   = "own"
)

// soundMediaOf are the film's sounds picture plays (soundOf), each as its master playlist names it: in
// SoundsOf's order, which every group keeps, so a player keeps the sound's place as it moves between
// groups; each named as its made mix is, a name the groups share; the playback's starting track's
// standard sound the default, or the first where the film no longer has that track.
func (film cutFilm) soundMediaOf(offered offer, playback Playback, picture Stream) []soundMedia {
	sounds := make([]soundMedia, 0, len(offered.sounds))
	for _, sound := range offered.sounds {
		stream := offered.soundOf(picture, sound)
		language := sound.Language
		if !langtag.IsWellFormed(language) {
			language = langtag.Undetermined
		}
		sounds = append(sounds, soundMedia{
			uri: stream.Name() + "/stream.m3u8", name: offered.madeSound(sound).Name(), language: language,
			channels: film.channelsOf(stream),
			isDefault: !sound.IsBoosted && playback.HasStartingAudio &&
				sound.AudioTrackIndex == playback.StartingAudioTrackIndex,
		})
	}
	// A playback that started with no track the film still has starts with its first sound.
	if len(sounds) > 0 && !slices.ContainsFunc(sounds, func(sound soundMedia) bool { return sound.isDefault }) {
		sounds[0].isDefault = true
	}
	return sounds
}

// Rung is one picture a playback is offered, as its master playlist names it: its stream's name
// and its average rate with its sound's, in kbit/s, as a player reads its level's (D286 §5).
type Rung struct {
	StreamName  string
	AverageKbps int64
}

// Ladder is every picture a playback is offered, lowest rate first: what the server may lower it
// along when its upload cannot carry every player (D286 §5). A picture whose average is not known is
// placed by its peak. ErrNoGrid as MasterPlaylist says.
func (service *Service) Ladder(ctx context.Context, playback Playback) ([]Rung, error) {
	film, err := service.cutForPlay(ctx, playback)
	if err != nil {
		return nil, err
	}
	offered, err := service.offerOf(ctx, film, playback)
	if err != nil {
		return nil, err
	}
	rungs := make([]Rung, 0, len(offered.pictures))
	for _, picture := range offered.pictures {
		rungs = append(rungs, Rung{
			StreamName:  picture.Name(),
			AverageKbps: film.averageWithSoundKbps(picture, film.soundKbpsWith(offered, playback, picture)),
		})
	}
	slices.SortStableFunc(rungs, func(one, other Rung) int { return cmp.Compare(one.AverageKbps, other.AverageKbps) })
	return rungs, nil
}

// Offered is what a playback on its film's ladder may play, as its player names it beside what it
// is made from: its pictures, highest first, the rate every sound is made at, the tracks whose own
// sound its top plays copied, and the film's own picture's average rate (the owner, 2026-10-20: which
// tracks are the original's, and which are converted, and from what).
type Offered struct {
	Pictures  []Stream
	SoundKbps int
	// CopiedTracks are the audio tracks, by place, the top, the film's own picture, plays copied as
	// they are (D368 §1).
	CopiedTracks []int
	// OwnPictureKbps is the film's own picture's average rate, 0 when not known.
	OwnPictureKbps int64
}

// Offered is what the playback's film offers it. ErrNoGrid as MasterPlaylist says.
func (service *Service) Offered(ctx context.Context, playback Playback) (Offered, error) {
	film, err := service.cutForPlay(ctx, playback)
	if err != nil {
		return Offered{}, err
	}
	offered, err := service.offerOf(ctx, film, playback)
	if err != nil {
		return Offered{}, err
	}
	return Offered{
		Pictures: offered.pictures, SoundKbps: offered.soundKbps, CopiedTracks: offered.copied,
		OwnPictureKbps: film.pictureAverageBps() / 1000,
	}, nil
}

// averageWithSoundKbps is a picture's average rate with its sound's, in kbit/s, as its master
// playlist says it, placed by its peak where its average is not known; rounded up, since a player
// keeps a level whose average is at most the ceiling it is told (D286 §5).
func (film cutFilm) averageWithSoundKbps(picture Stream, soundKbps int) int64 {
	shown := film.variantOf(picture, soundKbps)
	averageBps := shown.averageBandwidthBps
	if averageBps == 0 {
		averageBps = shown.bandwidthBps
	}
	return (averageBps + 999) / 1000
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
