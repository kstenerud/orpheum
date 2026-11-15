// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/settings"
)

// The film's own file chosen on a link remembered short of it starts on the film's ladder, where the
// link calls for, and its player moves to the original once its link carries it (the owner,
// 2026-11-15: "Better to start on a lower rung and then go back up later"); on a link not measured
// it plays at once (startOnLadder).

// isOriginalWaiting is whether a start that asked for the film's own file, and kept the ask past its
// device's data cap (deliveryUnderCap), started on its ladder instead: its link was short of the file
// (startOnLadder), so it waits for the link to carry it.
func isOriginalWaiting(request Request, onLadder *chunks.Offer) bool {
	return request.PreferredDelivery == decide.Direct && onLadder != nil
}

// linkCarries is whether a link remembered at linkKbps carries a film's own file of fileKbps, by the
// rule a start on its ladder chooses its quality by: the file within linkShareForQualityPercent of
// the link (D247 §6, D260 §2). A link not measured, or a file whose rate is not known, is taken to, so
// the original is tried first (the owner, 2026-11-15: "When we haven't measured, try the original
// first").
func (service *Service) linkCarries(ctx context.Context, fileKbps, linkKbps int64) (bool, error) {
	if linkKbps <= 0 || fileKbps <= 0 {
		return true, nil
	}
	sharePercent, err := service.settings.Int(ctx, settings.LinkShareForQualityPercent)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", settings.LinkShareForQualityPercent, err)
	}
	return chunks.IsWithinShare(fileKbps, linkKbps, sharePercent), nil
}
