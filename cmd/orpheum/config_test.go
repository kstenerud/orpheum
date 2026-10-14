// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/internal/core/certificate"
)

func writeConfig(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "orpheum.toml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil { //nolint:gosec // G703: a path under t.TempDir()
		t.Fatal(err)
	}
	return path
}

func TestLoadConfig_ReadsTheDevFilesAndReportsUnknownKeys(t *testing.T) {
	t.Parallel()
	for kind, name := range map[instanceKind]string{kindCore: "core.dev.toml", kindNode: "node.dev.toml"} {
		template, err := os.ReadFile(filepath.Join("..", "..", "deploy", "dev", name))
		if err != nil {
			t.Fatal(err)
		}
		rendered := strings.NewReplacer("{root}", "/srv/orpheum", "{ffmpeg_dir}", "/opt/ffmpeg").
			Replace(string(template))
		config, report, err := loadConfig(writeConfig(t, rendered+"\n[log.extra]\nsurprise = 1\n"), kind)
		if err != nil {
			t.Fatalf("the dev %s config does not load: %v", kind, err)
		}
		if config.Log.Format != "text" {
			t.Errorf("the dev %s config read as %+v", kind, config)
		}
		if kind == kindNode && (config.Node.ConfinementPolicy != "report" ||
			config.Node.CoreSocket != "/srv/orpheum/.dev/core/run/core.sock") {
			t.Errorf("the dev node config read as %+v", config.Node)
		}
		if len(report.Unknown) != 1 || report.Unknown[0] != "log.extra.surprise" || len(report.OnStartingValue) != 0 {
			t.Errorf("%s: report = %+v, want the one unknown key reported, not fatal", kind, report)
		}
	}
}

func TestLoadConfig_RunsAMissingKeyOnItsStartingValueOrRefusesOneWithNone(t *testing.T) {
	t.Parallel()
	for kind, text := range validConfigTexts {
		complete, report, err := loadConfig(writeConfig(t, text), kind)
		if err != nil || len(report.OnStartingValue) != 0 {
			t.Fatalf("a complete %s config: %+v, %v", kind, report, err)
		}
		for _, key := range configKeys(kind) {
			checkWithoutKey(t, kind, text, key)
		}
		if kind == kindCore && complete.Database.ReadConnectionsMinimum != 4 {
			t.Errorf("the complete config read read_connections_minimum as %d",
				complete.Database.ReadConnectionsMinimum)
		}
	}
}

// checkWithoutKey loads text of kind without key: refused when the key has no starting value,
// else loaded on it, reported, and the file not rewritten.
func checkWithoutKey(t *testing.T, kind instanceKind, text string, key configKey) {
	t.Helper()
	path := writeConfig(t, withoutKey(text, key))
	loaded, report, err := loadConfig(path, kind)
	if key.startingValue == "" {
		want := key.path() + " is required and has no starting value"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("without %s: error = %v, want %q", key.path(), err, want)
		}
		return
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
	core, node := validConfigTexts[kindCore], validConfigTexts[kindNode]
	cases := map[string]struct {
		kind        instanceKind
		text, names string
	}{
		"bad listen": {
			kindCore, strings.Replace(core, `listen = "127.0.0.1:8432"`, `listen = "nowhere"`, 1), "server.listen",
		},
		"relative core socket": {
			kindNode, strings.Replace(node, `core_socket = "/srv/core/run/core.sock"`, `core_socket = "core.sock"`, 1),
			"node.core_socket",
		},
		"bad confinement": {
			kindNode, strings.Replace(node, `confinement_policy = "require"`, `confinement_policy = "maybe"`, 1),
			"node.confinement_policy",
		},
		"a probe the core would not wait for": {
			kindNode, strings.Replace(node, "probe_timeout_seconds = 60", "probe_timeout_seconds = 1500", 1),
			"node.probe_timeout_seconds",
		},
		"bad log level":  {kindCore, strings.Replace(core, `level = "info"`, `level = "loud"`, 1), "log.level"},
		"bad log format": {kindCore, strings.Replace(core, `format = "json"`, `format = "xml"`, 1), "log format"},
		"relative log": {
			kindNode, strings.Replace(node, `destination = "stderr"`, `destination = "orpheum.log"`, 1),
			"log.destination",
		},
		"a relay address that is not an IP address": {
			kindCore, strings.Replace(core, `public_address = ""`, `public_address = "vps.example.org"`, 1),
			"turn.public_address",
		},
		"a relay port range the wrong way round": {
			kindCore, strings.Replace(strings.Replace(core, `public_address = ""`, `public_address = "203.0.113.7"`, 1),
				"relay_port_min = 49160", "relay_port_min = 49200", 1),
			"turn.relay_port_min",
		},
		"an ACME directory not over HTTPS": {
			kindCore, strings.NewReplacer(`domain = ""`, `domain = "media.example.org"`,
				`contact = ""`, `contact = "owner@example.org"`,
				`acme_directory = "https://`, `acme_directory = "http://`).Replace(core),
			"https.acme_directory",
		},
		"not TOML at all":  {kindCore, "server = [", "config"},
		"a wrong key type": {kindCore, "[server]\nlisten = 8432\n", "listen"},
	}
	for name, tc := range cases {
		if _, _, err := loadConfig(writeConfig(t, tc.text), tc.kind); err == nil ||
			!strings.Contains(err.Error(), tc.names) {
			t.Errorf("%s: error = %v, want a refusal naming %s", name, err, tc.names)
		}
	}
}

