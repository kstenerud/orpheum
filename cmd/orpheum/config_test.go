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
	template, err := os.ReadFile(filepath.Join("..", "..", "dev", "orpheum.dev.toml"))
	if err != nil {
		t.Fatal(err)
	}
	rendered := strings.NewReplacer("{root}", "/srv/orpheum", "{ffmpeg_dir}", "/opt/ffmpeg").Replace(string(template))
	config, unknown, err := loadConfig(writeConfig(t, rendered+"\n[server.extra]\nsurprise = 1\n"))
	if err != nil {
		t.Fatalf("the dev config does not load: %v", err)
	}
	if config.Server.DataDir != "/srv/orpheum/.dev/state" || config.Node.Confinement != "report" ||
		config.Log.Format != "text" {
		t.Errorf("the dev config read as %+v", config)
	}
	if len(unknown) != 1 || unknown[0] != "server.extra.surprise" {
		t.Errorf("unknown keys = %v, want the one reported, not fatal", unknown)
	}
}

func TestLoadConfig_RequiresEveryKeyAndHasNoDefaults(t *testing.T) {
	t.Parallel()
	if _, _, err := loadConfig(writeConfig(t, valid)); err != nil {
		t.Fatalf("a complete config: %v", err)
	}
	for _, key := range requiredKeys() {
		table, name := key[0], key[1]
		var kept []string
		section := ""
		for line := range strings.SplitSeq(valid, "\n") {
			if strings.HasPrefix(line, "[") {
				section = strings.Trim(line, "[]")
			}
			if section == table && strings.HasPrefix(line, name+" ") {
				continue
			}
			kept = append(kept, line)
		}
		_, _, err := loadConfig(writeConfig(t, strings.Join(kept, "\n")))
		if want := table + "." + name + " is required"; err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("without %s.%s: error = %v, want %q", table, name, err, want)
		}
	}
}

func TestLoadConfig_RefusesNamingTheKey(t *testing.T) {
	t.Parallel()
	cases := map[string]struct{ text, names string }{
		"relative state": {strings.Replace(valid, `data_dir = "/a"`, `data_dir = "state"`, 1), "server.data_dir"},
		"bad listen": {
			strings.Replace(valid, `listen = "127.0.0.1:8432"`, `listen = "nowhere"`, 1), "server.listen",
		},
		"bad confinement": {
			strings.Replace(valid, `confinement = "require"`, `confinement = "maybe"`, 1), "node.confinement",
		},
		"bad log level":    {strings.Replace(valid, `level = "info"`, `level = "loud"`, 1), "log.level"},
		"bad log format":   {strings.Replace(valid, `format = "json"`, `format = "xml"`, 1), "log format"},
		"not TOML at all":  {"server = [", "config"},
		"a wrong key type": {"[server]\nlisten = 8432\n", "listen"},
	}
	for name, tc := range cases {
		if _, _, err := loadConfig(writeConfig(t, tc.text)); err == nil || !strings.Contains(err.Error(), tc.names) {
			t.Errorf("%s: error = %v, want a refusal naming %s", name, err, tc.names)
		}
	}
}

const valid = `[server]
listen = "127.0.0.1:8432"
data_dir = "/a"
cache_dir = "/b"

[node]
roots = ["/media"]
ffmpeg_dir = "/opt/ffmpeg"
confinement = "require"

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
