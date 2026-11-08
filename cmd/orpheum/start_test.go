// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/kstenerud/orpheum/internal/deployment"
	"github.com/kstenerud/orpheum/internal/testutil"
)

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
	options := startOptionsFrom(t, "--lan-address", "192.168.1.20", "--lan-https-port", "8444",
		"--lan-webrtc-port", "8444", "--router", "192.168.1.1")
	if err := startDeployment(dir, runDir, "openmediavault", options); err != nil {
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
	if values.Deployment != deployment.Release || values.LAN.Address != "192.168.1.20" ||
		values.LAN.HTTPSPort != 8444 || values.LAN.WebRTCPort != 8444 || values.LAN.Router != "192.168.1.1" ||
		!slices.Equal(values.Media.Roots, []string{startMedia}) {
		t.Errorf("the values file written = %+v; want a release with the flags' address, ports and router, "+
			"and the media at %s, where the compose file mounts them", values, startMedia)
	}
	coreConfig, _ := writtenConfig(t, filepath.Join(dir, "core"), kindCore)
	lan := coreConfig["discovery"]["lan_addresses"]
	if !slices.Equal(anyStrings(t, lan), []string{"192.168.1.20:8444"}) {
		t.Errorf("the core's lan_addresses = %v; want the flags' address and HTTPS port", lan)
	}
	if route := coreConfig["server"]["install_route"]; route != "compose" {
		t.Errorf("the core config says it was installed by %v; want a compose file's route, compose, so the "+
			"System page shows only its steps to update (D450 §2)", route)
	}
	nodeConfig, node := writtenConfig(t, filepath.Join(dir, "node"), kindNode)
	if nodeConfig["node"]["name"] != "openmediavault" || node.RunDir != filepath.Join(runDir, "node") ||
		!slices.Equal(anyStrings(t, nodeConfig["node"]["root_paths"]), []string{startMedia}) {
		t.Errorf("the node's instance runs in %s with %v; want it in run/node, named by the machine, serving %s",
			node.RunDir, nodeConfig["node"], startMedia)
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
	if err := startDeployment(dir, runDir, "nas", startOptionsFrom(t, "--lan-address", "192.168.1.20")); err != nil {
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
	err := startDeployment(dir, runDir, "nas", startOptionsFrom(t, "--lan-address", "192.168.1.30",
		"--lan-https-port", "9444", "--media", "/srv/films", "--this-machine-port", "8451"))
	if err != nil {
		t.Fatal(err)
	}
	coreConfig, _ := writtenConfig(t, filepath.Join(dir, "core"), kindCore)
	lan := coreConfig["discovery"]["lan_addresses"]
	if !slices.Equal(anyStrings(t, lan), []string{"192.168.1.30:9444"}) {
		t.Errorf("the core's lan_addresses after a start with new flags = %v; want the new address and port", lan)
	}
	if listen := coreConfig["server"]["this_machine_listen"]; listen != "0.0.0.0:8451" {
		t.Errorf("the core's this_machine_listen = %v; want the new port, the same inside", listen)
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
		values.LAN.Address != "192.168.1.30" || !strings.Contains(string(text), "compose file") {
		t.Errorf("the values file after the second start = %v, %q; want the new address, and a header that sends "+
			"a change to the compose file", parseErr, text)
	}
	if held, readErr := os.ReadFile(kept); readErr != nil || string(held) != "the household's library" {
		t.Errorf("the core's database after a later start = %q, %v; want it kept", held, readErr)
	}
}

// A compose file passes each optional media folder, set or not; one not set comes empty and is no
// folder, and a folder is named as the path the node mounts it at, cleaned (D449 §4).
func TestStart_AnEmptyMediaFolderIsNoneAndEachIsCleaned(t *testing.T) {
	t.Parallel()
	options := startOptionsFrom(t, "--lan-address", "192.168.1.20", "--media", "/srv/films/",
		"--media", "", "--media", " /srv/shows ", "--media", "")
	if !slices.Equal(options.media, []string{"/srv/films", "/srv/shows"}) {
		t.Errorf("the media from the compose file's folders = %q; want the two set, cleaned, and no empty one",
			options.media)
	}
	if none := startOptionsFrom(t, "--lan-address", "192.168.1.20", "--media", ""); !slices.Equal(
		none.media, []string{startMedia}) {
		t.Errorf("the media with every folder empty = %q; want %s", none.media, startMedia)
	}
}

func TestFirstStart_RefusesToWriteWithoutTheAddressAtHome(t *testing.T) {
	t.Parallel()
	dir, runDir := t.TempDir(), filepath.Join(testutil.SocketDirectory(t), "run")
	err := startDeployment(dir, runDir, "nas", startOptionsFrom(t))
	if !errors.Is(err, errNoLANAddress) || !strings.Contains(err.Error(), "ORPHEUM_LAN_ADDRESS") {
		t.Errorf("a first start without --lan-address = %v; want it refused, naming the flag and the "+
			"compose file's variable", err)
	}
	if entries, readErr := os.ReadDir(dir); readErr != nil || len(entries) != 0 {
		t.Errorf("the directory after a refused first start holds %v (%v); want nothing written", entries, readErr)
	}
}

func TestFirstStart_ACutShortStartIsWrittenAgainKeepingItsInstances(t *testing.T) {
	t.Parallel()
	dir, runDir := t.TempDir(), filepath.Join(testutil.SocketDirectory(t), "run")
	// A start cut short after making the instances, before the values file that marks it done.
	if err := makeMissingInstances(dir, runDir); err != nil {
		t.Fatal(err)
	}
	if err := startDeployment(dir, runDir, "nas", startOptionsFrom(t, "--lan-address", "192.168.1.30")); err != nil {
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
