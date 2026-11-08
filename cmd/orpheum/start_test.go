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
	isWritten, err := firstStart(dir, runDir, "openmediavault", options)
	if err != nil || !isWritten {
		t.Fatalf("a first start in an empty directory = %v, %v; want the deployment written", isWritten, err)
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

func TestFirstStart_LeavesADeploymentThereAlreadyAsItIs(t *testing.T) {
	t.Parallel()
	dir, runDir := t.TempDir(), filepath.Join(testutil.SocketDirectory(t), "run")
	if _, err := firstStart(dir, runDir, "nas", startOptionsFrom(t, "--lan-address", "192.168.1.20")); err != nil {
		t.Fatal(err)
	}
	paths := []string{
		filepath.Join(dir, deployment.ValuesFile),
		filepath.Join(dir, "core", "persistent", "config.toml"),
		filepath.Join(dir, "node", "persistent", "config.toml"),
	}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("# the household's own\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	isWritten, err := firstStart(dir, runDir, "nas", startOptionsFrom(t, "--lan-address", "10.0.0.9"))
	if err != nil || isWritten {
		t.Errorf("a start over a deployment = %v, %v; want it left, nothing written", isWritten, err)
	}
	for _, path := range paths {
		if text, readErr := os.ReadFile(path); readErr != nil || string(text) != "# the household's own\n" {
			t.Errorf("%s after a later start = %q, %v; want it untouched: the files rule after the first start",
				path, text, readErr)
		}
	}
}

func TestFirstStart_RefusesToWriteWithoutTheAddressAtHome(t *testing.T) {
	t.Parallel()
	dir, runDir := t.TempDir(), filepath.Join(testutil.SocketDirectory(t), "run")
	isWritten, err := firstStart(dir, runDir, "nas", startOptionsFrom(t))
	if !errors.Is(err, errNoLANAddress) || isWritten ||
		!strings.Contains(err.Error(), "ORPHEUM_LAN_ADDRESS") {
		t.Errorf("a first start without --lan-address = %v, %v; want it refused, naming the flag and the "+
			"compose file's variable", isWritten, err)
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
	isWritten, err := firstStart(dir, runDir, "nas", startOptionsFrom(t, "--lan-address", "192.168.1.30"))
	if err != nil || !isWritten {
		t.Errorf("a start with the instances made but no values file = %v, %v; want the configs written again "+
			"over the instances kept", isWritten, err)
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
