// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// The share of a link's measured speed a quality may take is link_share_for_quality_percent: the rest
// is the sound's, a peak's and the link's own variation (D247 §6; slow-links.md §2, players' 0.7). The
// web client's player chooses by the same share, read from the client settings.

// linkSharePercent is link_share_for_quality_percent.
func (service *Service) linkSharePercent(ctx context.Context) (int64, error) {
	percent, err := service.settings.Int(ctx, settings.LinkShareForQualityPercent)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", settings.LinkShareForQualityPercent, err)
	}
	return percent, nil
}

// isWithinShare is whether a quality of kbps fits within sharePercent of a link of linkKbps.
func isWithinShare(kbps, linkKbps, sharePercent int64) bool {
	return kbps*100 <= linkKbps*sharePercent
}

// Offer is the quality a stalled playback is offered (D247 §2, §6): the one to start at, by its
// master playlist's BANDWIDTH, which is how a player names it among the playlist's pictures.
type Offer struct {
	Quality qualities.Quality
	// StartAtBps is the quality's BANDWIDTH in the master playlist: its capped peak and the sound's.
	StartAtBps int64
	// IsBelowTheLowest is whether even the lowest quality is beyond the link as measured: a player
	// that stalls on it has no quality left to go to (D247 §8).
	IsBelowTheLowest bool
	// SoundCarriedGainDB is the levelling gain the starting sound's chunks carry, as a prepared
	// copy's do (D226 §4), so the player, which levels after its volume, does not level it again.
	SoundCarriedGainDB float64
	// FilmPictureKbps is the film's own picture's average rate, as the quality was chosen by: what a
	// player weighs its link against once the quality is ready (D253 §2); 0 when not known.
	FilmPictureKbps int64
	// StreamName is the quality's stream, and AverageKbps its average rate with its sound's, rounded
	// up, as its master playlist says: what the playback needs of the server's upload (D286 §5).
	StreamName  string
	AverageKbps int64
	// IsHeldByUpload is whether the start took a lower quality than its link carries, to keep within
	// what is left of the server's upload (StartAtMostKbps).
	IsHeldByUpload bool
	// SoundStreamName is the starting sound's stream, empty for a film with none, and FromMs where in
	// the film both are made from: what the playback wants until its player says (D309 §1).
	SoundStreamName string
	FromMs          int64
	// LinkSharePercent is the share of a measured link the quality was chosen within,
	// link_share_for_quality_percent, which the player chooses its own qualities by (D247 §6).
	LinkSharePercent int64
}

// ErrNoQualityOffered is a film that offers no quality below its own picture: nothing lower to go to.
var ErrNoQualityOffered = errors.New("the film offers no quality below its own")

// Stalled answers a playback's stall at positionMs, on a link measured at linkKbps, zero when not
// measured (D247 §2, §6, §7): it chooses the quality, the highest within linkShareForQuality of the
// link, or the lowest when none is or the link is not measured; one below it whose chunks another
// player's stall is already making there is taken instead. It starts the quality's run and the
// playback's starting sound's from the stall's chunk, so they are on their way before the player
// moves to them, and returns the offer. A film with no grid yet is ErrNoGrid, and is read through.
func (service *Service) Stalled(ctx context.Context, playback Playback, positionMs, linkKbps int64) (Offer, error) {
	lowering, err := service.lowerQualities(ctx, playback)
	if err != nil {
		return Offer{}, err
	}
	return service.startLower(ctx, playback, lowering, positionMs, linkKbps)
}

// lowering is a film a playback plays, cut, what it offers the playback, and its qualities below
// its own picture, highest first, as the master playlist lists them.
type lowering struct {
	film    cutFilm
	offered offer
	lower   []Stream
}

// lowerQualities is how a playback's film can be lowered; a film with no quality below its own
// picture is ErrNoQualityOffered.
func (service *Service) lowerQualities(ctx context.Context, playback Playback) (lowering, error) {
	film, err := service.cutForPlay(ctx, playback)
	if err != nil {
		return lowering{}, err
	}
	offered, err := service.offerOf(ctx, film, playback)
	if err != nil {
		return lowering{}, err
	}
	var lower []Stream
	for _, picture := range offered.pictures {
		if picture.Kind == nodeproto.StreamQuality {
			lower = append(lower, picture)
		}
	}
	if len(lower) == 0 {
		return lowering{}, ErrNoQualityOffered
	}
	return lowering{film: film, offered: offered, lower: lower}, nil
}

