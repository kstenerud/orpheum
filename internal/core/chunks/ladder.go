// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"context"
)

// StartOnLadder starts a playback on its film's ladder (D260 §1, §2): every picture it is offered,
// its own copied first where its player decodes it, then each quality below. It starts at the top,
// or, with its link remembered at linkKbps, at the highest within linkShareForQuality of it, or the
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
	chosen, isBelowTheLowest := film.startingPicture(offered.pictures, linkKbps)
	return service.startAt(ctx, playback, film, offered, chosenPicture{
		stream: offered.pictures[chosen], isBelowTheLowest: isBelowTheLowest, chunk: film.grid.chunkAt(positionMs),
	})
}

// startingPicture is the place among pictures, highest first, a playback starts at: the top with
// the link not known; else the highest whose rate is within linkShareForQuality of linkKbps, or the
// lowest, then below the lowest, when none is. A picture copied whose rate is not known is passed
// over for a link that is known.
func (film cutFilm) startingPicture(pictures []Stream, linkKbps int64) (int, bool) {
	if linkKbps <= 0 {
		return 0, false
	}
	for index, picture := range pictures {
		kbps := film.qualityOf(picture).Kbps
		if kbps > 0 && float64(kbps) <= linkShareForQuality*float64(linkKbps) {
			return index, false
		}
	}
	return len(pictures) - 1, true
}
