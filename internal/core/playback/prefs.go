// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
	"fmt"
	"strconv"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// FilePrefs are a person's choices for one file (cut-a §5). A sync offset is in seconds of one
// file, so it is kept at file scope, and it lasts: the next playback starts with it.
type FilePrefs struct {
	// SubtitleOffsetMs shows the subtitles this many milliseconds later, or earlier when negative.
	SubtitleOffsetMs int64
}

// The preference table's names for what FilePrefs holds.
const (
	scopeFile            = "file"
	axisSubtitleOffsetMs = "subtitle_offset_ms"
)

// FilePrefs are the person's choices for the file, at their starting values where they made
// none.
func (service *Service) FilePrefs(ctx context.Context, userID, fileID int64) (FilePrefs, error) {
	stored, err := service.database.Read().ListPreferencesAt(ctx, sqlcgen.ListPreferencesAtParams{
		UserID: userID, Scope: scopeFile, ScopeID: fileID,
	})
	if err != nil {
		return FilePrefs{}, fmt.Errorf("read the preferences for file %d: %w", fileID, err)
	}
	var prefs FilePrefs
	for _, preference := range stored {
		if preference.Axis != axisSubtitleOffsetMs {
			continue
		}
		prefs.SubtitleOffsetMs, err = strconv.ParseInt(preference.Value, 10, 64)
		if err != nil {
			return FilePrefs{}, fmt.Errorf("read the subtitle offset for file %d: %w", fileID, err)
		}
	}
	return prefs, nil
}

// SetFilePrefs keeps the person's choices for a file they may play.
func (service *Service) SetFilePrefs(ctx context.Context, userID, fileID int64, prefs FilePrefs) error {
	if _, err := service.file(ctx, fileID); err != nil {
		return err
	}
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.SetPreference(ctx, sqlcgen.SetPreferenceParams{
			UserID: userID, Scope: scopeFile, ScopeID: fileID, Axis: axisSubtitleOffsetMs,
			Value: strconv.FormatInt(prefs.SubtitleOffsetMs, 10), SetAt: service.clock.Now().UnixMilli(),
		})
	})
	if err != nil {
		return fmt.Errorf("keep the preferences for file %d: %w", fileID, err)
	}
	return nil
}
