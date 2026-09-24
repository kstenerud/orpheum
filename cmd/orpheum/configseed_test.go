// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var seedTime = time.Date(2026, time.September, 24, 12, 0, 0, 0, time.UTC)

// partialConfig lacks every key that has a starting value, the whole [database] table, and one
// key with none; it has a comment and a typo worth keeping.
const partialConfig = `# My server. Keep this comment.
[server]
listen = "127.0.0.1:8432"   # loopback only
data_dir = "/srv/state"
cache_dir = "/srv/cache"

[node]
roots = ["/media"]
confinment = "report"

[log]
format = "text"
`

func TestSeedConfigFile_AddsWhatHasAStartingValueAndKeepsEverythingElse(t *testing.T) {
	t.Parallel()
	path := writeConfig(t, partialConfig)
	report, err := seedConfigFile(path, seedTime)
	if err != nil {
		t.Fatalf("seedConfigFile: %v", err)
	}
	want := "server.read_header_timeout_s server.shutdown_grace_s database.readers_minimum " +
		"database.migrate_space_factor node.confinement log.level"
	if written := keyPaths(report.Written); written != want {
		t.Errorf("written = %v, want %s", written, want)
	}
	if byHand := keyPaths(report.NeedsHand); byHand != "node.ffmpeg_dir" {
		t.Errorf("left for a person = %v, want node.ffmpeg_dir, which has no starting value", byHand)
	}
	if strings.Join(report.Unknown, " ") != "node.confinment" {
		t.Errorf("unknown = %v, want the typo node.confinment", report.Unknown)
	}

	seeded := string(readFile(t, path))
	for _, kept := range []string{
		"# My server. Keep this comment.", `listen = "127.0.0.1:8432"   # loopback only`,
		`confinment = "report"`, `format = "text"`,
	} {
		if !strings.Contains(seeded, kept) {
			t.Errorf("the seeded file lost %q:\n%s", kept, seeded)
		}
	}
	if !strings.Contains(seeded, "[database]\nreaders_minimum = 4"+seededMarker) {
		t.Errorf("the missing [database] table was not added with its starting values:\n%s", seeded)
	}
	if !strings.Contains(seeded, "cache_dir = \"/srv/cache\"\nread_header_timeout_s = 10"+seededMarker) {
		t.Errorf("a missing key was not added at the end of its table:\n%s", seeded)
	}
	if backup := string(readFile(t, report.Backup)); backup != partialConfig {
		t.Errorf("the copy of the file as it was = %q, want the original", backup)
	}

	withFFmpeg := strings.Replace(seeded, "[node]\n", "[node]\nffmpeg_dir = \"/opt/ffmpeg\"\n", 1)
	if err = os.WriteFile(path, []byte(withFFmpeg), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = loadConfig(path); err != nil {
		t.Errorf("seeded, with the one hand-set key added, the config does not load: %v", err)
	}
	again, err := seedConfigFile(path, seedTime.Add(time.Hour))
	if err != nil || len(again.Written) != 0 || again.Backup != "" {
		t.Errorf("seeding a complete file = %+v, %v; want nothing written and no copy", again, err)
	}
}

func TestSeedConfigFile_RefusesAFileThatIsNotTOMLAndChangesNothing(t *testing.T) {
	t.Parallel()
	path := writeConfig(t, "[server\nlisten = ")
	if _, err := seedConfigFile(path, seedTime); err == nil {
		t.Error("seedConfigFile accepted a file that does not parse")
	}
	if got := string(readFile(t, path)); got != "[server\nlisten = " {
		t.Errorf("a refused file was changed: %q", got)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Errorf("a refused seed left files beside the config: %v", entries)
	}
}

func keyPaths(keys []configKey) string {
	var paths []string
	for _, key := range keys {
		paths = append(paths, key.path())
	}
	return strings.Join(paths, " ")
}
