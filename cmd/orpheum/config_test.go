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
		!config.Debug.PProf || config.Log.Format != "text" {
		t.Errorf("the dev config read as %+v", config)
	}
	if len(unknown) != 1 || unknown[0] != "server.extra.surprise" {
		t.Errorf("unknown keys = %v, want the one reported, not fatal", unknown)
	}
}

func TestLoadConfig_DefaultsAreTheSafeOnes(t *testing.T) {
	t.Parallel()
	config, _, err := loadConfig(writeConfig(t,
		"[server]\nlisten = \"127.0.0.1:8432\"\ndata_dir = \"/srv/state\"\ncache_dir = \"/srv/cache\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if config.Node.Confinement != "require" || config.Log.Level != "info" || config.Log.Format != "json" ||
		config.Debug.PProf || config.Debug.Metrics {
		t.Errorf("defaults = %+v; want confinement required, info, json, and no debug endpoints", config)
	}
}

func TestLoadConfig_RefusesNamingTheKey(t *testing.T) {
	t.Parallel()
	cases := map[string]struct{ text, names string }{
		"no listen": {"[server]\ndata_dir = \"/a\"\ncache_dir = \"/b\"\n", "server.listen"},
		"relative state": {
			"[server]\nlisten = \":1\"\ndata_dir = \"state\"\ncache_dir = \"/b\"\n", "server.data_dir",
		},
		"bad listen": {
			"[server]\nlisten = \"nowhere\"\ndata_dir = \"/a\"\ncache_dir = \"/b\"\n", "server.listen",
		},
		"bad confinement":  {valid + "[node]\nconfinement = \"maybe\"\n", "node.confinement"},
		"bad log level":    {valid + "[log]\nlevel = \"loud\"\n", "log.level"},
		"bad log format":   {valid + "[log]\nformat = \"xml\"\n", "log format"},
		"not TOML at all":  {"server = [", "config"},
		"a wrong key type": {"[server]\nlisten = 8432\n", "listen"},
	}
	for name, tc := range cases {
		if _, _, err := loadConfig(writeConfig(t, tc.text)); err == nil || !strings.Contains(err.Error(), tc.names) {
			t.Errorf("%s: error = %v, want a refusal naming %s", name, err, tc.names)
		}
	}
}

const valid = "[server]\nlisten = \"127.0.0.1:8432\"\ndata_dir = \"/a\"\ncache_dir = \"/b\"\n"

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
