// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// resumePositionMs is where the person's playback of a file starts (play-state.md), by the rule a
// title's page names its Play by (library's ResumeAtMs).
func (service *Service) resumePositionMs(ctx context.Context, userID, fileID, durationMs int64) (int64, error) {
	stored, err := store.OrNotFound(service.database.Read().GetPlayPosition(ctx, sqlcgen.GetPlayPositionParams{
		UserID: userID, FileID: fileID,
	}))
	if errors.Is(err, store.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read the position in file %d: %w", fileID, err)
	}
	thresholds, err := library.ReadResumeThresholds(ctx, service.settings)
	if err != nil {
		return 0, err
	}
	return thresholds.ResumeAtMs(stored.PositionMs, stored.PositionReadAt, stored.MarkedAt, durationMs), nil
}
