// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/internal/core/certificate"
	"github.com/kstenerud/orpheum/internal/core/home"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
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
		"an install route neither the script's nor a compose file's": {
			kindCore, strings.Replace(core, `install_route = ""`, `install_route = "helm"`, 1),
			"server.install_route",
		},
		"a compose file's version below none's": {
			kindCore, strings.Replace(core, "compose_version = 0", "compose_version = -1", 1),
			"server.compose_version",
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
host_names = []
home_networks = ["127.0.0.0/8", "::1/128"]
not_home_networks = []
address_at_home_from_default_route = false
this_machine_listen = ""
install_route = ""
compose_version = 0
image_tag = ""

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
configured_addresses = []
address_at_home_seed = ""
send_interface_addresses = true
house_ipv6_networks = []
router = ""

[webrtc]
udp_port = 0
stun_servers = []
max_connections = 64

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
probe_timeout_seconds = 60
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

// The most WebRTC connections the core holds is the config's [webrtc] max_connections, its starting
// value when the file names none, and a value under one is refused (D348).
func TestLoadConfig_TheWebRTCConnectionLimitIsTheFilesOrItsStartingValue(t *testing.T) {
	t.Parallel()
	core := validConfigTexts[kindCore]
	named := strings.Replace(core, "max_connections = 64", "max_connections = 7", 1)
	config, _, err := loadConfig(writeConfig(t, named), kindCore)
	if err != nil || config.WebRTC.MaxConnections != 7 {
		t.Errorf("with 7 named: %d, %v; want 7", config.WebRTC.MaxConnections, err)
	}
	unnamed := strings.Replace(core, "max_connections = 64\n", "", 1)
	if config, _, err = loadConfig(writeConfig(t, unnamed), kindCore); err != nil ||
		config.WebRTC.MaxConnections != rtcapi.DefaultMaxConnections {
		t.Errorf("with none named: %d, %v; want %d", config.WebRTC.MaxConnections, err,
			rtcapi.DefaultMaxConnections)
	}
	zero := strings.Replace(core, "max_connections = 64", "max_connections = 0", 1)
	if _, _, err = loadConfig(writeConfig(t, zero), kindCore); err == nil {
		t.Error("with 0 named: loaded; want it refused")
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

// The seed of the address at home is a private IPv4 address or none, and is no configured candidate:
// the heartbeat sends the address at home as the keeper holds it (D462 §10.6); a public, loopback,
// IPv6 or ported one is refused.
func TestDiscoveryConfig_TheAddressAtHomeSeedIsAPrivateIPv4Address(t *testing.T) {
	t.Parallel()
	discovery := DiscoveryConfig{ConfiguredAddresses: []string{"11.22.44.146:443"}, AddressAtHomeSeed: "192.168.1.199"}
	if err := discovery.check(); err != nil {
		t.Fatal(err)
	}
	if got := discovery.addressAtHomeSeed(); got != netip.MustParseAddr("192.168.1.199") {
		t.Errorf("the seed is %v; want 192.168.1.199", got)
	}
	if got := discovery.configuredCandidates(); len(got) != 1 || got[0].Address != "11.22.44.146" {
		t.Errorf("the configured candidates are %+v; want the public one alone", got)
	}
	for _, refused := range []string{"11.22.44.146", "127.0.0.1", "192.168.1.20:8443", "fd00::1"} {
		discovery.AddressAtHomeSeed = refused
		if discovery.check() == nil {
			t.Errorf("seed %q was taken", refused)
		}
	}
}

// The house's IPv6 networks an admin names are public IPv6 /64s, at most four, which the discovery
// is sent as configured; a wider, private, IPv4 or unmasked network, and a fifth, are refused (D356 §6).
func TestDiscoveryConfig_HouseIPv6NetworksArePublicSlash64s(t *testing.T) {
	t.Parallel()
	discovery := DiscoveryConfig{HouseIPv6Networks: []string{"2a0f:5000:1:2::/64", "2a0f:3000:9:8::/64"}}
	if err := discovery.check(); err != nil {
		t.Fatal(err)
	}
	got := discovery.houseNetworks()
	if len(got) != 2 || got[0].String() != "2a0f:5000:1:2::/64" || got[1].String() != "2a0f:3000:9:8::/64" {
		t.Errorf("the house networks are %v; want both named", got)
	}
	for _, refused := range []string{
		"2a0f:5000::/32", "fd77:1:2:3::/64", "2001:db8:1:2::/64", "11.22.33.0/24", "2a0f:5000:1:2::9/64",
		"2a0f:5000:1:2::1",
	} {
		discovery.HouseIPv6Networks = []string{refused}
		if discovery.check() == nil {
			t.Errorf("house network %q was taken", refused)
		}
	}
	discovery.HouseIPv6Networks = []string{
		"2a0f:5000:1:1::/64", "2a0f:5000:1:2::/64", "2a0f:5000:1:3::/64", "2a0f:5000:1:4::/64", "2a0f:5000:1:5::/64",
	}
	if discovery.check() == nil {
		t.Error("five house networks were taken; a discovery counts four")
	}
}

// A configured address is a public address with its port, sent as a configured candidate; the
// machine's own or a documentation address is refused (the protocol's §2.4).
func TestDiscoveryConfig_ConfiguredAddressesArePublicAndSentAsConfigured(t *testing.T) {
	t.Parallel()
	discovery := DiscoveryConfig{
		ConfiguredAddresses: []string{"11.22.44.146:443", "[2a0f:2000:1c16:2684::3]:8443"},
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

// The networks an admin administers from are networks, read as such; anything else is refused by
// name (D297 §6).
func TestServerConfig_HomeNetworksAreNetworks(t *testing.T) {
	t.Parallel()
	server := ServerConfig{
		Listen: "0.0.0.0:8440", ReadHeaderTimeoutSeconds: 10, ShutdownGraceSeconds: 10,
		HomeNetworks: []string{"192.168.1.7/24", "fd00::/8"},
	}
	if err := server.check(); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(server.homeNetworks()); got != "[192.168.1.0/24 fd00::/8]" {
		t.Errorf("the home networks read as %s; want each network, masked", got)
	}
	server.HomeNetworks = []string{"192.168.1.7"}
	if err := server.check(); err == nil || !strings.Contains(err.Error(), "server.home_networks") {
		t.Errorf("an address alone = %v; want refused by server.home_networks", err)
	}
}

// The networks never home are networks too; anything else is refused by name (D452 §1).
func TestServerConfig_NotHomeNetworksAreNetworks(t *testing.T) {
	t.Parallel()
	server := ServerConfig{
		Listen: "0.0.0.0:8440", ReadHeaderTimeoutSeconds: 10, ShutdownGraceSeconds: 10,
		NotHomeNetworks: []string{"10.77.0.1/24", "fd77::/64"},
	}
	if err := server.check(); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(server.notHomeNetworks()); got != "[10.77.0.0/24 fd77::/64]" {
		t.Errorf("the networks never home read as %s; want each network, masked", got)
	}
	server.NotHomeNetworks = []string{"10.77.0.1"}
	if err := server.check(); err == nil || !strings.Contains(err.Error(), "server.not_home_networks") {
		t.Errorf("an address alone = %v; want refused by server.not_home_networks", err)
	}
}

// Every private network is home but those not_home_networks names, whatever home_networks lists, and a
// listed network is home beside them under the same exceptions (D452 §1; the owner, 2026-11-09).
func TestHomeRuleOf_HomeIsEveryPrivateNetworkButNotHomeNetworks(t *testing.T) {
	t.Parallel()
	var config Config
	config.Server.HomeNetworks = []string{"100.64.5.0/24", "10.77.0.0/24"}
	config.Server.NotHomeNetworks = []string{"10.77.0.0/24"}
	isHome := func(address string) bool { return homeRuleOf(config).Contains(netip.MustParseAddr(address)) }
	for address, want := range map[string]bool{
		"192.168.1.20": true, "192.168.7.20": true, "100.64.5.7": true, "127.0.0.1": true,
		"10.77.0.3": false, "100.64.6.7": false, "203.0.113.9": false,
	} {
		if got := isHome(address); got != want {
			t.Errorf("%s is home: %t; want %t", address, got, want)
		}
	}
}

// The core's own STUN servers are stun: URLs; a host alone, or a TURN server, which a discovery names
// and the core takes from it, is refused (the Plex-chain review's L3).
func TestWebRTCConfig_STUNServersAreSTUNURLs(t *testing.T) {
	t.Parallel()
	webrtc := WebRTCConfig{
		STUNServers:    []string{"stun:stun.cloudflare.com:3478", "stuns:stun.example.org:5349"},
		MaxConnections: 64,
	}
	if err := webrtc.check(); err != nil {
		t.Fatal(err)
	}
	for _, wrong := range []string{"stun.cloudflare.com:3478", "turn:turn.example.org:3478"} {
		webrtc.STUNServers = []string{wrong}
		if err := webrtc.check(); err == nil || !strings.Contains(err.Error(), "webrtc.stun_servers") {
			t.Errorf("STUN server %q = %v; want refused by webrtc.stun_servers", wrong, err)
		}
	}
}

// The core holds at least one WebRTC connection: a limit of none or fewer is refused (D348).
func TestWebRTCConfig_MaxConnectionsAllowsOne(t *testing.T) {
	t.Parallel()
	for limit, wantRefused := range map[int]bool{-1: true, 0: true, 1: false, 64: false} {
		err := WebRTCConfig{MaxConnections: limit}.check()
		if refused := err != nil && strings.Contains(err.Error(), "webrtc.max_connections"); refused != wantRefused {
			t.Errorf("max_connections %d: check = %v; want refused %v", limit, err, wantRefused)
		}
	}
}

// The WebRTC server the core serves holds as many connections as its config's
// webrtc.max_connections says, not the package's default (D348).
func TestNewWebRTC_HoldsAsManyConnectionsAsTheConfigSays(t *testing.T) {
	t.Parallel()
	rtc := newWebRTC(t.Context(), http.NotFoundHandler(), WebRTCConfig{MaxConnections: 5},
		slog.New(slog.DiscardHandler))
	if got := rtc.MaxConnections(); got != 5 {
		t.Errorf("with max_connections = 5, the server holds %d", got)
	}
}

// A config that names no networks starts on none listed, and home is then the private ranges but a
// network of the core's own interfaces: never Docker's bridge, where docker-proxy would make the
// internet look like home (the Plex-chain review's L1, D452 §1).
func TestServerConfig_StartsOnNoHomeNetworkAProxyOrTunnelSpeaksFrom(t *testing.T) {
	t.Parallel()
	var starting string
	for _, key := range serverConfigKeys() {
		if key.name == "home_networks" {
			starting = key.startingValue
		}
	}
	var parsed struct {
		Networks []string `toml:"networks"`
	}
	if _, err := toml.Decode("networks = "+starting, &parsed); err != nil {
		t.Fatalf("home_networks' starting value %q: %v", starting, err)
	}
	if len(parsed.Networks) != 0 {
		t.Errorf("home_networks' starting value is %s; want none listed", starting)
	}
	server := ServerConfig{HomeNetworks: parsed.Networks}
	rule := home.NewRule(server.homeNetworksOf([]netip.Prefix{netip.MustParsePrefix("172.17.0.0/16")}))
	for address, want := range map[string]bool{"172.17.0.1": false, "192.168.1.20": true, "203.0.113.9": false} {
		if got := rule.Contains(netip.MustParseAddr(address)); got != want {
			t.Errorf("with home_networks' starting value, %s is home: %t; want %t", address, got, want)
		}
	}
}

// A play's GET queues the chunks it plays on purpose, which the store's tests allow; the server
// warned of it on every play (the production NAS, 2026-10-23). Only a write no reading request may make warns.
func TestLoggingReaderWrites_WarnsOnlyOfAWriteNoReadingRequestMayMake(t *testing.T) {
	t.Parallel()
	var logged bytes.Buffer
	logWrite := loggingReaderWrites(t.Context(), slog.New(slog.NewTextHandler(&logged,
		&slog.HandlerOptions{Level: slog.LevelDebug})))
	logWrite("cache.db", "github.com/kstenerud/orpheum/internal/core/chunks.(*Service).storeIDOf")
	if text := logged.String(); !strings.Contains(text, "level=DEBUG") || strings.Contains(text, "level=WARN") ||
		!strings.Contains(text, "chunk store") {
		t.Errorf("an allowed write logged %q; want it at debug, with why", text)
	}
	logged.Reset()
	logWrite("persistent.db", "github.com/kstenerud/orpheum/internal/core/auth.(*Service).Authenticate")
	if text := logged.String(); !strings.Contains(text, "level=WARN") || !strings.Contains(text, "Authenticate") {
		t.Errorf("a write no reading request may make logged %q; want a warning naming it", text)
	}
}
