// SPDX-License-Identifier: AGPL-3.0-or-later

package deployment

import (
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// fillIn is what the values file's template says of each value the owner must give.
const fillIn = "fill in"

// vpsKey is a WireGuard public key: 32 bytes in base64.
const vpsKey = "YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE="

// filledValues is a values file with every value given.
func filledValues(t *testing.T, kind Kind) Values {
	t.Helper()
	values, err := ParseValues(`deployment = "`+string(kind)+`"
domain = "media.orpheum-app.com"
contact = "owner@example.org"
[vps]
endpoint = "[2001:db8::7]:51820"
public_ipv4 = "203.0.113.7"
public_key = "`+vpsKey+`"
[media]
roots = ["/mnt/tank/media/video/movies", "/srv/a \"b\""]
[lan]
address = "192.168.1.20"
`, "nas")
	if err != nil {
		t.Fatal(err)
	}
	return values
}

// runtime runs orpheum:v0.1.0 on a machine with a GPU whose render node's group is 105.
var runtime = Runtime{Image: "orpheum:v0.1.0", UID: 1000, GID: 1001, RenderGID: 105}

// begun is a deployment's directory with its values file filled in and its instances' directories
// made, as `orpheum write-deployment` finds it the second time.
func begun(t *testing.T, kind Kind) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := Begin(dir, kind); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"core", "node"} {
		if err := os.MkdirAll(filepath.Join(dir, kind, "persistent"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// decodeTOML reads a written TOML file.
func decodeTOML(t *testing.T, path string) map[string]map[string]any {
	t.Helper()
	config := map[string]map[string]any{}
	if _, err := toml.DecodeFile(path, &config); err != nil {
		t.Fatalf("%s is not TOML: %v", path, err)
	}
	return config
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(text)
}

func TestValues_TheTemplateAsBegunSaysWhatToFillInAndNothingElse(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if _, err := Begin(dir, Release); err != nil {
		t.Fatal(err)
	}
	values, err := ParseValues(readFile(t, filepath.Join(dir, ValuesFile)), "nas")
	if err != nil {
		t.Fatal(err)
	}
	if values.Deployment != Release || values.Domain != "" || values.HasTunnel() || values.HTTP.Port != 8440 {
		t.Errorf("the begun values are %+v; want a release with no name of its own and no tunnel, on 8440 "+
			"(D314 §3)", values)
	}
	var keys []string
	for _, problem := range values.Problems() {
		keys = append(keys, strings.Fields(problem)[0])
	}
	// A household with no VPS gives only its media (D314 §3) and its address at home, which the install
	// script fills in where it can tell it (the Plex-chain review's L1).
	want := []string{"media.roots:", "lan.address"}
	if !slices.Equal(keys, want) {
		t.Errorf("a begun values file needs %v; want %v: what the owner fills in, and nothing else", keys, want)
	}
	marked := strings.Count(strings.ToLower(readFile(t, filepath.Join(dir, ValuesFile))), fillIn)
	if marked < len(want) {
		t.Errorf("the values file marks %d values %q; want each of the %d to fill in marked", marked, fillIn,
			len(want))
	}
	if _, err = Begin(dir, Release); err == nil {
		t.Error("Begin over a values file succeeded; want it refused")
	}
}

func TestValues_ProblemsNameEachKeyThatLacksAValue(t *testing.T) {
	t.Parallel()
	if problems := filledValues(t, Release).Problems(); len(problems) != 0 {
		t.Errorf("filled values have problems %v; want none", problems)
	}
	for _, tc := range []struct {
		name, change, wantKey string
	}{
		{"a deployment of no known kind", `deployment = "prod"`, "deployment"},
		{"an endpoint with no port", `vps.endpoint = "203.0.113.7"`, "vps.endpoint"},
		{"an endpoint by name", `vps.endpoint = "vps.example.org:51820"`, "vps.endpoint"},
		{"an IPv6 endpoint without brackets", `vps.endpoint = "2001:db8::7:51820"`, "vps.endpoint"},
		{"a public address that is IPv6", `vps.public_ipv4 = "2001:db8::7"`, "vps.public_ipv4"},
		{"a short key", `vps.public_key = "YWFh"`, "vps.public_key"},
		{"an MTU out of range", `vps.mtu = 9000`, "vps.mtu"},
		{"a relative media root", `media.roots = ["movies"]`, "media.roots"},
		{"the dev loop's port", `http.port = 8432`, "http.port"},
		{"Vite's port", `http.port = 5173`, "http.port"},
		{"a publish address by name", `http.publish = "lan"`, "http.publish"},
		{"a tunnel IPv4 outside the tunnel's subnet", `vps.tunnel_ipv4 = "10.78.0.3"`, "vps.tunnel_ipv4"},
		{"the VPS's own tunnel IPv4", `vps.tunnel_ipv4 = "10.77.0.1"`, "vps.tunnel_ipv4"},
		{"a tunnel IPv4 that is IPv6", `vps.tunnel_ipv4 = "fd77::3"`, "vps.tunnel_ipv4"},
		{"the VPS's own tunnel IPv6", `vps.tunnel_ipv6 = "fd77::1"`, "vps.tunnel_ipv6"},
		{"a tunnel IPv6 outside the tunnel's subnet", `vps.tunnel_ipv6 = "fd78::3"`, "vps.tunnel_ipv6"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			values := filledValues(t, Release)
			changed, err := ParseValues(tc.change, "nas")
			if err != nil {
				t.Fatal(err)
			}
			overlay(&values, changed, tc.change)
			problems := values.Problems()
			if len(problems) != 1 || !strings.HasPrefix(problems[0], tc.wantKey) {
				t.Errorf("problems = %v; want one naming %s", problems, tc.wantKey)
			}
		})
	}
}

// overlay copies onto values the one value change sets.
func overlay(values *Values, changed Values, change string) {
	switch strings.Fields(change)[0] {
	case "deployment":
		values.Deployment = changed.Deployment
	case "vps.endpoint":
		values.VPS.Endpoint = changed.VPS.Endpoint
	case "vps.public_ipv4":
		values.VPS.PublicIPv4 = changed.VPS.PublicIPv4
	case "vps.public_key":
		values.VPS.PublicKey = changed.VPS.PublicKey
	case "vps.mtu":
		values.VPS.MTU = changed.VPS.MTU
	case "media.roots":
		values.Media.Roots = changed.Media.Roots
	case "http.port":
		values.HTTP.Port = changed.HTTP.Port
	case "http.publish":
		values.HTTP.Publish = changed.HTTP.Publish
	case "vps.tunnel_ipv4":
		values.VPS.TunnelIPv4 = changed.VPS.TunnelIPv4
	case "vps.tunnel_ipv6":
		values.VPS.TunnelIPv6 = changed.VPS.TunnelIPv6
	}
}

func TestWrite_ACoreWithNoNameOfItsOwnServesNoHTTPSOfOne(t *testing.T) {
	t.Parallel()
	dir := begun(t, Staging)
	values := filledValues(t, Staging)
	values.Domain = ""
	if problems := values.Problems(); len(problems) != 0 {
		t.Errorf("a values file with no domain has problems %v; want none: its discovery names it", problems)
	}
	if err := Write(dir, values, runtime); err != nil {
		t.Fatal(err)
	}
	if got := decodeTOML(t, filepath.Join(dir, coreConfig))["https"]["domain"]; got != "" {
		t.Errorf("the core config's https.domain is %v; want empty, so it serves no HTTPS of a name of its own", got)
	}
}

func TestValues_TheMTUFollowsTheEndpointsFamilyUnlessTheOwnerSetsOne(t *testing.T) {
	t.Parallel()
	values := filledValues(t, Release)
	if got := values.TunnelMTU(); got != mtuOverIPv6 {
		t.Errorf("over IPv6 the MTU is %d, want %d", got, mtuOverIPv6)
	}
	values.VPS.Endpoint = "203.0.113.7:51820"
	if got := values.TunnelMTU(); got != mtuOverIPv4 {
		t.Errorf("over IPv4 the MTU is %d, want %d, room for DS-Lite", got, mtuOverIPv4)
	}
	values.VPS.MTU = 1300
	if got := values.TunnelMTU(); got != 1300 {
		t.Errorf("with the owner's MTU it is %d, want 1300", got)
	}
}

func TestValues_TheNodeIsNamedByTheValuesFileOrTheMachine(t *testing.T) {
	t.Parallel()
	named, err := ParseValues("[node]\nname = \"sandbox\"\n", "nas")
	if err != nil {
		t.Fatal(err)
	}
	unnamed, err := ParseValues("", "nas")
	if err != nil {
		t.Fatal(err)
	}
	if named.Node.Name != "sandbox" || unnamed.Node.Name != "nas" {
		t.Errorf("the node is named %q and, unnamed, %q; want sandbox and the machine's nas",
			named.Node.Name, unnamed.Node.Name)
	}
}

func TestWrite_RewritesTheFilesAndKeepsTheValuesAndKeys(t *testing.T) {
	t.Parallel()
	dir := begun(t, Release)
	values := filledValues(t, Release)
	if err := Write(dir, values, runtime); err != nil {
		t.Fatal(err)
	}
	private := readFile(t, filepath.Join(dir, privateKeyFile))
	valuesText := readFile(t, filepath.Join(dir, ValuesFile))
	if err := os.WriteFile(filepath.Join(dir, coreConfig), []byte("edited"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, values, Runtime{Image: "orpheum:v0.2.0", UID: 1000, GID: 1001, RenderGID: -1}); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(dir, privateKeyFile)) != private ||
		readFile(t, filepath.Join(dir, ValuesFile)) != valuesText {
		t.Error("writing again changed the tunnel's key or the values file; want both kept")
	}
	if config := readFile(t, filepath.Join(dir, coreConfig)); config == "edited" {
		t.Error("writing again kept an edited core config; want it rendered again")
	}
	if env := readFile(t, filepath.Join(dir, environmentFile)); !strings.Contains(env,
		"ORPHEUM_IMAGE=orpheum:v0.2.0\n") {
		t.Errorf(".env is %q; want it to name the new image", env)
	}
	for name, want := range map[string]os.FileMode{
		privateKeyFile: 0o600, tunnelConfig: 0o600, coreConfig: 0o600, nodeConfig: 0o600,
		publicKeyFile: 0o644, composeFile: 0o644, overrideFile: 0o644, environmentFile: 0o644,
	} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != want {
			t.Errorf("%s has mode %o, want %o", name, info.Mode().Perm(), want)
		}
	}
}

func TestWrite_NothingIsWrittenWhileTheValuesHaveProblems(t *testing.T) {
	t.Parallel()
	dir := begun(t, Release)
	values := filledValues(t, Release)
	values.Contact = ""
	err := Write(dir, values, runtime)
	if err == nil || !strings.Contains(err.Error(), "contact") {
		t.Errorf("Write = %v; want it refused, naming contact", err)
	}
	if _, err = os.Stat(filepath.Join(dir, composeFile)); err == nil {
		t.Error("a compose file was written; want nothing written")
	}
}

func TestWrite_TheConfigsServeTheNameRelayThroughTheVPSAndNameTheMedia(t *testing.T) {
	t.Parallel()
	dir := begun(t, Release)
	if err := Write(dir, filledValues(t, Release), runtime); err != nil {
		t.Fatal(err)
	}
	core := decodeTOML(t, filepath.Join(dir, coreConfig))
	for _, check := range []struct{ table, key, want string }{
		{"https", "domain", "media.orpheum-app.com"},
		{"https", "contact", "owner@example.org"},
		{"turn", "public_address", "203.0.113.7"},
		{"turn", "listen", "10.77.0.2:3478"},
		{"server", "listen", "0.0.0.0:8440"},
	} {
		if got := core[check.table][check.key]; got != check.want {
			t.Errorf("the core config's %s.%s is %v, want %s", check.table, check.key, got, check.want)
		}
	}
	node := decodeTOML(t, filepath.Join(dir, nodeConfig))["node"]
	roots, err := json.Marshal(node["root_paths"])
	if err != nil {
		t.Fatal(err)
	}
	if string(roots) != `["/mnt/tank/media/video/movies","/srv/a \"b\""]` || node["name"] != "nas" ||
		node["confinement_policy"] != "require" || node["core_socket"] != "/run/orpheum/core/core.sock" {
		// Confinement required: a node does not run FFmpeg unconfined (D273 §5 as revised by the
		// owner, 2026-10-20, the Plex-chain review's N2).
		t.Errorf("the node config is %v; want the media, the machine's name, confinement required and "+
			"the core's socket", node)
	}
	tunnel := readFile(t, filepath.Join(dir, tunnelConfig))
	for _, want := range []string{
		"PrivateKey = " + strings.TrimSpace(readFile(t, filepath.Join(dir, privateKeyFile))),
		"PublicKey = " + vpsKey, "Endpoint = [2001:db8::7]:51820",
	} {
		if !strings.Contains(tunnel, want) {
			t.Errorf("the tunnel config lacks %q", want)
		}
	}
	if strings.Contains(tunnel, "Address") {
		t.Error("the tunnel config names an Address; wg setconf's format has none, the addresses are tunnel.sh's")
	}
}

func TestWrite_ADeploymentTakesItsOwnTunnelAddressesForTheTunnelAndTURN(t *testing.T) {
	t.Parallel()
	dir := begun(t, Staging)
	values := filledValues(t, Staging)
	if values.VPS.TunnelIPv4 != "10.77.0.2" || values.VPS.TunnelIPv6 != "fd77::2" {
		t.Errorf("a values file naming no tunnel addresses has %s and %s; want the first deployment's, "+
			"10.77.0.2 and fd77::2", values.VPS.TunnelIPv4, values.VPS.TunnelIPv6)
	}
	changed, err := ParseValues("[vps]\ntunnel_ipv4 = \"10.77.0.3\"\ntunnel_ipv6 = \"fd77::3\"\n", "nas")
	if err != nil {
		t.Fatal(err)
	}
	values.VPS.TunnelIPv4, values.VPS.TunnelIPv6 = changed.VPS.TunnelIPv4, changed.VPS.TunnelIPv6
	if err = Write(dir, values, runtime); err != nil {
		t.Fatal(err)
	}
	if got := decodeTOML(t, filepath.Join(dir, coreConfig))["turn"]["listen"]; got != "10.77.0.3:3478" {
		t.Errorf("the core config's turn.listen is %v; want the deployment's own 10.77.0.3:3478", got)
	}
	env := readFile(t, filepath.Join(dir, environmentFile))
	for _, want := range []string{"ORPHEUM_TUNNEL_IPV4=10.77.0.3\n", "ORPHEUM_TUNNEL_IPV6=fd77::3\n"} {
		if !strings.Contains(env, want) {
			t.Errorf(".env is %q; want it to hold %q for the tunnel service", env, want)
		}
	}
}

func TestWrite_ReleaseRefusesAnUnmigratableDatabaseAndStagingStartsOver(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		kind                Kind
		wantWhen, wantLevel string
		wantProject         string
		// A release node requires confinement; staging, in a sandbox that may lack it, reports (D273
		// §5 as revised by the owner, 2026-10-20, the Plex-chain review's N2).
		wantConfinement string
	}{
		{Release, "refuse", "info", "orpheum", "require"},
		{Staging, "start_over", "debug", "orpheum-staging", "report"},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			t.Parallel()
			dir := begun(t, tc.kind)
			if err := Write(dir, filledValues(t, tc.kind), runtime); err != nil {
				t.Fatal(err)
			}
			core := decodeTOML(t, filepath.Join(dir, coreConfig))
			if core["database"]["when_unmigratable"] != tc.wantWhen || core["log"]["level"] != tc.wantLevel {
				t.Errorf("%s's core config: when_unmigratable %v, log level %v; want %s and %s", tc.kind,
					core["database"]["when_unmigratable"], core["log"]["level"], tc.wantWhen, tc.wantLevel)
			}
			node := decodeTOML(t, filepath.Join(dir, nodeConfig))
			if node["node"]["confinement_policy"] != tc.wantConfinement {
				t.Errorf("%s's node config: confinement_policy %v; want %s", tc.kind,
					node["node"]["confinement_policy"], tc.wantConfinement)
			}
			if env := readFile(t, filepath.Join(dir, environmentFile)); !strings.Contains(env,
				"COMPOSE_PROJECT_NAME="+tc.wantProject+"\n") {
				t.Errorf("%s's .env is %q; want the project %s", tc.kind, env, tc.wantProject)
			}
		})
	}
}

