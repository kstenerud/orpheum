// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/settings"
)

// A row a release retired is expected and logged as information, so a server does not warn of it at
// every start; a row no release read, most likely a typo, is still a warning (nas.example, 2026-11-01).
func TestLogUnusedSettings_WarnsOnlyOfARowNoReleaseRead(t *testing.T) {
	t.Parallel()
	var logged bytes.Buffer
	retired := settings.RetiredKey{Key: "scan_video_extensions", Release: "0.3.0", Reason: "moved to the library"}
	logUnusedSettings(t.Context(), []settings.UnusedSetting{
		{Key: "scan_video_extensions", ValueJSON: "[]", Retired: &retired},
		{Key: "scan_vidoe_extensions", ValueJSON: "[]"},
	}, slog.New(slog.NewJSONHandler(&logged, nil)))
	lines := strings.Split(strings.TrimSpace(logged.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("logged %q; want two lines", logged.String())
	}
	if !strings.Contains(lines[0], `"level":"INFO"`) || !strings.Contains(lines[0], `"retiredIn":"0.3.0"`) {
		t.Errorf("the retired row logged %s; want information naming its release", lines[0])
	}
	if !strings.Contains(lines[1], `"level":"WARN"`) || !strings.Contains(lines[1], `"key":"scan_vidoe_extensions"`) {
		t.Errorf("the unknown row logged %s; want a warning", lines[1])
	}
}
