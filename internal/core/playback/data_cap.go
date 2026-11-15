// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import "github.com/kstenerud/orpheum/internal/core/decide"

// A device's data cap is kept by the core (D493), one for each kind of network its browser says it
// is on, and the core applies the one its start says itself: the delivery the cap asks for
// (deliveryUnderCap) and the rung on the film's ladder it starts on (withinDataCap). Its player keeps
// to the cap from there, choosing each next chunk under it (D488).

// deliveryUnderCap is the delivery a playback prefers under a data cap of capKbps, 0 for none, of a
// file whose own rate, every track of it, is fileKbps, 0 where not known (D488 §4, D491 §3, §4): the
// film's own file where the person chose it and the file is within the cap; else, under a cap, the
// ladder, whose rungs include the film's own picture; with no cap, what the person preferred. The
// ladder asked for is a wish (isWish), so a film with no ladder yet still plays its own file (D489).
func deliveryUnderCap(preferred decide.Delivery, capKbps, fileKbps int64) decide.Delivery {
	if capKbps <= 0 {
		return preferred
	}
	if preferred == decide.Direct && fileKbps > 0 && fileKbps <= capKbps {
		return decide.Direct
	}
	return decide.HLS
}

// withinDataCap is the most a start on the ladder may take, a picture's average with its sound's,
// under both what is left of the server's upload, atMostKbps (0 for no bound), and the device's data
// cap, capKbps (0 for none): a capped device starts on the highest rung under its cap, or the lowest,
// never above it while its player drops to it (staging, 2026-11-15: a phone capped at 400 kbit/s
// started at 2109 and had the own picture's chunks made). A start held under the cap counts as one
// held by the upload (chunks.Offer.IsHeldByUpload), its need the rung it took.
func withinDataCap(atMostKbps, capKbps int64) int64 {
	if capKbps <= 0 {
		return atMostKbps
	}
	if atMostKbps <= 0 {
		return capKbps
	}
	return min(atMostKbps, capKbps)
}