func TestWrite_ComposeReadsEveryVariableTheEnvironmentFileSets(t *testing.T) {
	t.Parallel()
	dir := begun(t, Release)
	if err := Write(dir, filledValues(t, Release), runtime); err != nil {
		t.Fatal(err)
	}
	env := readFile(t, filepath.Join(dir, environmentFile))
	for _, want := range []string{
		"ORPHEUM_IMAGE=orpheum:v0.1.0", "ORPHEUM_UID=1000", "ORPHEUM_GID=1001",
		"ORPHEUM_HTTP_PUBLISH=0.0.0.0", "ORPHEUM_HTTP_PORT=8440", "ORPHEUM_LAN_HTTPS_PORT=8443",
		"ORPHEUM_TUNNEL_MTU=1420",
		"ORPHEUM_TUNNEL_IPV4=10.77.0.2", "ORPHEUM_TUNNEL_IPV6=fd77::2",
	} {
		if !strings.Contains(env, want+"\n") {
			t.Errorf(".env lacks %s", want)
		}
	}
	compose := readFile(t, filepath.Join(dir, composeFile))
	for line := range strings.SplitSeq(env, "\n") {
		name, _, isSetting := strings.Cut(line, "=")
		if isSetting && strings.HasPrefix(name, "ORPHEUM_") && !strings.Contains(compose, "${"+name) {
			t.Errorf(".env sets %s, which the compose file never reads", name)
		}
	}
	if strings.Contains(compose, "ORPHEUM_DIR") {
		t.Error("the compose file names ORPHEUM_DIR; want its paths relative to its own directory")
	}
}

