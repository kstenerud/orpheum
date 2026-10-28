// SPDX-License-Identifier: AGPL-3.0-or-later

package settings

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// A page with no client settings, a Watch Together guest's, waits by the SDK's own margin, so its
// copy is the setting's starting value, as the link share's is (the owner, 2026-10-28).
func TestRegistry_TheWebPlayersMarginBeforeTheServerSaysIsTheReadyAheadSettingsStart(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "packages", "sdk", "src", "ready-ahead.ts"))
	if err != nil {
		t.Fatal(err)
	}
	margin := regexp.MustCompile(`export const readyAheadOfNextChunkPercent = (\d+);`).FindSubmatch(source)
	if margin == nil {
		t.Fatal("ready-ahead.ts has no readyAheadOfNextChunkPercent this test can read; say where the copy went")
	}
	web, parseErr := strconv.ParseInt(string(margin[1]), 10, 64)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	if start := StartingInt(ReadyAheadOfNextChunkPercent); web != start {
		t.Errorf("the web player waits for %d%% of its next chunk's load before the server says; the setting "+
			"starts at %d%%", web, start)
	}
}
