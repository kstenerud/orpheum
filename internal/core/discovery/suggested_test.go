// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/trust"
)

// ownersDomains are the owner's names (D390, D391 §5): the paid service's, staging's and the
// website's.
var ownersDomains = regexp.MustCompile(`orpheum\.(cc|dev|mov)\b|` +
	`orpheum-(app|app-test|client|client-test|core|test)\.com\b`)

// The one shipped source that may name them: this package's suggestion.
const suggestionFile = "internal/core/discovery/suggested.go"

// shippedSourceExtensions are the sources a build ships: Go, the web client's TypeScript and
// JavaScript, and the templates and scripts the binary carries or a deployment runs, the install
// script among them.
var shippedSourceExtensions = []string{".go", ".ts", ".tsx", ".mts", ".js", ".mjs", ".sh", ".html", ".yaml", ".toml"}

// No shipped source but suggested.go names the owner's domains, so the suggestion has one home and a
// name changes in one place (D391 §7): the install script says the loader its discovery named, not
// one of its own. Tests, the browser tests' tree, docs and what a build makes or fetches
// (node_modules, dist, hidden directories) are not shipped sources.
func TestSuggested_IsTheOnlyShippedSourceThatNamesTheOwnersDomains(t *testing.T) {
	t.Parallel()
	naming, err := sourcesNamingTheOwnersDomains(repositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(naming) != 0 {
		t.Errorf("shipped sources name the owner's domains, which only %s may:\n%s", suggestionFile,
			strings.Join(naming, "\n"))
	}
	suggestion, err := os.ReadFile(filepath.Join(repositoryRoot, suggestionFile))
	if err != nil || !ownersDomains.Match(suggestion) {
		t.Errorf("%s names none of the owner's domains (%v): the scan would find nothing", suggestionFile, err)
	}
}

// repositoryRoot is the repository's root, from this package's directory, where its tests run.
const repositoryRoot = "../../.."

// sourcesNamingTheOwnersDomains are the shipped sources under root but the suggestion that name one
// of the owner's domains, each with the name it found.
func sourcesNamingTheOwnersDomains(root string) ([]string, error) {
	var naming []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		switch {
		case walkErr != nil:
			return walkErr
		case entry.IsDir():
			if path != root && isUnshippedDirectory(root, path) {
				return filepath.SkipDir
			}
			return nil
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil || !isShippedSource(entry.Name()) || filepath.ToSlash(relative) == suggestionFile {
			return relErr
		}
		contents, readErr := os.ReadFile(path) //nolint:gosec // the repository's own sources
		if found := ownersDomains.Find(contents); found != nil {
			naming = append(naming, filepath.ToSlash(relative)+": "+string(found))
		}
		return readErr
	})
	return naming, err
}

// isUnshippedDirectory is whether the directory at path holds nothing a build ships: a hidden one,
// what a build fetches or makes, test data, or the browser tests.
func isUnshippedDirectory(root, path string) bool {
	name := filepath.Base(path)
	return strings.HasPrefix(name, ".") || name == "node_modules" || name == "dist" || name == "testdata" ||
		filepath.Clean(path) == filepath.Join(root, "web", "e2e")
}

// The suggestion is an issuer, and each genesis this build holds, once the owner has put it there, is
// a chain's first statement of the right kind, signed by one of its own keys: an owner's mistake
// fails here, not at a household's enrolment or update.
func TestSuggested_IsAnIssuerAndItsRootKeysAreKeys(t *testing.T) {
	t.Parallel()
	suggested := Suggested()
	if issuer, err := addressdoc.Issuer(suggested.Issuer); err != nil || issuer != suggested.Issuer {
		t.Errorf("the suggested issuer %q is not an issuer as typed: %q, %v", suggested.Issuer, issuer, err)
	}
	for kind, genesis := range map[trust.ChainKind]trust.Signed{
		trust.ChainDiscovery: suggested.Genesis, trust.ChainPublisher: Publisher().Genesis,
	} {
		if genesis.Payload == "" {
			continue
		}
		if _, err := trust.Genesis(kind, genesis); err != nil {
			t.Errorf("the compiled %s genesis: %v", kind, err)
		}
	}
	for _, location := range Publisher().Locations {
		if issuer, err := addressdoc.Issuer(location); err != nil || issuer != location {
			t.Errorf("the publisher's location %q is not an origin: %v", location, err)
		}
	}
}

// isShippedSource is whether name is a source a build ships, not a test's.
func isShippedSource(name string) bool {
	if strings.HasSuffix(name, "_test.go") || strings.Contains(name, ".test.") || strings.Contains(name, ".spec.") {
		return false
	}
	for _, extension := range shippedSourceExtensions {
		if strings.HasSuffix(name, extension) {
			return true
		}
	}
	return false
}