func TestWrite_TheOverrideMountsEachRootReadOnlyAndTheGPUWhenThereIsOne(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		renderGID int
		wantGPU   bool
	}{
		{"with a GPU", 105, true},
		{"without one", -1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			text, err := mediaOverride([]string{"/mnt/a b", "/srv/c"}, tc.renderGID, NVIDIANone)
			if err != nil {
				t.Fatal(err)
			}
			var override struct {
				Services map[string]struct {
					Volumes  []string `json:"volumes"`
					Devices  []string `json:"devices"`
					GroupAdd []string `json:"group_add"` //nolint:tagliatelle // compose's own key
				} `json:"services"`
			}
			if err = json.Unmarshal(text, &override); err != nil {
				t.Fatal(err)
			}
			node := override.Services["orpheum-node"]
			if !slices.Equal(node.Volumes, []string{"/mnt/a b:/mnt/a b:ro", "/srv/c:/srv/c:ro"}) {
				t.Errorf("the node's volumes are %v; want each root read-only at its own path", node.Volumes)
			}
			hasGPU := slices.Equal(node.Devices, []string{"/dev/dri:/dev/dri"}) &&
				slices.Equal(node.GroupAdd, []string{"105"})
			if hasGPU != tc.wantGPU || !tc.wantGPU && (node.Devices != nil || node.GroupAdd != nil) {
				t.Errorf("the node's devices are %v and groups %v; want the GPU: %t", node.Devices, node.GroupAdd,
					tc.wantGPU)
			}
		})
	}
}

