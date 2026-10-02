// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "orpheum.toml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil { //nolint:gosec // G703: a path under t.TempDir()
		t.Fatal(err)
	}
	return path
}

func TestLoadConfig_ReadsTheDevFileAndReportsUnknownKeys(t *testing.T) {
	t.Parallel()
	template, err := os.ReadFile(filepath.Join("..", "..", "deploy", "dev", "orpheum.dev.toml"))
	if err != nil {
		t.Fatal(err)
	}
	rendered := strings.NewReplacer("{root}", "/srv/orpheum", "{ffmpeg_dir}", "/opt/ffmpeg").Replace(string(template))
	config, report, err := loadConfig(writeConfig(t, rendered+"\n[server.extra]\nsurprise = 1\n"))
	if err != nil {
		t.Fatalf("the dev config does not load: %v", err)
	}
	if config.Server.DataDir != "/srv/orpheum/.dev/state" || config.Node.ConfinementPolicy != "report" ||
		config.Log.Format != "text" {
		t.Errorf("the dev config read as %+v", config)
	}
	if len(report.Unknown) != 1 || report.Unknown[0] != "server.extra.surprise" || len(report.OnStartingValue) != 0 {
		t.Errorf("report = %+v, want the one unknown key reported, not fatal", report)
	}
}

func TestLoadConfig_RunsAMissingKeyOnItsStartingValueOrRefusesOneWithNone(t *testing.T) {
	t.Parallel()
	complete, report, err := loadConfig(writeConfig(t, validConfigText))
	if err != nil || len(report.OnStartingValue) != 0 {
		t.Fatalf("a complete config: %+v, %v", report, err)
	}
	for _, key := range configKeys() {
		path := writeConfig(t, withoutKey(validConfigText, key))
		loaded, report, err := loadConfig(path)
		if key.startingValue == "" {
			want := key.path() + " is required and has no starting value"
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("without %s: error = %v, want %q", key.path(), err, want)
			}
			continue
		}
		if err != nil || len(report.OnStartingValue) != 1 || report.OnStartingValue[0].path() != key.path() {
			t.Errorf("without %s: %+v, %v; want it loaded on its starting value and reported", key.path(), report, err)
		}
		if got := string(readFile(t, path)); strings.Contains(got, startingValueMarker) {
			t.Errorf("loading without %s rewrote the file", key.path())
		}
		if key.path() == "log.level" && loaded.Log.Level != "info" {
			t.Errorf("without log.level, the level is %q, want its starting value", loaded.Log.Level)
		}
	}
	if complete.Database.ReadConnectionsMinimum != 4 {
		t.Errorf("the complete config read read_connections_minimum as %d", complete.Database.ReadConnectionsMinimum)
	}
}

// withoutKey removes one key's line from a config's text.
func withoutKey(text string, key configKey) string {
	var kept []string
	section := ""
	for line := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(line, "[") {
			section = strings.Trim(line, "[]")
		}
		if section == key.table && strings.HasPrefix(line, key.name+" ") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func TestLoadConfig_RefusesNamingTheKey(t *testing.T) {
	t.Parallel()
	cases := map[string]struct{ text, names string }{
		"relative state": {
			strings.Replace(validConfigText, `data_dir = "/a"`, `data_dir = "state"`, 1),
			"server.data_dir",
		},
		"bad listen": {
			strings.Replace(validConfigText, `listen = "127.0.0.1:8432"`, `listen = "nowhere"`, 1), "server.listen",
		},
		"bad confinement": {
			strings.Replace(validConfigText, `confinement_policy = "require"`, `confinement_policy = "maybe"`, 1),
			"node.confinement_policy",
		},
		"bad log level":    {strings.Replace(validConfigText, `level = "info"`, `level = "loud"`, 1), "log.level"},
		"bad log format":   {strings.Replace(validConfigText, `format = "json"`, `format = "xml"`, 1), "log format"},
		"not TOML at all":  {"server = [", "config"},
		"a wrong key type": {"[server]\nlisten = 8432\n", "listen"},
	}
	for name, tc := range cases {
		if _, _, err := loadConfig(writeConfig(t, tc.text)); err == nil || !strings.Contains(err.Error(), tc.names) {
			t.Errorf("%s: error = %v, want a refusal naming %s", name, err, tc.names)
		}
	}
}

const validConfigText = `[server]
listen = "127.0.0.1:8432"
data_dir = "/a"
cache_dir = "/b"
read_header_timeout_seconds = 10
shutdown_grace_seconds = 10
socket_dir = "/run/orpheum"

[database]
read_connections_minimum = 4
migrate_free_space_db_multiple = 3

[node]
name = "local"
root_paths = ["/media"]
ffmpeg_dir = "/opt/ffmpeg"
confinement_policy = "require"
video_extensions = ["mkv", "mp4"]
subtitle_extensions = ["srt"]
max_errors_per_root = 1000
probe_timeout_seconds = 60
status_interval_seconds = 10
progress_every_seconds = 2
ended_work_kept_hours = 168
prepared_dir = "/var/cache/orpheum/prepared"
prepared_budget_gib = 1
prepared_kept_after_play_hours = 72
prepared_disk_used_limit_percent = 95
encode_slot_count = 1
remux_slot_count = 2
slowest_read_mib_per_second = 20

[log]
level = "info"
format = "json"
`

func TestIsLoopback(t *testing.T) {
	t.Parallel()
	for listen, want := range map[string]bool{
		"127.0.0.1:8432": true, "[::1]:8432": true, "localhost:8432": true,
		"0.0.0.0:8432": false, "192.168.1.5:8432": false, ":8432": false, "nonsense": false,
	} {
		if got := isLoopback(listen); got != want {
			t.Errorf("isLoopback(%q) = %v, want %v", listen, got, want)
		}
	}
}