// startLower chooses, of the lower qualities, the one for a link of linkKbps, starts its run and the
// starting sound's from the chunk at positionMs, and returns the offer (Stalled).
func (service *Service) startLower(
	ctx context.Context, playback Playback, lowering lowering, positionMs, linkKbps int64,
) (Offer, error) {
	film, lower := lowering.film, lowering.lower
	chunk := film.grid.chunkAt(positionMs)
	sharePercent, err := service.linkSharePercent(ctx)
	if err != nil {
		return Offer{}, err
	}
	chosen, isBelowTheLowest := chooseQuality(lower, linkKbps, sharePercent)
	below := chosen + 1
	if below < len(lower) && service.runs.isCovered(StoreDir(playback.FileID, lower[below], film.grid.ID()), chunk) {
		chosen++ // another's, already on its way, and near enough (D247 §7)
	}
	return service.startAt(ctx, playback, lowering.film, lowering.offered,
		chosenPicture{
			stream: lower[chosen], isBelowTheLowest: isBelowTheLowest, chunk: chunk, sharePercent: sharePercent,
		})
}

// chosenPicture is the picture of a film a playback starts at, whether even the lowest is beyond its
// link, and the chunk it starts from.
type chosenPicture struct {
	stream           Stream
	isBelowTheLowest bool
	chunk            int
	// isHeldByUpload is whether it is lower than the link chose, to keep within the upload left.
	isHeldByUpload bool
	// sharePercent is the share of the link it was chosen within, which the player chooses by too.
	sharePercent int64
}

// startAt starts the runs of the chosen picture of the film's and the playback's starting sound from
// its chunk, and returns the offer of the picture.
func (service *Service) startAt(
	ctx context.Context, playback Playback, film cutFilm, offered offer, chosen chosenPicture,
) (Offer, error) {
	picture, chunk := chosen.stream, chosen.chunk
	// A stall asks for its quality again, a new try, though a run of it was evicted (D250 §6).
	service.runs.unevict(StoreDir(playback.FileID, picture, film.grid.ID()))
	if err := service.Ensure(ctx, playback.FileID, film.kind, picture, chunk); err != nil {
		return Offer{}, err
	}
	offer := Offer{
		Quality:          film.qualityOf(picture),
		StartAtBps:       film.variantOf(picture, offered.soundKbps).bandwidthBps,
		IsBelowTheLowest: chosen.isBelowTheLowest,
		FilmPictureKbps:  max(0, film.pictureAverageBps()/1000),
		StreamName:       picture.Name(),
		AverageKbps:      film.averageWithSoundKbps(picture, offered.soundKbps),
		IsHeldByUpload:   chosen.isHeldByUpload,
		FromMs:           film.grid.StartsMs[chunk],
		LinkSharePercent: chosen.sharePercent,
	}
	if playback.HasStartingAudio {
		sound := startingSound(playback, offered.soundKbps, offered.isUnlevelled)
		offer.SoundStreamName = sound.Name()
		service.runs.unevict(StoreDir(playback.FileID, sound, film.grid.ID()))
		if err := service.Ensure(ctx, playback.FileID, film.kind, sound, chunk); err != nil {
			return Offer{}, err
		}
		track, err := service.levelledTrack(ctx, film, sound)
		if err != nil {
			return Offer{}, err
		}
		offer.SoundCarriedGainDB = track.CarriedGainDB
	}
	return offer, nil
}

// qualityOf is a picture's rate and height: a quality's own, or the film's picture's, copied.
func (film cutFilm) qualityOf(picture Stream) qualities.Quality {
	if picture.Kind == nodeproto.StreamQuality {
		return picture.Quality
	}
	return qualities.Quality{Kbps: int(max(0, film.pictureAverageBps()/1000)), Height: film.picture.Video.Height}
}

// startingSound is the stream of the sound a playback started with, in chunks of soundKbps,
// unlevelled or not: what its chunks' master playlist starts with (D247 §10).
func startingSound(playback Playback, soundKbps int, isUnlevelled bool) Stream {
	return Stream{
		Kind: nodeproto.StreamSound, AudioTrackIndex: playback.StartingAudioTrackIndex, SoundKbps: soundKbps,
		IsUnlevelled: isUnlevelled,
	}
}

// chooseQuality is the place among lower, highest first, of the highest quality within sharePercent
// of linkKbps, or of the lowest when none is, which is then below the lowest, or when the link is not
// measured.
func chooseQuality(lower []Stream, linkKbps, sharePercent int64) (int, bool) {
	lowest := len(lower) - 1
	if linkKbps <= 0 {
		return lowest, false
	}
	for index, quality := range lower {
		if isWithinShare(int64(quality.Quality.Kbps), linkKbps, sharePercent) {
			return index, false
		}
	}
	return lowest, true
}
