// SPDX-License-Identifier: AGPL-3.0-or-later

package secrets

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The settings page says what each key and password is for in the person's words, from the web
// client's catalogue (D46, D51): every secret a person sets has its description there (the owner,
// 2026-10-21).
func TestRegistry_TheWebCatalogueDescribesEverySecret(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "apps", "web", "src", "locales",
		"en-settings.ts"))
	if err != nil {
		t.Fatal(err)
	}
	_, described, found := strings.Cut(string(source), "secretDescriptions: {")
	described, _, _ = strings.Cut(described, "}")
	if !found {
		t.Fatal("en-settings.ts has no secretDescriptions this test can read; say where they went")
	}
	for _, definition := range Registry() {
		if definition.IsMadeByServer {
			continue // the settings page does not show it
		}
		camel := regexp.MustCompile(`_([a-z0-9])`).ReplaceAllStringFunc(string(definition.Name),
			func(match string) string { return strings.ToUpper(match[1:]) })
		if !regexp.MustCompile(`\n\s+` + camel + `:`).MatchString(described) {
			t.Errorf("the catalogue does not say what %s is for", definition.Name)
		}
	}
}
