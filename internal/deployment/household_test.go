// SPDX-License-Identifier: AGPL-3.0-or-later

package deployment

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// synologyCompose is the compose file a Synology's Container Manager is given, which needs no .env
// file: making one is the hardest step on a Synology (the owner, 2026-11-09).
const synologyCompose = "../../deploy/synology/compose.yaml"

// Every route's published file is the one template's rendering, so no route's file can fall out of
// step with another's (D462 §10.2); and writing them again over the published ones changes nothing.
func TestHouseholdComposeFiles_AreTheTemplatesRenderingAtEachRoutesPath(t *testing.T) {
	t.Parallel()
	for _, route := range ComposeRoutes() {
		want, err := HouseholdComposeFile(route)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join("..", "..", route.PublishedPath()))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Errorf("%s is not templates/household.yaml rendered for %s; run make generate", route.PublishedPath(),
				route)
		}
	}
	root := t.TempDir()
	if err := WriteHouseholdComposeFiles(root); err != nil {
		t.Fatal(err)
	}
	for _, route := range ComposeRoutes() {
		if _, err := os.Stat(filepath.Join(root, route.PublishedPath())); err != nil {
			t.Errorf("compose-files wrote no %s: %v", route.PublishedPath(), err)
		}
	}
	if _, err := FilledComposeFile(ByInstallScript, DefaultYours()); err == nil {
		t.Error("the install script's compose file was rendered; want it refused, as it has none of its own")
	}
}

// Each file tells start which route it is, so the Update page shows its steps alone, and the image
// tag it runs (D462 §9, §10.7); an unknown route is docker's, so a newer file still starts on an older
// release (D462 §10.10).
func TestHouseholdComposeFiles_EachNamesItsOwnRouteToStart(t *testing.T) {
	t.Parallel()
	for _, route := range ComposeRoutes() {
		text := householdCompose(t, "../../"+route.PublishedPath())
		for _, want := range []string{
			"      - --compose-file\n      - " + string(route) + "\n",
			"      - --image-tag\n      - ${ORPHEUM_TAG:-latest}\n",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("%s does not say %q", route.PublishedPath(), want)
			}
		}
		if ComposeRoute(string(route)) != route || !route.IsCompose() {
			t.Errorf("route %s is not taken as itself", route)
		}
	}
	if ComposeRoute("truenas") != ByDocker || ComposeRoute("") != ByDocker || ByInstallScript.IsCompose() {
		t.Error("an unknown route, or none, is not taken as docker's, or the install script's is a compose file's")
	}
}

// yoursLine is a line under the Synology file's "Yours": its key, anchor and value.
var yoursLine = regexp.MustCompile(`(?m)^ {2}([a-z0-9-]+): &[a-z0-9]+ "?([^"\n]*)"?$`)

// yoursOf is the values under a Synology file's "Yours", as a household reads them back.
func yoursOf(t *testing.T, text string) Yours {
	t.Helper()
	_, block, isThere := strings.Cut(text, "\nx-yours:\n")
	if !isThere {
		t.Fatal("no x-yours block")
	}
	block, _, _ = strings.Cut(block, "\n\n")
	values := map[string]string{}
	for _, match := range yoursLine.FindAllStringSubmatch(block, -1) {
		values[match[1]] = match[2]
	}
	port := func(key string) int {
		number, err := strconv.Atoi(values[key])
		if err != nil {
			t.Fatalf("%s %q is not a port", key, values[key])
		}
		return number
	}
	return Yours{
		Media:    []string{values["media"], values["media-2"], values["media-3"], values["media-4"]},
		User:     values["user"],
		TimeZone: values["time-zone"],
		HTTPPort: port("http-port"), ThisMachinePort: port("this-machine-port"),
	}
}

// The file the Update page hands a Synology household carries its values under "Yours", each read back
// as it was, a folder not set as /unset/<n>, so pasting it over the old one keeps them (D462 §4,
// §10.5); and the published file is that with the defaults.
func TestFilledComposeFile_KeepsASynologysValuesUnderYours(t *testing.T) {
	t.Parallel()
	yours := Yours{
		Media: []string{"/volume2/films", "/volume1/shows"}, User: "1027:101", TimeZone: "America/Toronto",
		HTTPPort: 8540, ThisMachinePort: 8541,
	}
	filled, err := FilledComposeFile(BySynology, yours)
	if err != nil {
		t.Fatal(err)
	}
	got := yoursOf(t, string(filled))
	want := yours
	want.Media = []string{"/volume2/films", "/volume1/shows", Unset + "3", Unset + "4"}
	if !slices.Equal(got.Media, want.Media) || got.User != want.User || got.TimeZone != want.TimeZone ||
		got.HTTPPort != want.HTTPPort || got.ThisMachinePort != want.ThisMachinePort {
		t.Errorf("the filled file's Yours read back = %+v; want %+v", got, want)
	}
	again, err := FilledComposeFile(BySynology, got)
	if err != nil || string(again) != string(filled) {
		t.Errorf("the file filled from its own Yours differs (%v); want the same file, so a second update keeps them",
			err)
	}
	published := householdCompose(t, synologyCompose)
	if defaults := yoursOf(t, published); defaults.Media[0] != "/volume1/video" || defaults.User != "1026:100" ||
		defaults.TimeZone != "Europe/Berlin" {
		t.Errorf("the published Synology file's Yours = %+v; want the defaults", defaults)
	}
	if _, err = FilledComposeFile(BySynology, Yours{Media: make([]string, 5)}); err == nil {
		t.Error("a file with five media folders was filled; want it refused, as the file names four")
	}
}

// The Synology file reads no variable a .env file would set, and its header says to keep a copy, as a
// change outside "Yours" is lost when the Update page's file is pasted (D462 §10.5).
func TestSynologyCompose_NeedsNoEnvFileAndSaysToKeepACopy(t *testing.T) {
	t.Parallel()
	text := householdCompose(t, synologyCompose)
	services := composeLines(t, synologyCompose)
	for _, needed := range []string{"PATH_TO_APPDATA", "ORPHEUM_MEDIA", "APPUSER_", "TIME_ZONE_VALUE", "_PORT"} {
		if strings.Contains(services, needed) {
			t.Errorf("%s reads %s, which needs a .env file", synologyCompose, needed)
		}
	}
	if !strings.Contains(text, "# Keep a copy of this file.") || !strings.Contains(text, "is lost when you paste it") {
		t.Errorf("%s does not say to keep a copy, and that a change outside Yours is lost", synologyCompose)
	}
}