func TestQuoteTOML_QuotesOrRefuses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		value   any
		want    string
		isError bool
	}{
		{"a plain string", "nas", `"nas"`, false},
		{"quotes and a backslash", `a "b" \c`, `"a \"b\" \\c"`, false},
		{"beyond ASCII", "Médiathèque", `"Médiathèque"`, false},
		{"a list", []string{"/a", "/b"}, `["/a", "/b"]`, false},
		{"a newline", "a\nb", "", true},
		{"bytes that are not UTF-8", "a\xffb", "", true},
		{"a number", 1, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := quoteTOML(tc.value)
			if (err != nil) != tc.isError || got != tc.want {
				t.Errorf("quoteTOML(%q) = %q, %v; want %q, an error: %t", tc.value, got, err, tc.want, tc.isError)
			}
			if err == nil {
				var decoded map[string]any
				if _, err = toml.Decode("v = "+got, &decoded); err != nil || !equalValue(decoded["v"], tc.value) {
					t.Errorf("TOML reads %s as %v (%v); want %v", got, decoded["v"], err, tc.value)
				}
			}
		})
	}
}

// equalValue reports whether a value TOML decoded is the string or list of strings given.
func equalValue(decoded, given any) bool {
	if list, isList := given.([]string); isList {
		items, isItems := decoded.([]any)
		if !isItems || len(items) != len(list) {
			return false
		}
		for index, item := range items {
			if item != list[index] {
				return false
			}
		}
		return true
	}
	return decoded == given
}