// validConfigTexts are a complete full config of each kind.
var validConfigTexts = map[instanceKind]string{
	kindCore: `[server]
listen = "127.0.0.1:8432"
read_header_timeout_seconds = 10
shutdown_grace_seconds = 10

[https]
domain = ""
contact = ""
listen = ":443"
acme_directory = "https://acme-v02.api.letsencrypt.org/directory"

[turn]
public_address = ""
listen = ":3478"
relay_port_min = 49160
relay_port_max = 49199

[discovery]
retry_first_milliseconds = 1000
retry_most_milliseconds = 300000
configured_addresses = []
send_interface_addresses = true

[database]
read_connections_minimum = 4
migrate_free_space_db_multiple = 3
when_unmigratable = "refuse"

[log]
level = "info"
format = "json"
destination = "stderr"
`,
	kindNode: `[node]
name = "local"
core_socket = "/srv/core/run/core.sock"
root_paths = ["/media"]
ffmpeg_dir = "/opt/ffmpeg"
confinement_policy = "require"
max_errors_per_root = 1000
probe_timeout_seconds = 60
status_interval_seconds = 10
progress_every_seconds = 2
ended_work_kept_hours = 168
read_header_timeout_seconds = 10
prepared_budget_gib = 1
prepared_kept_after_play_hours = 72
prepared_watched_minutes = 10
prepared_disk_used_limit_percent = 95
prepared_disk_min_free_mib = 100
encode_slot_count = 1
remux_slot_count = 2
slowest_read_mib_per_second = 20
watch_quiet_seconds = 30

[log]
level = "info"
format = "json"
destination = "stderr"
`,
}

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

// The certificate comes from Let's Encrypt's production directory unless the file names another, as
// staging may name Let's Encrypt's staging one (D261 §5).
func TestLoadConfig_TheACMEDirectoryIsTheFilesLetsEncryptsByDefault(t *testing.T) {
	t.Parallel()
	core := validConfigTexts[kindCore]
	const stagingCA = "https://acme-staging-v02.api.letsencrypt.org/directory"
	named := strings.Replace(core, certificate.LetsEncrypt, stagingCA, 1)
	config, _, err := loadConfig(writeConfig(t, named), kindCore)
	if err != nil || config.HTTPS.ACMEDirectory != stagingCA {
		t.Errorf("with the staging CA named: %q, %v; want it", config.HTTPS.ACMEDirectory, err)
	}
	unnamed := strings.Replace(core, `acme_directory = "`+certificate.LetsEncrypt+`"`+"\n", "", 1)
	if config, _, err = loadConfig(writeConfig(t, unnamed), kindCore); err != nil ||
		config.HTTPS.ACMEDirectory != certificate.LetsEncrypt {
		t.Errorf("with none named: %q, %v; want Let's Encrypt's", config.HTTPS.ACMEDirectory, err)
	}
}

// A node whose kernel lacks Landlock or seccomp runs and says so unless its admin asks it to refuse:
// confinement is a hardening, not a requirement (D273 §5).
func TestConfinementPolicy_StartsAtReport(t *testing.T) {
	t.Parallel()
	for _, key := range configKeys(kindNode) {
		if key.name == "confinement_policy" {
			if key.startingValue != `"report"` {
				t.Errorf("node.confinement_policy starts at %s, want \"report\"", key.startingValue)
			}
			return
		}
	}
	t.Error("the node has no confinement_policy key")
}

func TestDiscoveryConfig_RefusesAWaitThatIsNoneAndAMostShorterThanTheFirst(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		first, most int64
		isRefused   bool
	}{
		{1000, 300000, false},
		{1000, 1000, false},
		{0, 1000, true},
		{2000, 1000, true},
	} {
		err := DiscoveryConfig{RetryFirstMilliseconds: tc.first, RetryMostMilliseconds: tc.most}.check()
		if (err != nil) != tc.isRefused {
			t.Errorf("first %d ms, most %d ms: %v; want refused: %t", tc.first, tc.most, err, tc.isRefused)
		}
	}
}

// A configured address is a public address with its port, sent as a configured candidate; the
// machine's own or a documentation address is refused (the protocol's §2.4).
func TestDiscoveryConfig_ConfiguredAddressesArePublicAndSentAsConfigured(t *testing.T) {
	t.Parallel()
	discovery := DiscoveryConfig{
		RetryFirstMilliseconds: 1000, RetryMostMilliseconds: 1000,
		ConfiguredAddresses: []string{"11.22.44.146:443", "[2a0f:4000:1c16:2684::3]:8443"},
	}
	if err := discovery.check(); err != nil {
		t.Fatal(err)
	}
	got := discovery.configuredCandidates()
	if len(got) != 2 || got[0].Address != "11.22.44.146" || got[1].Port != 8443 ||
		got[1].Source != discoveryproto.SourceConfigured {
		t.Errorf("the configured candidates are %+v; want both, as configured", got)
	}
	for _, refused := range []string{"192.168.1.20:443", "203.0.113.7:443", "11.22.44.146", "[::1]:443"} {
		discovery.ConfiguredAddresses = []string{refused}
		if discovery.check() == nil {
			t.Errorf("configured address %q was taken", refused)
		}
	}
}
