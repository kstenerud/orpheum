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

// resumePositionMs is where the person's playback of a file starts (play-state.md): where they left it,
// when that is partway through the file by the resume thresholds, and otherwise the beginning.
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
	if !thresholds.IsPartwayThrough(stored.PositionMs, durationMs) {
		return 0, nil
	}
	return stored.PositionMs, nil
}
