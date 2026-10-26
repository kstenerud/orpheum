// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"errors"
)

// Leave takes a player's word that it left its playback's page (D309 §7): Left, playing nothing else,
// its runs finish what it keeps ahead and stop, its other work keeping its priority; or Replaced by
// another film (D258 §2), its runs stop at once, as no other player wants them, and its chunks and
// its timeline's read-through fall to background at the next demotion, where they would have kept
// play priority for play_priority_kept_seconds; a player still watching the film asks again and
// keeps them. The token stays until it idles out: the player's page takes over the film page's token, and
// a player's last report may arrive after its leave. A token not the holder's, or gone, is
// ErrNoToken.
func (service *Service) Leave(ctx context.Context, token string, holder Holder, gone Disposition) error {
	idle, err := service.tokenIdleExpiry(ctx)
	if err != nil {
		return err
	}
	playback, found := service.tokens.use(token, holder, service.clock.Now(), idle)
	if !found {
		return ErrNoToken
	}
	if gone != Left {
		gone = Replaced
	}
	service.tokens.leave(token, gone)
	released := service.stopUnwanted(ctx, playback.fileID)
	// Left, the person may come back from the film's page: its work keeps play priority until another
	// film is chosen (the owner, 2026-10-07: "Until the user signals intent to watch a different video,
	// we should keep data").
	if gone != Replaced {
		return released
	}
	if service.chunks != nil {
		released = errors.Join(released, service.chunks.ReleaseWanted(ctx, playback.fileID))
	}
	return released
}
