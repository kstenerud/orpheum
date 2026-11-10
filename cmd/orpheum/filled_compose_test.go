// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/deployment"
)

// A server started by each route's file, a Synology's with a household's own values under "Yours",
// hands back over the Update page that very file: the core fills a newer one from what it was started
// with, so nothing the household set is lost by pasting it (D462 §4, §10.5).
func TestFilledComposeFile_AServerStartedByEachFileHandsThatFileBack(t *testing.T) {
	t.Parallel()
	household := deployment.Yours{
		Media: []string{"/volume2/films", "/volume1/tv"}, User: "1030:100", TimeZone: "America/New_York",
		HTTPPort: 8450, ThisMachinePort: 8451,
	}
	for _, route := range deployment.ComposeRoutes() {
		t.Run(string(route), func(t *testing.T) {
			t.Parallel()
			checkFilledComposeFile(t, route, household)
		})
	}
}

// checkFilledComposeFile checks that a server started by route's file, filled with household, hands
// that very file back.
func checkFilledComposeFile(t *testing.T, route deployment.InstallRoute, household deployment.Yours) {
	t.Helper()
	given, err := deployment.FilledComposeFile(route, household)
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(t.TempDir(), "compose.yaml")
	if err = os.WriteFile(name, given, 0o600); err != nil {
		t.Fatal(err)
	}
	core := releasedServices(composeResolved(t, name, composeEnvironments[1]))["orpheum-core"]
	if core == nil || len(core.command) == 0 {
		t.Fatalf("the %s file has no orpheum-core with a command", route)
	}
	var stderr bytes.Buffer
	options, isParsed := parseStart(core.command[1:], &stderr)
	if !isParsed {
		t.Fatalf("start refuses %q: %s", core.command[1:], stderr.String())
	}
	dir := t.TempDir()
	if err = startIn(dir, filepath.Join(dir, "run"), "nas", options); err != nil {
		t.Fatal(err)
	}
	_, made := writtenConfig(t, filepath.Join(dir, "core"), kindCore)
	config, _, err := loadConfig(made.fullConfigPath(), kindCore)
	if err != nil {
		t.Fatal(err)
	}
	told, err := composeFileOf(config.Server, household.User, household.TimeZone)
	if err != nil {
		t.Fatal(err)
	}
	if told.Filled != string(given) {
		t.Errorf("a server started by the %s file hands back another:\n%s", route,
			lineDifferences(string(given), told.Filled))
	}
}

// lineDifferences are the lines of want and got that differ, each as the two say it.
func lineDifferences(want, got string) string {
	wanted, gotten := strings.Split(want, "\n"), strings.Split(got, "\n")
	var differences []string
	for index := range max(len(wanted), len(gotten)) {
		var one, other string
		if index < len(wanted) {
			one = wanted[index]
		}
		if index < len(gotten) {
			other = gotten[index]
		}
		if one != other {
			differences = append(differences, "want "+one+"\n got "+other)
		}
	}
	return strings.Join(differences, "\n")
}