func TestRender_ANameWithNoValueIsAnError(t *testing.T) {
	t.Parallel()
	if _, err := render("core.toml", map[string]any{"IsStaging": false}); err == nil {
		t.Error("rendering the core config without its values succeeded; want an error")
	}
}

func TestNewKeyPair_IsAClampedX25519PairAsWgMakesOne(t *testing.T) {
	t.Parallel()
	private, public, err := newKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	secret, err := base64.StdEncoding.DecodeString(private)
	if err != nil || len(secret) != keyLength {
		t.Fatalf("the private key %q is not 32 bytes of base64", private)
	}
	if secret[0]&7 != 0 || secret[31]&128 != 0 || secret[31]&64 == 0 {
		t.Errorf("the private key is not clamped: first byte %08b, last %08b", secret[0], secret[31])
	}
	key, err := ecdh.X25519().NewPrivateKey(secret)
	if err != nil {
		t.Fatal(err)
	}
	if want := base64.StdEncoding.EncodeToString(key.PublicKey().Bytes()); public != want {
		t.Errorf("the public key is %s, want %s, the private key's", public, want)
	}
	if !isWireGuardKey(public) {
		t.Errorf("the public key %s is not a WireGuard key", public)
	}
}

// The machine's router at home, which the core's container cannot see, goes to the core for its port
// mapping to ask, not Docker's bridge (item 15 of the owner's comments of 2026-11-02); one that is not
// a router at home is a problem the values name.
func TestWrite_TheCoreAsksTheRouterTheValuesName(t *testing.T) {
	t.Parallel()
	dir := begun(t, Staging)
	values := filledValues(t, Staging)
	values.LAN.Router = "192.168.1.1"
	if err := Write(dir, values, runtime); err != nil {
		t.Fatal(err)
	}
	if got := decodeTOML(t, filepath.Join(dir, coreConfig))["discovery"]["router"]; got != "192.168.1.1" {
		t.Errorf("the core's discovery.router is %v; want the values file's 192.168.1.1", got)
	}
	values.LAN.Router = "8.8.8.8"
	if err := Write(dir, values, runtime); err == nil || !strings.Contains(err.Error(), "lan.router") {
		t.Errorf("writing with a public router = %v; want it refused, naming lan.router", err)
	}
}

