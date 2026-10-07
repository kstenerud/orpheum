// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
	"errors"
)

// Leave lets go of the work a playback asked for when its player left its page, the film's or the
// player's, before or after it played (D258 §2): its rendition's prepare, its chunks and its
// timeline's read-through fall to background at the next demotion, where they would have kept play
// priority for play_priority_kept_seconds; a player still watching the film asks again and keeps
// them. The token stays until it idles out: the player's page takes over the film page's token, and
// a player's last report may arrive after its leave. A token not the holder's, or gone, is
// ErrNoToken.
func (service *Service) Leave(ctx context.Context, token string, holder Holder) error {
	idle, err := service.tokenIdleExpiry(ctx)
	if err != nil {
		return err
	}
	playback, found := service.tokens.use(token, holder, service.clock.Now(), idle)
	if !found {
		return ErrNoToken
	}
	var released error
	if playback.profile != "" {
		released = service.renditions.ReleaseWanted(ctx, playback.fileID, playback.profile)
	}
	if service.chunks != nil {
		released = errors.Join(released, service.chunks.ReleaseWanted(ctx, playback.fileID))
	}
	return released
}
