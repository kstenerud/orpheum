// SPDX-License-Identifier: AGPL-3.0-or-later

package deployment

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// synologyCompose is the compose file a Synology's Container Manager is given, which needs no .env
// file: making one is the hardest step on a Synology (the owner, 2026-11-09).
const synologyCompose = "../../deploy/synology/compose.yaml"

// synologyRenamed is OpenMediaVault's compose file's services as the Synology one says them: the
// household's three values (the media folder, the user, the time zone) by the anchors under "Yours",
// in place of its variables, and its data in the project's own folder, data/, in place of
// PATH_TO_APPDATA. The data folder's owner reaches orpheum-folders' chown by its environment, as an
// anchor is a whole value and cannot sit inside the command's string.
func synologyRenamed(text string) string {
	for _, rename := range []struct{ pattern, replacement string }{
		{`\$\{PATH_TO_APPDATA(:\?[^}]*)?\}/orpheum`, "./data"},
		{`"\$\{APPUSER_PUID:-1000\}:\$\{APPUSER_PGID:-100\}"`, "*user"},
		{`chown \$\{APPUSER_PUID:-1000\}:\$\{APPUSER_PGID:-100\} `, "chown $$$$OWNER "},
		{`(    command: \["mkdir[^\n]*\n)`, "${1}    environment:\n      OWNER: *user\n"},
		{`\$\{TIME_ZONE_VALUE:-Etc/UTC\}`, "*timezone"},
		{`\$\{ORPHEUM_MEDIA(:\?[^}]*)?\}`, "*media"},
	} {
		text = regexp.MustCompile(rename.pattern).ReplaceAllString(text, rename.replacement)
	}
	return text
}

// The Synology file is OpenMediaVault's with the household's values in the file and its data in the
// project's folder, and nothing else changed, so a change to one that is not made to the other fails
// here; and the values it names are under "Yours", each an anchor the services use.
func TestSynologyCompose_IsTheOMVOnesWithItsValuesInTheFile(t *testing.T) {
	t.Parallel()
	want, got := synologyRenamed(servicesOf(t, omvComposeFile)), servicesOf(t, synologyCompose)
	if got != want {
		wantLines, gotLines := strings.Split(want, "\n"), strings.Split(got, "\n")
		for index := range max(len(wantLines), len(gotLines)) {
			wantLine, gotLine := lineAt(wantLines, index), lineAt(gotLines, index)
			if wantLine != gotLine {
				t.Fatalf("%s differs from %s with its values in the file at its service line %d:\n got %q\nwant %q",
					synologyCompose, omvComposeFile, index+1, gotLine, wantLine)
			}
		}
	}
	text, err := os.ReadFile(synologyCompose)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"\nx-yours:\n", "\n  media: &media /volume1/video\n", "\n  user: &user \"1026:100\"\n",
		"\n  time-zone: &timezone Europe/Berlin\n",
	} {
		if !strings.Contains(string(text), want) {
			t.Errorf("%s does not say %q", synologyCompose, want)
		}
	}
	envOnly := []string{"PATH_TO_APPDATA", "ORPHEUM_MEDIA}", "ORPHEUM_MEDIA:", "APPUSER_", "TIME_ZONE_VALUE"}
	for _, needed := range envOnly {
		if strings.Contains(got, needed) {
			t.Errorf("%s still reads %s, which needs a .env file", synologyCompose, needed)
		}
	}
}