// The core's config says how the deployment was installed: the install script's Write says script,
// and a compose file's start names compose to WriteConfigs, so the System page shows only that way's
// steps to update (D450 §2).
func TestWriteConfigs_TheCoreSaysHowTheDeploymentWasInstalled(t *testing.T) {
	t.Parallel()
	dir := begun(t, Staging)
	values := filledValues(t, Staging)
	if err := Write(dir, values, runtime); err != nil {
		t.Fatal(err)
	}
	if got := decodeTOML(t, filepath.Join(dir, coreConfig))["server"]["install_route"]; got != "script" {
		t.Errorf("written by the install script's Write, the core's server.install_route is %v; want script", got)
	}
	if err := WriteConfigs(dir, values, ByComposeFile); err != nil {
		t.Fatal(err)
	}
	if got := decodeTOML(t, filepath.Join(dir, coreConfig))["server"]["install_route"]; got != "compose" {
		t.Errorf("written for a compose file's start, the core's server.install_route is %v; want compose", got)
	}
}

// The machine's address at home, which the core's container cannot see, goes to the core with the
// port HTTPS is published on there, for its discovery to name for devices at home; none is sent when
// the values file names none, and an address that is not the house's is a problem the values name
// (the owner, 2026-10-14).
func TestWrite_TheCoreSendsItsDiscoveryTheMachinesAddressAtHome(t *testing.T) {
	t.Parallel()
	dir := begun(t, Staging)
	values := filledValues(t, Staging)
	values.LAN.Address = "192.168.1.199"
	if problems := values.Problems(); len(problems) != 0 {
		t.Fatalf("the values have problems: %v", problems)
	}
	if err := Write(dir, values, runtime); err != nil {
		t.Fatal(err)
	}
	discovery := decodeTOML(t, filepath.Join(dir, coreConfig))["discovery"]
	if got := fmt.Sprint(discovery["lan_addresses"]); got != "[192.168.1.199:8443]" {
		t.Errorf("the core's discovery.lan_addresses are %s; want the machine's address on the HTTPS port", got)
	}
	if compose := readFile(t, filepath.Join(dir, composeFile)); !strings.Contains(compose,
		"${ORPHEUM_LAN_HTTPS_PORT:-8443}:443") {
		t.Error("the compose file does not publish HTTPS at home")
	}
	values.LAN.Address = ""
	if got := values.LANAddresses(); len(got) != 0 {
		t.Errorf("with no address at home the LAN addresses are %v; want none", got)
	}
	if problems := values.Problems(); !strings.Contains(strings.Join(problems, " "), "lan.address") {
		t.Errorf("no address at home is no problem: %v; want lan.address named (the Plex-chain review's L1)",
			problems)
	}
	for _, wrong := range []string{"11.22.44.146", "a-name"} {
		values.LAN.Address = wrong
		if problems := values.Problems(); !strings.Contains(strings.Join(problems, " "), "lan.address") {
			t.Errorf("lan.address %q is no problem: %v", wrong, problems)
		}
	}
}

