// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/kstenerud/orpheum/internal/deployment"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// everyPortBinds is a machine that lets the core take every port.
func everyPortBinds(Ports) error { return nil }

// startIn starts the deployment in dir with options on a machine that lets it take every port.
func startIn(dir, runDir, hostName string, options startOptions) error {
	_, err := startDeployment(dir, runDir, hostName, options, everyPortBinds)
	return err
}

// startOptionsFrom are the options start's flags give, as the compose file passes them.
func startOptionsFrom(t *testing.T, arguments ...string) startOptions {
	t.Helper()
	var stderr bytes.Buffer
	options, isParsed := parseStart(arguments, &stderr)
	if !isParsed {
		t.Fatalf("start %v refused: %s", arguments, stderr.String())
	}
	return options
}

func TestFirstStart_WritesTheDeploymentFromItsFlagsWhenThereIsNone(t *testing.T) {
	t.Parallel()
	dir, runDir := t.TempDir(), filepath.Join(testutil.SocketDirectory(t), "run")
	options := startOptionsFrom(t, "--compose-file", "omv", "--image-tag", "v0.4.0")
	if err := startIn(dir, runDir, "openmediavault", options); err != nil {
		t.Fatalf("a first start in an empty directory = %v; want the deployment written", err)
	}
	text, err := os.ReadFile(filepath.Join(dir, deployment.ValuesFile))
	if err != nil {
		t.Fatal(err)
	}
	values, err := deployment.ParseValues(string(text), "elsewhere")
	if err != nil {
		t.Fatal(err)
	}
	if values.Deployment != deployment.Release || values.LAN.Address != "" ||
		values.LAN.HTTPSPort != startHTTPSPort || values.LAN.WebRTCPort != startHTTPSPort || values.LAN.Router != "" ||
		!slices.Equal(values.Media.Roots, []string{startMedia}) {
		t.Errorf("the values file written = %+v; want a release with no address at home, which is learned, start's "+
			"HTTPS and WebRTC port and no router until the System page sets them, and the media at %s", values,
			startMedia)
	}
	checkFirstStartCore(t, dir)
	nodeConfig, node := writtenConfig(t, filepath.Join(dir, "node"), kindNode)
	if nodeConfig["node"]["name"] != "openmediavault" || node.RunDir != filepath.Join(runDir, "node") ||
		!slices.Equal(anyStrings(t, nodeConfig["node"]["root_paths"]), []string{startMedia}) {
		t.Errorf("the node's instance runs in %s with %v; want it in run/node, named by the machine, serving %s",
			node.RunDir, nodeConfig["node"], startMedia)
	}
}

// checkFirstStartCore checks the core's config and network settings after a first start from the omv
// file at image tag v0.4.0.
func checkFirstStartCore(t *testing.T, dir string) {
	t.Helper()
	coreConfig, _ := writtenConfig(t, filepath.Join(dir, "core"), kindCore)
	if route := coreConfig["server"]["install_route"]; route != "omv" {
		t.Errorf("the core config says it was installed by %v; want the compose file's route, omv, so the "+
			"Update page shows only its steps (D462 §9)", route)
	}
	if tag := coreConfig["server"]["image_tag"]; tag != "v0.4.0" {
		t.Errorf("the core config says it runs image tag %v; want the file's, v0.4.0 (D462 §10.7)", tag)
	}
	if media := anyStrings(t, coreConfig["server"]["compose_media"]); !slices.Equal(media, []string{startMedia}) {
		t.Errorf("the core config says the compose file names the media %v; want %s, for the Update page to "+
			"fill into a newer file (D462 §4)", media, startMedia)
	}
	kept, err := deployment.ReadNetwork(deployment.NetworkPath(dir))
	if err != nil || kept != (deployment.Network{HTTPSPort: startHTTPSPort, WebRTCPort: startHTTPSPort}) {
		t.Errorf("the network settings kept after a first start = %+v, %v; want start's ports and no router", kept, err)
	}
}

// writtenConfig is the full config of the instance in dir, its tables by name, and the instance.
func writtenConfig(t *testing.T, dir string, kind instanceKind) (map[string]map[string]any, Instance) {
	t.Helper()
	made, err := loadInstance(dir, kind)
	if err != nil {
		t.Fatal(err)
	}
	config := map[string]map[string]any{}
	if _, err = toml.DecodeFile(made.fullConfigPath(), &config); err != nil {
		t.Fatal(err)
	}
	return config, made
}

