// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"context"
	"errors"

	"github.com/kstenerud/orpheum/internal/core/chunks/qualities"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// linkShareForQuality is the share of a link's measured speed a quality may take: the rest is the
// sound's, a peak's and the link's own variation (D247 §6; slow-links.md §2, players' 0.7).
const linkShareForQuality = 0.7

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
	film, err := service.cutForPlay(ctx, playback)
	if err != nil {
		return Offer{}, err
	}
	offered, err := service.offerOf(ctx, film, playback)
	if err != nil {
		return Offer{}, err
	}
	var lower []Stream // highest first, as the master playlist lists them
	for _, picture := range offered.pictures {
		if picture.Kind == nodeproto.StreamQuality {
			lower = append(lower, picture)
		}
	}
	if len(lower) == 0 {
		return Offer{}, ErrNoQualityOffered
	}
	chunk := film.grid.chunkAt(positionMs)
	chosen, isBelowTheLowest := chooseQuality(lower, linkKbps)
	below := chosen + 1
	if below < len(lower) && service.runs.isCovered(StoreDir(playback.FileID, lower[below], film.grid.ID()), chunk) {
		chosen++ // another's, already on its way, and near enough (D247 §7)
	}
	quality := lower[chosen]
	// A stall asks for its quality again, a new try, though a run of it was evicted (D250 §6).
	service.runs.unevict(StoreDir(playback.FileID, quality, film.grid.ID()))
	if err := service.Ensure(ctx, playback.FileID, quality, chunk); err != nil {
		return Offer{}, err
	}
	offer := Offer{
		Quality:          quality.Quality,
		StartAtBps:       film.variantOf(quality, offered.soundKbps).bandwidthBps,
		IsBelowTheLowest: isBelowTheLowest,
	}
	if playback.HasStartingAudio {
		sound := startingSound(playback, offered.soundKbps)
		service.runs.unevict(StoreDir(playback.FileID, sound, film.grid.ID()))
		if err := service.Ensure(ctx, playback.FileID, sound, chunk); err != nil {
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

// startingSound is the stream of the sound a playback started with, in chunks of soundKbps: what
// its chunks' master playlist starts with (D247 §10).
func startingSound(playback Playback, soundKbps int) Stream {
	return Stream{Kind: nodeproto.StreamSound, AudioTrackIndex: playback.StartingAudioTrackIndex, SoundKbps: soundKbps}
}

// chooseQuality is the place among lower, highest first, of the highest quality within
// linkShareForQuality of linkKbps, or of the lowest when none is, which is then below the lowest, or
// when the link is not measured.
func chooseQuality(lower []Stream, linkKbps int64) (int, bool) {
	lowest := len(lower) - 1
	if linkKbps <= 0 {
		return lowest, false
	}
	for index, quality := range lower {
		if float64(quality.Quality.Kbps) <= linkShareForQuality*float64(linkKbps) {
			return index, false
		}
	}
	return lowest, true
}
