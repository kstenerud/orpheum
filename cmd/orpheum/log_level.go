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

// keepLogLevelCurrent writes the log at the level the logLevel setting names, now and at each
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
		return setLogLevel(ctx, level, fileLevel, value, logger)
	}
	if err := apply(ctx); err != nil {
		logger.ErrorContext(ctx, "the log's level stays as it was", "err", err)
	}
	settingsService.OnChange(settings.LogLevel, apply)
}

// setLogLevel writes the log at the level named, a logLevel setting's value, whose "file" is
// fileLevel, the config file's log.level; the change is logged at the level it leaves, which the log
// still writes. A name that is no level leaves the level as it was.
func setLogLevel(
	ctx context.Context, level *slog.LevelVar, fileLevel slog.Level, name string, logger *slog.Logger,
) error {
	chosen := fileLevel
	if name != settings.LogLevelFromFile {
		var err error
		if chosen, err = parseLevel(name); err != nil {
			return err
		}
	}
	if chosen != level.Level() {
		logger.Log(ctx, level.Level(), "the log's level changes", "from", level.Level().String(),
			"to", chosen.String(), "setting", name)
		level.Set(chosen)
	}
	return nil
}

// nodeLogLevelSetter writes a node's log at the level its core names (D525 §1), "file" being the
// node's config file's; nil for a log with no level to set, a test's.
func nodeLogLevelSetter(log processLog) func(ctx context.Context, name string) {
	if log.level == nil {
		return nil
	}
	return func(ctx context.Context, name string) {
		if err := setLogLevel(ctx, log.level, log.fileLevel, name, log.logger); err != nil {
			log.logger.ErrorContext(ctx, "the node's log level stays as it was", "err", err)
		}
	}
}
