// SPDX-License-Identifier: AGPL-3.0-or-later

package deployment

import (
	"encoding/json"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The catalogue entries (D449 §5): the compose file packaged for the catalogues of NAS and Docker
// panels, which the owner submits.
const (
	portainerTemplates = "../../deploy/catalogue/portainer/templates.json"
	casaOSCompose      = "../../deploy/catalogue/casaos/docker-compose.yml"
	// Portainer's compose file as its template names it, from the repository's root.
	portainerComposePath = "deploy/portainer/compose.yaml"
)

// composeDefault is a variable the compose file gives a default, and the default.
var composeDefault = regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*):-([^${}]*)\}`)

// portainerTemplate is what a Portainer app template (format version 3) says that the test keeps in step.
type portainerTemplate struct {
	Version   string `json:"version"`
	Templates []struct {
		Type       int `json:"type"`
		Repository struct {
			URL       string `json:"url"`
			Stackfile string `json:"stackfile"`
		} `json:"repository"`
		Env []struct {
			Name    string `json:"name"`
			Label   string `json:"label"`
			Default string `json:"default"`
		} `json:"env"`
	} `json:"templates"`
}

// Portainer's template installs the compose file itself, as a Compose stack from the repository, and
// asks for every variable it reads, with the compose file's defaults and none for one it needs.
func TestCatalogue_ThePortainerTemplateAsksForEveryVariableTheComposeFileReads(t *testing.T) {
	t.Parallel()
	text, err := os.ReadFile(portainerTemplates)
	if err != nil {
		t.Fatal(err)
	}
	var file portainerTemplate
	if err = json.Unmarshal(text, &file); err != nil {
		t.Fatal(err)
	}
	if file.Version != "3" || len(file.Templates) != 1 {
		t.Fatalf("%s: version %q with %d templates; want version 3 with Orpheum's one", portainerTemplates,
			file.Version, len(file.Templates))
	}
	template := file.Templates[0]
	if template.Type != 3 || template.Repository.URL != "https://github.com/kstenerud/orpheum" ||
		template.Repository.Stackfile != portainerComposePath {
		t.Errorf("%s: type %d from %+v; want a Compose stack (3) of %s in the repository", portainerTemplates,
			template.Type, template.Repository, portainerComposePath)
	}
	read, defaults := composeVariables(t, "../../"+portainerComposePath)
	asked := map[string]bool{}
	for _, variable := range template.Env {
		asked[variable.Name] = true
		if variable.Label == "" {
			t.Errorf("%s: %s has no label", portainerTemplates, variable.Name)
		}
		if isNeeded, isRead := read[variable.Name]; !isRead {
			t.Errorf("%s asks for %s, which %s does not read", portainerTemplates, variable.Name, portainerComposePath)
		} else if want := defaults[variable.Name]; !isNeeded && variable.Default != want {
			t.Errorf("%s: %s defaults to %q; want the compose file's %q", portainerTemplates, variable.Name,
				variable.Default, want)
		}
	}
	missing := slices.DeleteFunc(slices.Sorted(maps.Keys(read)), func(name string) bool { return asked[name] })
	if len(missing) > 0 {
		t.Errorf("%s does not ask for %v, which %s reads", portainerTemplates, missing, portainerComposePath)
	}
}

// composeVariables is each variable the compose file reads, whether it needs it, and the default it
// gives one: the first, which its first use reads (a media folder not set is empty there).
func composeVariables(t *testing.T, path string) (isNeeded map[string]bool, defaults map[string]string) {
	t.Helper()
	compose := composeLines(t, path)
	isNeeded, defaults = map[string]bool{}, map[string]string{}
	for _, match := range composeVariable.FindAllStringSubmatch(compose, -1) {
		isNeeded[match[1]] = isNeeded[match[1]] || match[2] == ":?"
	}
	for _, match := range composeDefault.FindAllStringSubmatch(compose, -1) {
		if _, isGiven := defaults[match[1]]; !isGiven {
			defaults[match[1]] = match[2]
		}
	}
	return isNeeded, defaults
}

// CasaOS's entry reads only the folders, user and time zone its store gives every app, so it installs
// with no value typed, and carries the store's metadata, naming the core as the app's page; its file,
// like every route's, is the one template's (D462 §10.2).
func TestCatalogue_TheCasaOSComposeReadsOnlyCasaOSsValuesAndCarriesItsMetadata(t *testing.T) {
	t.Parallel()
	text, err := os.ReadFile(casaOSCompose)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"\nname: orpheum\n", "\nx-casaos:\n", "\n  main: orpheum-core\n", "\n  port_map: \"8440\"\n",
		"SETUP-CODE.txt", "/DATA/AppData/orpheum:/appdata", "${ORPHEUM_MEDIA:-/DATA/Media}",
	} {
		if !strings.Contains(string(text), want) {
			t.Errorf("%s does not say %q", casaOSCompose, want)
		}
	}
	needed, _ := composeVariables(t, casaOSCompose)
	for name, isNeeded := range needed {
		if isNeeded {
			t.Errorf("%s needs %s; want every value defaulted, as the store installs with none typed", casaOSCompose,
				name)
		}
	}
	for _, foreign := range []string{"PATH_TO_APPDATA", "APPUSER_", "TIME_ZONE_VALUE"} {
		if strings.Contains(composeLines(t, casaOSCompose), foreign) {
			t.Errorf("%s reads %s, which CasaOS does not set", casaOSCompose, foreign)
		}
	}
}
