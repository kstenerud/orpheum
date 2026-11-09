// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import "errors"

// ErrTooSlowToConvert is a file its device cannot play as it is, nor its picture copied, whose node
// encodes even the lowest quality slower than playback (D457): converted, it would stall over and
// over.
var ErrTooSlowToConvert = errors.New("this file's node converts too slowly to play it on this device")

// notOnLadderTooSlow is why a playback is not on its ladder when its film's node encodes too slowly
// for the only pictures its player could play (D457).
const notOnLadderTooSlow = "tooSlowToEncode"

// cannotConvertBecause is the refusal of a playback its device cannot play as it is that is not on
// its ladder for why: ErrTooSlowToConvert when its node encodes too slowly, else ErrCannotConvert.
func cannotConvertBecause(notOnLadderBecause string) error {
	if notOnLadderBecause == notOnLadderTooSlow {
		return ErrTooSlowToConvert
	}
	return ErrCannotConvert
}
