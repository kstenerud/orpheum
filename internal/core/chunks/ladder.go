// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"
)

// StartOnLadder starts a playback on its film's ladder (D260 §1, §2): every picture it is offered,
// its own copied first where its player decodes it, then each quality below. It starts at the top,
// or, with its link remembered at linkKbps, at the highest within link_share_for_quality_percent of it, or the
// lowest when none is; it starts that picture's run and the starting sound's from the chunk at
// positionMs, and returns the offer. Its player moves along the ladder by itself from there. A film
// with no grid yet is ErrNoGrid, and is read through; one that offers no picture,
// ErrNoQualityOffered.
func (service *Service) StartOnLadder(
	ctx context.Context, playback Playback, positionMs, linkKbps int64,
) (Offer, error) {
	film, err := service.cutForPlay(ctx, playback)
	if err != nil {
		return Offer{}, err
	}
	offered, err := service.offerOf(ctx, film, playback)
	if err != nil {
		return Offer{}, err
	}
	if len(offered.pictures) == 0 {
		return Offer{}, ErrNoQualityOffered
	}
	sharePercent, err := service.linkSharePercent(ctx)
	if err != nil {
		return Offer{}, err
	}
	chosen, isBelowTheLowest := film.startingPicture(offered.pictures, linkKbps, sharePercent)
	held := film.heldByUpload(offered, playback, chosen)
	return service.startAt(ctx, playback, film, offered, chosenPicture{
		stream: offered.pictures[held], isBelowTheLowest: isBelowTheLowest, chunk: film.grid.chunkAt(positionMs),
		isHeldByUpload: held != chosen, sharePercent: sharePercent,
	})
}

// heldByUpload is the place among the offered pictures, highest first, a start the link put at
// chosen takes within the playback's StartAtMostKbps, its average with its sound's: chosen or lower,
// the lowest when none is within it; chosen with no bound (D286 §5).
func (film cutFilm) heldByUpload(offered offer, playback Playback, chosen int) int {
	atMostKbps := playback.StartAtMostKbps
	if atMostKbps <= 0 {
		return chosen
	}
	for index := chosen; index < len(offered.pictures); index++ {
		picture := offered.pictures[index]
		if film.averageWithSoundKbps(picture, film.soundKbpsWith(offered, playback, picture)) <= atMostKbps {
			return index
		}
	}
	return len(offered.pictures) - 1
}

// startingPicture is the place among pictures, highest first, a playback starts at: the top with
// the link not known; else the highest whose rate is within sharePercent of linkKbps, or the
// lowest, then below the lowest, when none is. A picture copied whose rate is not known is passed
// over for a link that is known.
func (film cutFilm) startingPicture(pictures []Stream, linkKbps, sharePercent int64) (int, bool) {
	if linkKbps <= 0 {
		return 0, false
	}
	for index, picture := range pictures {
		kbps := film.qualityOf(picture).Kbps
		if kbps > 0 && isWithinShare(int64(kbps), linkKbps, sharePercent) {
			return index, false
		}
	}
	return len(pictures) - 1, true
}
