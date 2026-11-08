// SPDX-License-Identifier: AGPL-3.0-or-later

package deployment

import (
	"path/filepath"
	"strings"
	"testing"
)

// A compose file's start writes the values file over the one there each time, from its command line,
// and its header says so, sending a change to the compose file; the tunnel's key is kept.
func TestWriteStartValues_IsWrittenEachTimeSayingTheComposeFileRules(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	values := StartingValues(Release)
	values.LAN.Address = "192.168.1.20"
	if err := WriteStartValues(dir, values); err != nil {
		t.Fatal(err)
	}
	private := readFile(t, filepath.Join(dir, privateKeyFile))
	values.LAN.Address = "192.168.1.30"
	values.LAN.HTTPSPort = 9443
	if err := WriteStartValues(dir, values); err != nil {
		t.Fatal(err)
	}
	text := readFile(t, filepath.Join(dir, ValuesFile))
	parsed, err := ParseValues(text, "nas")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.LAN.Address != "192.168.1.30" || parsed.LAN.HTTPSPort != 9443 {
		t.Errorf("the values file after a second start says %+v; want the second start's address and port", parsed.LAN)
	}
	if !strings.HasPrefix(text, startValuesHeader) || strings.Contains(text, "Fill in every value") {
		t.Errorf("the values file begins %q; want the start's header, which sends a change to the compose file, "+
			"and not the template's, which asks for edits", text[:min(len(text), 300)])
	}
	if readFile(t, filepath.Join(dir, privateKeyFile)) != private {
		t.Error("a second start made a new tunnel key; want the first kept")
	}
}
