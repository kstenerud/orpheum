// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/kstenerud/orpheum/internal/core/settings"
)

// mountDiagnostics keeps the log's level as its setting says (keepLogLevelCurrent) and mounts the
// debug endpoints their settings ask for (mountDebug).
func mountDiagnostics(
	ctx context.Context, mux *http.ServeMux, config Config, level *slog.LevelVar, settingsService *settings.Service,
	logger *slog.Logger,
) error {
	fileLevel, err := parseLevel(config.Log.Level)
	if err != nil {
		return err
	}
	keepLogLevelCurrent(ctx, level, fileLevel, settingsService, logger)
	return mountDebug(ctx, mux, config, settingsService, logger)
}

// keepLogLevelCurrent writes the log at the level the log_level setting names, now and at each
// change, with no restart (D347): its "file" is fileLevel, the config file's log.level. A value that
// cannot be read leaves the level as it was, and says so. A nil level, a test's, is left alone.
func keepLogLevelCurrent(
	ctx context.Context, level *slog.LevelVar, fileLevel slog.Level, settingsService *settings.Service,
	logger *slog.Logger,
) {
	if level == nil || settingsService == nil {
		return
	}
	apply := func(ctx context.Context) error {
		value, err := settingsService.String(ctx, settings.LogLevel)
		if err != nil {
			return fmt.Errorf("read the log's level: %w", err)
		}
		chosen := fileLevel
		if value != settings.LogLevelFromFile {
			if chosen, err = parseLevel(value); err != nil {
				return err
			}
		}
		if chosen != level.Level() {
			// Said at the level it leaves, which the log still writes.
			logger.Log(ctx, level.Level(), "the log's level changes", "from", level.Level().String(),
				"to", chosen.String(), "setting", value)
			level.Set(chosen)
		}
		return nil
	}
	if err := apply(ctx); err != nil {
		logger.ErrorContext(ctx, "the log's level stays as it was", "err", err)
	}
	settingsService.OnChange(settings.LogLevel, apply)
}