// Every start writes the values file and the configs again from its flags (D449 §1): a port or a
// folder changed in the compose file takes at the next start, and a hand edit of a config is lost,
// while the instances, the database among them, are kept.
func TestStart_WritesTheConfigsAgainFromTheFlagsAtEveryStart(t *testing.T) {
	t.Parallel()
	dir, runDir := t.TempDir(), filepath.Join(testutil.SocketDirectory(t), "run")
	if err := startIn(dir, runDir, "nas", startOptionsFrom(t)); err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(dir, "core", "persistent", "persistent.db")
	if err := os.WriteFile(kept, []byte("the household's library"), 0o600); err != nil {
		t.Fatal(err)
	}
	coreConfigPath := filepath.Join(dir, "core", "persistent", "config.toml")
	if err := os.WriteFile(coreConfigPath, []byte("# edited by hand\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := startIn(dir, runDir, "nas", startOptionsFrom(t, "--http-port", "8450", "--media", "/srv/films",
		"--this-machine-port", "8451"))
	if err != nil {
		t.Fatal(err)
	}
	coreConfig, _ := writtenConfig(t, filepath.Join(dir, "core"), kindCore)
	if listen := coreConfig["server"]["listen"]; listen != ":8450" {
		t.Errorf("the core's server.listen = %v; want the new port", listen)
	}
	if listen := coreConfig["server"]["this_machine_listen"]; listen != "127.0.0.1:8451" {
		t.Errorf("the core's this_machine_listen = %v; want the new port on this machine's loopback", listen)
	}
	nodeConfig, _ := writtenConfig(t, filepath.Join(dir, "node"), kindNode)
	if roots := anyStrings(t, nodeConfig["node"]["root_paths"]); !slices.Equal(roots, []string{"/srv/films"}) {
		t.Errorf("the node's root_paths after a start with a new folder = %v; want it", roots)
	}
	text, err := os.ReadFile(filepath.Join(dir, deployment.ValuesFile))
	if err != nil {
		t.Fatal(err)
	}
	if values, parseErr := deployment.ParseValues(string(text), "nas"); parseErr != nil ||
		values.HTTP.Port != 8450 || !strings.Contains(string(text), "compose file") {
		t.Errorf("the values file after the second start = %v, %q; want the new port, and a header that sends "+
			"a change to the compose file", parseErr, text)
	}
	if held, readErr := os.ReadFile(kept); readErr != nil || string(held) != "the household's library" {
		t.Errorf("the core's database after a later start = %q, %v; want it kept", held, readErr)
	}
}

// A compose file passes each optional media folder, set or not; one not set comes empty, or under
// /unset/ from a Synology's file, and is no folder, and a folder is named as the path the node mounts
// it at, cleaned (D449 §4, D462 §10.5).
func TestStart_AnEmptyOrUnsetMediaFolderIsNoneAndEachIsCleaned(t *testing.T) {
	t.Parallel()
	options := startOptionsFrom(t, "--media", "/srv/films/", "--media", "", "--media", " /srv/shows ",
		"--media", deployment.Unset+"4")
	if !slices.Equal(options.media, []string{"/srv/films", "/srv/shows"}) {
		t.Errorf("the media from the compose file's folders = %q; want the two set, cleaned, and none empty or unset",
			options.media)
	}
	if none := startOptionsFrom(t, "--media", "", "--media", deployment.Unset+"2"); !slices.Equal(
		none.media, []string{startMedia}) {
		t.Errorf("the media with every folder empty or unset = %q; want %s", none.media, startMedia)
	}
}

