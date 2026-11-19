// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/platform/localtime"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// The log is written at the level logLevel names as soon as an admin sets it, with no restart, and
// at the config file's level while it says "file", its start (D347).
func TestKeepLogLevelCurrent_TheLogFollowsItsSettingAtOnce(t *testing.T) {
	t.Parallel()
	service := settingstest.Seeded(t, storetest.OpenMigrated(t))
	level := &slog.LevelVar{}
	level.Set(slog.LevelWarn)
	keepLogLevelCurrent(t.Context(), level, slog.LevelWarn, service, slog.New(slog.DiscardHandler))
	if level.Level() != slog.LevelWarn {
		t.Errorf("at its start the level is %v; want the file's, warn", level.Level())
	}
	settingstest.Set(t, service, settings.LogLevel, `"debug"`)
	if level.Level() != slog.LevelDebug {
		t.Errorf("set to debug, the level is %v", level.Level())
	}
	settingstest.Set(t, service, settings.LogLevel, `"error"`)
	if level.Level() != slog.LevelError {
		t.Errorf("set to error, the level is %v", level.Level())
	}
	settingstest.Set(t, service, settings.LogLevel, `"file"`)
	if level.Level() != slog.LevelWarn {
		t.Errorf("set back to file, the level is %v; want the file's, warn", level.Level())
	}
}

// The core's logger writes at the level leveledLogger hands back, so changing it changes what the
// log keeps (D347).
func TestLeveledLogger_WritesAtTheLevelItHandsBack(t *testing.T) {
	t.Parallel()
	var stderr strings.Builder
	config := Config{Log: LogConfig{Level: "info", Format: "text", Destination: logToStderr}}
	opened, err := invocation{stderr: &stderr}.leveledLogger(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := opened.closeLog(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	opened.logger.Debug("hidden")
	opened.level.Set(slog.LevelDebug)
	opened.logger.Debug("shown")
	if text := stderr.String(); strings.Contains(text, "hidden") || !strings.Contains(text, "shown") {
		t.Errorf("the log is\n%s\nwant only the line logged once the level was debug", text)
	}
}

// A core started with a debug log.level in its file logs at debug until logLevel names another:
// mountDiagnostics reads the file's level for "file" (D347).
func TestMountDiagnostics_TheFilesLevelHoldsUntilTheSettingNamesAnother(t *testing.T) {
	t.Parallel()
	service := settingstest.Seeded(t, storetest.OpenMigrated(t))
	level := &slog.LevelVar{}
	config := Config{Log: LogConfig{Level: "debug"}}
	if err := mountDiagnostics(t.Context(), nil, config, level, service, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	if level.Level() != slog.LevelDebug {
		t.Errorf("with log.level debug in the file, the level is %v", level.Level())
	}
	settingstest.Set(t, service, settings.LogLevel, `"info"`)
	if level.Level() != slog.LevelInfo {
		t.Errorf("set to info, the level is %v", level.Level())
	}
}

// A release's run hands the serve its opened log's level, which the logLevel setting then moves: wired
// to another, an admin's change of the setting reached nothing the log wrote at (D347).
func TestReleaseWiringFor_HandsTheServeTheLevelItsLogWritesAt(t *testing.T) {
	opened := processLog{level: &slog.LevelVar{}, recent: slogx.NewRecent(slogx.ProcessRecentLimits())}
	wired := releaseWiringFor(opened, localtime.Zone{})
	if wired.logLevel != opened.level {
		t.Fatalf("the serve's level is %p; want the opened log's, %p", wired.logLevel, opened.level)
	}
	// And the lines the log keeps, whatever its level, for a problem report (D524 §3).
	if wired.recent != opened.recent {
		t.Errorf("the serve's recent log is %p; want the opened log's, %p", wired.recent, opened.recent)
	}
}

// A node's log is written at the level its core names (D525 §1): "file" is the node's own config
// file's level, and a name that is no level leaves it as it was, saying why.
func TestNodeLogLevelSetter_TheNodesLogFollowsTheLevelItsCoreNames(t *testing.T) {
	t.Parallel()
	var written strings.Builder
	level := &slog.LevelVar{}
	level.Set(slog.LevelWarn)
	logger := slog.New(slog.NewTextHandler(&written, &slog.HandlerOptions{Level: level}))
	set := nodeLogLevelSetter(processLog{logger: logger, level: level, fileLevel: slog.LevelWarn})
	set(t.Context(), "debug")
	if level.Level() != slog.LevelDebug {
		t.Errorf("named debug, the node's level is %v", level.Level())
	}
	set(t.Context(), "loudest")
	if level.Level() != slog.LevelDebug || !strings.Contains(written.String(), "the node's log level stays as it was") {
		t.Errorf("named no level, the node's level is %v and its log\n%s\nwant debug kept and why", level.Level(),
			written.String())
	}
	set(t.Context(), settings.LogLevelFromFile)
	if level.Level() != slog.LevelWarn {
		t.Errorf("named file, the node's level is %v; want its file's, warn", level.Level())
	}
	if nodeLogLevelSetter(processLog{logger: logger}) != nil {
		t.Error("a log with no level to set has a setter; want none, which keeps the node's level")
	}
}
