// SPDX-License-Identifier: AGPL-3.0-or-later

package settings

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// camelCase is a snake_case name as the web catalogue keys it (catalogue-key.ts).
func camelCase(name string) string {
	return regexp.MustCompile(`_([a-z0-9])`).ReplaceAllStringFunc(name, func(match string) string {
		return strings.ToUpper(match[1:])
	})
}

// The settings page says what each setting is for and names each section in the person's words,
// from the web client's catalogue (D46, D51): every setting has its description there, and every
// section its name (the owner, 2026-10-21).
func TestRegistry_TheWebCatalogueDescribesEverySettingAndNamesEverySection(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "apps", "web", "src", "locales",
		"en-settings.ts"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	sections, descriptions, found := strings.Cut(text, "descriptions: {")
	if !found {
		t.Fatal("en-settings.ts has no descriptions this test can read; say where they went")
	}
	for _, definition := range Registry() {
		if !regexp.MustCompile(`\n\s+` + camelCase(string(definition.Key)) + `:`).MatchString(descriptions) {
			t.Errorf("the catalogue does not say what %s is for", definition.Key)
		}
	}
	for _, section := range Sections() {
		if !regexp.MustCompile(`\n\s+` + camelCase(string(section)) + `:`).MatchString(sections) {
			t.Errorf("the catalogue does not name the section %s", section)
		}
	}
}

// The settings page names each setting above its field in the person's words, its key in small type
// under it (ui-audit-2026-11-04.md §3, Settings): every setting has its label in the web catalogue.
func TestRegistry_TheWebCatalogueLabelsEverySetting(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "apps", "web", "src", "locales",
		"en-setting-labels.ts"))
	if err != nil {
		t.Fatal(err)
	}
	_, labels, found := strings.Cut(string(source), "\n  labels: {")
	if !found {
		t.Fatal("en-setting-labels.ts has no labels this test can read; say where they went")
	}
	for _, definition := range Registry() {
		if !regexp.MustCompile(`\n\s+` + camelCase(string(definition.Key)) + `:`).MatchString(labels) {
			t.Errorf("the catalogue has no label for %s", definition.Key)
		}
	}
}

// The chunk length's help says it is a floor its film's keyframes decide, not each chunk's length:
// a chunk ends only at a keyframe, and most film time plays from chunks of 6-7 s at the 2 s setting
// (D414 §5).
func TestRegistry_TheChunkLengthsHelpSaysItsKeyframesDecide(t *testing.T) {
	t.Parallel()
	definition, ok := Lookup(HLSSegmentLengthSeconds)
	if !ok {
		t.Fatalf("no definition for %s", HLSSegmentLengthSeconds)
	}
	for _, want := range []string{"the shortest chunk", "keyframe", "D414 §5"} {
		if !strings.Contains(definition.Description, want) {
			t.Errorf("the help %q says nothing of %q", definition.Description, want)
		}
	}
}