// A flag this release does not take is logged and skipped with its value, so a newer compose file, or
// v0.3.3's with the flags D462 §2 moved to the System page, still starts (D462 §10.10); a route it does
// not know is taken as docker's, said.
func TestStart_SkipsAFlagItDoesNotTakeAndTakesAnUnknownRouteAsDockers(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	options, isParsed := parseStart([]string{
		"--lan-address", "", "--http-port", "8450", "--router", "192.168.1.1",
		"--lan-https-port=8444", "--webrtc-later", "--compose-file", "truenas", "--media", "/srv/films",
	}, &stderr)
	if !isParsed || options.httpPort != 8450 || !slices.Equal(options.media, []string{"/srv/films"}) ||
		options.composeFile != deployment.ByDocker {
		t.Fatalf("start with flags it does not take = %+v, %v, saying %q; want the ones it takes, the route docker",
			options, isParsed, stderr.String())
	}
	for _, said := range []string{"--lan-address", "--router", "--lan-https-port=8444", "--webrtc-later", `"truenas"`} {
		if !strings.Contains(stderr.String(), said) {
			t.Errorf("start says %q; want it to name %s", stderr.String(), said)
		}
	}
	if strings.Contains(stderr.String(), "192.168.1.1") {
		t.Errorf("start says %q; want a skipped flag's value skipped with it, not named as a flag", stderr.String())
	}
}

// The core's config names the version of the compose file that started it, 1 for a file that names
// none, as every file released before the flag did, so the System page can say when a newer one
// brings something; a version below 1 is no compose file's, and start refuses it (D461 §3).
func TestStart_TheCoreSaysTheComposeFilesVersion(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		arguments []string
		want      int64
	}{
		{nil, 1},
		{[]string{"--compose-version", "2"}, 2},
	} {
		dir, runDir := t.TempDir(), filepath.Join(testutil.SocketDirectory(t), "run")
		if err := startIn(dir, runDir, "nas", startOptionsFrom(t, test.arguments...)); err != nil {
			t.Fatal(err)
		}
		coreConfig, _ := writtenConfig(t, filepath.Join(dir, "core"), kindCore)
		if got := coreConfig["server"]["compose_version"]; got != test.want {
			t.Errorf("started with %q, the core's server.compose_version is %v; want %d",
				test.arguments, got, test.want)
		}
	}
	told, err := composeFileOf(ServerConfig{ComposeVersion: 1}, "1000:100", "UTC")
	if err != nil || told.Running != 1 || told.Newest != deployment.ComposeFileVersion ||
		!reflect.DeepEqual(told.Changes, deployment.ComposeFileChanges()) {
		t.Errorf("the System page is told of compose file %+v; want version 1, the newest %d and its changes",
			told, deployment.ComposeFileVersion)
	}
	var stderr bytes.Buffer
	if _, isParsed := parseStart([]string{"--compose-version", "0"}, &stderr); isParsed ||
		!strings.Contains(stderr.String(), "--compose-version 0") {
		t.Errorf("start --compose-version 0 parsed %v, saying %q; want it refused, naming the flag",
			isParsed, stderr.String())
	}
}

// The Update page hands a Synology household the newest file with its own values under "Yours": its
// media folders, the user and time zone the server runs with and the ports it was started on; any
// other compose route the published file, its values being its variables; and a server no compose
// file started none (D462 §4).
func TestComposeFileOf_ASynologyFileIsHandedOverWithTheHouseholdsValues(t *testing.T) {
	t.Parallel()
	synology := ServerConfig{
		InstallRoute: "synology", ComposeVersion: 1, Listen: ":8450", ThisMachineListen: "127.0.0.1:8451",
		ComposeMedia: []string{"/volume2/films", "/volume1/tv"},
	}
	told, err := composeFileOf(synology, "1030:100", "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		"media: &media /volume2/films", "media-2: &media2 /volume1/tv", "media-3: &media3 /unset/3",
		`user: &user "1030:100"`, "time-zone: &timezone America/New_York", `http-port: &httpport "8450"`,
		`this-machine-port: &thismachineport "8451"`,
	} {
		if !strings.Contains(told.Filled, line) {
			t.Errorf("the Synology file handed over lacks %q:\n%s", line, told.Filled)
		}
	}
	omv := synology
	omv.InstallRoute = "omv"
	published, err := deployment.HouseholdComposeFile(deployment.ByOMV)
	if err != nil {
		t.Fatal(err)
	}
	if told, err = composeFileOf(omv, "1030:100", "America/New_York"); err != nil || told.Filled != string(published) {
		t.Errorf("an OpenMediaVault server is handed another file than the published one (%v)", err)
	}
	if told, err = composeFileOf(ServerConfig{InstallRoute: "script"}, "0:0", "UTC"); err != nil || told.Filled != "" {
		t.Errorf("the install script's server is handed a compose file (%v): %q", err, told.Filled)
	}
}