// With an address at home the core takes WebRTC on its published UDP port, the same inside, which it
// offers devices in the house; with none it takes none (D300 §1).
func TestWrite_TheCoreTakesWebRTCAtHomeOnItsPublishedPort(t *testing.T) {
	t.Parallel()
	dir := begun(t, Staging)
	values := filledValues(t, Staging)
	values.LAN.Address = "192.168.1.199"
	values.LAN.WebRTCPort = 8451
	if err := Write(dir, values, runtime); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(decodeTOML(t, filepath.Join(dir, coreConfig))["webrtc"]["udp_port"]); got != "8451" {
		t.Errorf("the core's webrtc.udp_port is %s; want the published port, 8451", got)
	}
	if compose := readFile(t, filepath.Join(dir, composeFile)); !strings.Contains(compose,
		"${ORPHEUM_LAN_WEBRTC_PORT:-8443}:${ORPHEUM_LAN_WEBRTC_PORT:-8443}/udp") {
		t.Error("the compose file does not publish WebRTC at home on the same port inside")
	}
	if environment := readFile(t, filepath.Join(dir, environmentFile)); !strings.Contains(environment,
		"ORPHEUM_LAN_WEBRTC_PORT=8451\n") {
		t.Errorf("the environment does not name the WebRTC port:\n%s", environment)
	}
	values.LAN.Address = ""
	if got := values.WebRTCUDPPort(); got != 0 {
		t.Errorf("with no address at home the core's WebRTC port is %d; want 0", got)
	}
	if err := Write(dir, values, runtime); err == nil || !strings.Contains(err.Error(), "lan.address") {
		t.Errorf("writing with no address at home = %v; want it refused, naming lan.address", err)
	}
	values.LAN.Address = "192.168.1.199"
	values.LAN.WebRTCPort = 0
	if problems := values.Problems(); !strings.Contains(strings.Join(problems, " "), "lan.webrtc_port") {
		t.Errorf("lan.webrtc_port 0 is no problem: %v", problems)
	}
}

// The core sends its discovery the VPS's public addresses on its HTTPS port, the IPv6 one too when
// the deployment has its own, as configured candidates (the protocol's §2.4).
func TestWrite_TheCoreSendsItsDiscoveryTheVPSsAddresses(t *testing.T) {
	t.Parallel()
	dir := begun(t, Staging)
	values := filledValues(t, Staging)
	values.VPS.PublicIPv6 = "2a0f:2000:1c16:2684::3"
	if problems := values.Problems(); len(problems) != 0 {
		t.Fatalf("the values have problems: %v", problems)
	}
	if err := Write(dir, values, runtime); err != nil {
		t.Fatal(err)
	}
	discovery := decodeTOML(t, filepath.Join(dir, coreConfig))["discovery"]
	want := []any{values.VPS.PublicIPv4 + ":443", "[2a0f:2000:1c16:2684::3]:443"}
	if got := discovery["configured_addresses"]; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("the core's discovery.configured_addresses are %v; want %v", got, want)
	}
	// In its container the core's own interfaces reach nothing at home: it sends none of them.
	if sends, isBool := discovery["send_interface_addresses"].(bool); !isBool || sends {
		t.Errorf("the core's discovery.send_interface_addresses is %v; want false in a container", sends)
	}
	values.VPS.PublicIPv6 = "10.0.0.1"
	if problems := values.Problems(); !strings.Contains(strings.Join(problems, "\n"), "vps.public_ipv6") {
		t.Errorf("an IPv4 address as vps.public_ipv6 gave no problem: %v", problems)
	}
}

