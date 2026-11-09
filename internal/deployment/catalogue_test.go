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
	// The compose file as a catalogue names it, from the repository's root.
	omvComposePath = "deploy/omv/compose.yaml"
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
		template.Repository.Stackfile != omvComposePath {
		t.Errorf("%s: type %d from %+v; want a Compose stack (3) of %s in the repository", portainerTemplates,
			template.Type, template.Repository, omvComposePath)
	}
	read, defaults := composeVariables(t)
	asked := map[string]bool{}
	for _, variable := range template.Env {
		asked[variable.Name] = true
		if variable.Label == "" {
			t.Errorf("%s: %s has no label", portainerTemplates, variable.Name)
		}
		if isNeeded, isRead := read[variable.Name]; !isRead {
			t.Errorf("%s asks for %s, which %s does not read", portainerTemplates, variable.Name, omvComposeFile)
		} else if want := defaults[variable.Name]; !isNeeded && variable.Default != want {
			t.Errorf("%s: %s defaults to %q; want the compose file's %q", portainerTemplates, variable.Name,
				variable.Default, want)
		}
	}
	missing := slices.DeleteFunc(slices.Sorted(maps.Keys(read)), func(name string) bool { return asked[name] })
	if len(missing) > 0 {
		t.Errorf("%s does not ask for %v, which %s reads", portainerTemplates, missing, omvComposeFile)
	}
}

// composeVariables is each variable the compose file reads, whether it needs it, and the default it
// gives one: the first, which its first use reads (a media folder not set is empty there).
func composeVariables(t *testing.T) (isNeeded map[string]bool, defaults map[string]string) {
	t.Helper()
	compose := omvCompose(t)
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

// casaOSRenamed is the compose file's services as CasaOS's entry says them: the same, but CasaOS's
// folders, and the names of the user and time zone its app store gives every app, in place of
// OpenMediaVault's.
func casaOSRenamed(text string) string {
	for _, rename := range []struct{ pattern, replacement string }{
		{`\$\{PATH_TO_APPDATA(:\?[^}]*)?\}`, "/DATA/AppData"},
		{`\$\{APPUSER_PUID:-`, "${PUID:-"},
		{`\$\{APPUSER_PGID:-`, "${PGID:-"},
		{`\$\{TIME_ZONE_VALUE:-`, "${TZ:-"},
		{`\$\{ORPHEUM_MEDIA(:\?[^}]*)?\}`, "${ORPHEUM_MEDIA:-/DATA/Media}"},
	} {
		text = regexp.MustCompile(rename.pattern).ReplaceAllLiteralString(text, rename.replacement)
	}
	return text
}

// servicesOf is a compose file's services and networks, its comments left out: from "services:" to
// the first top-level key after its networks, or the end.
func servicesOf(t *testing.T, path string) string {
	t.Helper()
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	isIn := false
	for line := range strings.SplitSeq(string(text), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") {
			isIn = line == "services:" || line == "networks:"
		}
		if isIn {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// CasaOS's entry is OpenMediaVault's compose file with CasaOS's folders and names, and nothing else
// changed, so a change to one that is not made to the other fails here; and it carries the store's
// metadata, naming the core as the app's page.
func TestCatalogue_TheCasaOSComposeIsTheOMVOnesWithCasaOSsFolders(t *testing.T) {
	t.Parallel()
	want, got := casaOSRenamed(servicesOf(t, omvComposeFile)), servicesOf(t, casaOSCompose)
	if got != want {
		wantLines, gotLines := strings.Split(want, "\n"), strings.Split(got, "\n")
		for index := range max(len(wantLines), len(gotLines)) {
			wantLine, gotLine := lineAt(wantLines, index), lineAt(gotLines, index)
			if wantLine != gotLine {
				t.Fatalf("%s differs from %s with CasaOS's folders at its service line %d:\n got %q\nwant %q",
					casaOSCompose, omvComposeFile, index+1, gotLine, wantLine)
			}
		}
	}
	text, err := os.ReadFile(casaOSCompose)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"\nx-casaos:\n", "\n  main: orpheum-core\n", "\n  port_map: \"8440\"\n", "SETUP-CODE.txt",
	} {
		if !strings.Contains(string(text), want) {
			t.Errorf("%s does not say %q", casaOSCompose, want)
		}
	}
}

// lineAt is lines[index], or "" past its end.
func lineAt(lines []string, index int) string {
	if index < len(lines) {
		return lines[index]
	}
	return ""
}