func TestFirstStart_ACutShortStartIsWrittenAgainKeepingItsInstances(t *testing.T) {
	t.Parallel()
	dir, runDir := t.TempDir(), filepath.Join(testutil.SocketDirectory(t), "run")
	// A start cut short after making the instances, before the values file that marks it done.
	if err := makeMissingInstances(dir, runDir); err != nil {
		t.Fatal(err)
	}
	if err := startIn(dir, runDir, "nas", startOptionsFrom(t)); err != nil {
		t.Errorf("a start with the instances made but no values file = %v; want the configs written again "+
			"over the instances kept", err)
	}
	if _, err := os.Stat(filepath.Join(dir, deployment.ValuesFile)); err != nil {
		t.Errorf("the values file after a start over a cut-short one: %v; want it written", err)
	}
}

// anyStrings is a TOML array of strings as decoded into an any.
func anyStrings(t *testing.T, value any) []string {
	t.Helper()
	items, isList := value.([]any)
	if !isList {
		t.Fatalf("%v is not a list", value)
	}
	texts := make([]string, 0, len(items))
	for _, item := range items {
		text, isText := item.(string)
		if !isText {
			t.Fatalf("%v in %v is not a string", item, value)
		}
		texts = append(texts, text)
	}
	return texts
}

// The address at home is learned once the core runs, or set on the System page (D452 §5, D462 §2), so
// a start writes the deployment without it; the core takes every port on every address of this
// machine's network, the address at home never alone, as it moves (D454 §5, D462 §10.11), and this
// machine's on its loopback.
func TestStart_WritesTheDeploymentWithoutTheAddressAtHomeOnEveryAddress(t *testing.T) {
	t.Parallel()
	dir, runDir := t.TempDir(), filepath.Join(testutil.SocketDirectory(t), "run")
	if err := makeMissingInstances(dir, runDir); err != nil {
		t.Fatal(err)
	}
	if err := deployment.WriteNetwork(deployment.NetworkPath(dir), deployment.Network{
		HTTPSPort:  9444,
		WebRTCPort: 9445,
	}); err != nil {
		t.Fatal(err)
	}
	options := startOptionsFrom(t, "--http-port", "8450", "--this-machine-port", "8451")
	if err := startIn(dir, runDir, "nas", options); err != nil {
		t.Fatalf("a first start without an address at home = %v; want the deployment written", err)
	}
	coreConfig, _ := writtenConfig(t, filepath.Join(dir, "core"), kindCore)
	for _, want := range []struct {
		table, key string
		value      any
	}{
		{"server", "listen", ":8450"},
		{"server", "this_machine_listen", "127.0.0.1:8451"},
		{"https", "listen", ":9444"},
		{"webrtc", "udp_port", int64(9445)},
	} {
		if got := coreConfig[want.table][want.key]; got != want.value {
			t.Errorf("the core's %s.%s = %#v; want %#v", want.table, want.key, got, want.value)
		}
	}
	if seed := coreConfig["discovery"]["address_at_home_seed"]; seed != "" {
		t.Errorf("the core's address_at_home_seed = %v; want none until the address is learned", seed)
	}
}

// The network settings are the System page's: a start keeps the ones there, and the first takes them
// once from the values file an earlier release's start wrote, so an update keeps a household's ports
// and router (D462 §10.6).
func TestStart_TakesTheNetworkSettingsOnceFromTheValuesFileThenKeepsThem(t *testing.T) {
	t.Parallel()
	dir, runDir := t.TempDir(), filepath.Join(testutil.SocketDirectory(t), "run")
	if err := makeMissingInstances(dir, runDir); err != nil {
		t.Fatal(err)
	}
	earlier := deployment.StartingValues(deployment.Release)
	earlier.LAN.HTTPSPort, earlier.LAN.WebRTCPort, earlier.LAN.Router = 9444, 9445, "192.168.1.1"
	earlier.Media.Roots = []string{startMedia}
	if err := deployment.WriteStartValues(dir, earlier); err != nil {
		t.Fatal(err)
	}
	if err := startIn(dir, runDir, "nas", startOptionsFrom(t)); err != nil {
		t.Fatal(err)
	}
	want := deployment.Network{HTTPSPort: 9444, WebRTCPort: 9445, Router: "192.168.1.1"}
	if kept, err := deployment.ReadNetwork(deployment.NetworkPath(dir)); err != nil || kept != want {
		t.Errorf("the network settings after an update's first start = %+v, %v; want the values file's, %+v",
			kept, err, want)
	}
	set := deployment.Network{HTTPSPort: 9544, WebRTCPort: 9545}
	if err := deployment.WriteNetwork(deployment.NetworkPath(dir), set); err != nil {
		t.Fatal(err)
	}
	if err := startIn(dir, runDir, "nas", startOptionsFrom(t)); err != nil {
		t.Fatal(err)
	}
	coreConfig, _ := writtenConfig(t, filepath.Join(dir, "core"), kindCore)
	if listen, router := coreConfig["https"]["listen"], coreConfig["discovery"]["router"]; listen != ":9544" ||
		router != "" {
		t.Errorf("the core's HTTPS listen and router after the System page set them = %v, %q; want :9544 and none, "+
			"not the values file's", listen, router)
	}
}

