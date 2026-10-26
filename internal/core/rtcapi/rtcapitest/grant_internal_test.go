// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapitest

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// The test client grants the core more of an answer as often as the browser's reader does, so what
// the tests see of the core's credit is what a browser sees (the audit of 2026-10-25, R3).
func TestGrantEvery_IsTheBrowsersReaders(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "..", "..", "web", "packages", "sdk", "src", "rtc-request.ts")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	found := regexp.MustCompile(`export const grantEvery = (\d+);`).FindSubmatch(source)
	if found == nil {
		t.Fatal("rtc-request.ts has no grantEvery this test can read; say where the reader's grant went")
	}
	if browsers, err := strconv.Atoi(string(found[1])); err != nil || browsers != grantEvery {
		t.Errorf("the browser's reader grants every %s messages (%v), the test client every %d; want the same",
			found[1], err, grantEvery)
	}
}
