// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// resumeAt is where the person's playback of a file starts (play-state.md): where they left it,
// unless that is before resume_min_percent of the file's length, or after finished_after_percent,
// when it starts from the beginning. A file whose length is unknown resumes where it was left: a
// missing fact never becomes "finished".
func (service *Service) resumeAt(ctx context.Context, userID, fileID, durationMs int64) (int64, error) {
	stored, err := store.One(service.database.Read().GetPlayPosition(ctx, sqlcgen.GetPlayPositionParams{
		UserID: userID, FileID: fileID,
	}))
	if errors.Is(err, store.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read the position in file %d: %w", fileID, err)
	}
	if durationMs <= 0 {
		return stored.PositionMs, nil
	}
	minimum, err := service.settings.Int(ctx, settings.ResumeMinPercent)
	if err != nil {
		return 0, fmt.Errorf("read resume_min_percent: %w", err)
	}
	finished, err := service.settings.Int(ctx, settings.FinishedAfterPercent)
	if err != nil {
		return 0, fmt.Errorf("read finished_after_percent: %w", err)
	}
	// Compared as products, so no rounding moves the edges.
	if stored.PositionMs*100 < minimum*durationMs || stored.PositionMs*100 > finished*durationMs {
		return 0, nil
	}
	return stored.PositionMs, nil
}