// A kept port this machine will not let the core take falls back to start's default, said, so the
// server still starts and the System page shows the one it took is not the one kept (D462 §10.11).
func TestStart_APortThatWillNotBindFallsBackToItsDefault(t *testing.T) {
	t.Parallel()
	dir, runDir := t.TempDir(), filepath.Join(testutil.SocketDirectory(t), "run")
	kept := deployment.Network{HTTPSPort: 9444, WebRTCPort: 9445}
	if err := makeMissingInstances(dir, runDir); err != nil {
		t.Fatal(err)
	}
	if err := deployment.WriteNetwork(deployment.NetworkPath(dir), kept); err != nil {
		t.Fatal(err)
	}
	held := func(ports Ports) error {
		if ports.HTTPS == 9444 {
			return errWontBind
		}
		return nil
	}
	said, err := startDeployment(dir, runDir, "nas", startOptionsFrom(t), held)
	if err != nil {
		t.Fatal(err)
	}
	if len(said) != 1 || !strings.Contains(said[0], "8444") {
		t.Errorf("start says %q; want one line naming the HTTPS port it took instead, %d", said, startHTTPSPort)
	}
	coreConfig, _ := writtenConfig(t, filepath.Join(dir, "core"), kindCore)
	if listen, udp := coreConfig["https"]["listen"], coreConfig["webrtc"]["udp_port"]; listen != ":8444" ||
		udp != int64(9445) {
		t.Errorf("the core's HTTPS listen and WebRTC port = %v, %v; want the default for the one held, :8444, "+
			"and the kept 9445", listen, udp)
	}
	if after, readErr := deployment.ReadNetwork(deployment.NetworkPath(dir)); readErr != nil || after != kept {
		t.Errorf("the network settings after a fallback = %+v, %v; want the kept ones, %+v", after, readErr, kept)
	}
}

// portBinds tries each port on every address and holds none after; one another program holds will
// not bind.
func TestPortBinds_RefusesAPortHeldAndHoldsNoneItTried(t *testing.T) {
	t.Parallel()
	var config net.ListenConfig
	listener, err := config.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() }) //nolint:errcheck // closed again after the check
	address, isTCP := listener.Addr().(*net.TCPAddr)
	if !isTCP {
		t.Fatalf("the listener's address %v is not TCP", listener.Addr())
	}
	port := address.Port
	if err = portBinds(Ports{HTTPS: port}); !errors.Is(err, errWontBind) {
		t.Errorf("portBinds of a held port = %v; want it refused", err)
	}
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}
	if err = portBinds(Ports{HTTPS: port, WebRTC: port}); err != nil {
		t.Errorf("portBinds of a free port = %v; want it taken", err)
	}
	if err = portBinds(Ports{HTTPS: port}); err != nil {
		t.Errorf("portBinds of the port it tried = %v; want it free again", err)
	}
}

// A start makes nothing outside the deployment's directory, which the compose file mounts: with none
// there, it refuses.
func TestFirstStart_RefusesWithoutTheDeploymentsDirectory(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "not-mounted")
	err := startIn(missing, filepath.Join(testutil.SocketDirectory(t), "run"), "nas", startOptionsFrom(t))
	if !errors.Is(err, errNoDeployment) {
		t.Errorf("a start with no deployment directory = %v; want it refused", err)
	}
	if _, statErr := os.Stat(missing); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("the missing directory after a refused start: %v; want it still missing", statErr)
	}
}