// directValues is a values file with no tunnel and no name of its own: a core reached through its
// discovery (D314 §3).
func directValues(t *testing.T) Values {
	t.Helper()
	values, err := ParseValues(`deployment = "release"
[media]
roots = ["/srv/media"]
[lan]
address = "192.168.1.20"
`, "nas")
	if err != nil {
		t.Fatal(err)
	}
	return values
}

func TestWrite_ACoreWithNoTunnelPublishesItsOwnPortsAndServesNoRelay(t *testing.T) {
	t.Parallel()
	dir := begun(t, Release)
	values := directValues(t)
	if problems := values.Problems(); len(problems) > 0 {
		t.Fatalf("a core with no tunnel and no domain has problems %v; want none: no VPS and no contact "+
			"needed", problems)
	}
	if err := Write(dir, values, runtime); err != nil {
		t.Fatal(err)
	}
	compose := readFile(t, filepath.Join(dir, composeFile))
	if strings.Contains(compose, "orpheum-tunnel") || strings.Contains(compose, "wg0") {
		t.Error("the compose file of a core with no tunnel names the tunnel")
	}
	for _, want := range []string{"${ORPHEUM_LAN_HTTPS_PORT:-8443}:443", "${ORPHEUM_HTTP_PORT:-8440}:8440", "/udp"} {
		if !strings.Contains(compose, want) {
			t.Errorf("the compose file of a core with no tunnel lacks %q: the core publishes its own ports", want)
		}
	}
	core := decodeTOML(t, filepath.Join(dir, coreConfig))
	if address := core["turn"]["public_address"]; address != "" {
		t.Errorf("turn.public_address is %v; want empty, no relay with no tunnel (D314 §4)", address)
	}
	if configured, isList := core["discovery"]["configured_addresses"].([]any); !isList || len(configured) != 0 {
		t.Errorf("discovery.configured_addresses is %v; want none with no VPS forwarding to it", configured)
	}
	if _, err := os.Stat(filepath.Join(dir, tunnelConfig)); err == nil {
		t.Error("a core with no tunnel had the tunnel's config written")
	}
	env := readFile(t, filepath.Join(dir, environmentFile))
	if strings.Contains(env, "ORPHEUM_TUNNEL_") {
		t.Errorf(".env names the tunnel's variables for a core with none: %q", env)
	}
	expectComposeReadsEnvironment(t, env, compose)
}

// expectComposeReadsEnvironment fails t for each variable env sets that compose never reads.
func expectComposeReadsEnvironment(t *testing.T, env, compose string) {
	t.Helper()
	for line := range strings.SplitSeq(env, "\n") {
		name, _, isSetting := strings.Cut(line, "=")
		if isSetting && strings.HasPrefix(name, "ORPHEUM_") && !strings.Contains(compose, "${"+name) {
			t.Errorf(".env sets %s, which the compose file never reads", name)
		}
	}
}

func TestValues_ATunnelHalfDescribedIsAProblemNotNoTunnel(t *testing.T) {
	t.Parallel()
	values, err := ParseValues(`deployment = "release"
[vps]
public_ipv4 = "203.0.113.7"
[media]
roots = ["/srv/media"]
`, "nas")
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, problem := range values.Problems() {
		keys = append(keys, strings.Fields(problem)[0])
	}
	if !slices.Contains(keys, "vps.endpoint:") {
		t.Errorf("a [vps] with an address and no endpoint has problems %v; want vps.endpoint named", keys)
	}
}
