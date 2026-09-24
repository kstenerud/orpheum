// SPDX-License-Identifier: GPL-3.0-or-later

package archtest

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// modulePath is this module's import path; every rule is phrased relative to it.
const modulePath = "github.com/kstenerud/orpheum"

// listedPackage is the part of `go list -json` output the rules read. The keys are the go
// command's, which is why this struct is exempt from the camelCase rule in .golangci.yml.
type listedPackage struct {
	ImportPath   string   `json:"ImportPath"`
	Name         string   `json:"Name"`
	Dir          string   `json:"Dir"`
	GoFiles      []string `json:"GoFiles"`
	TestGoFiles  []string `json:"TestGoFiles"`
	XTestGoFiles []string `json:"XTestGoFiles"`
	Imports      []string `json:"Imports"`
	Deps         []string `json:"Deps"`
	TestImports  []string `json:"TestImports"`
	XTestImports []string `json:"XTestImports"`
}

// sourceFile is one Go file's path and contents, for the rules that read source text.
type sourceFile struct {
	path     string
	contents string
}

// loadModulePackages lists every package of this module that the rules govern. A missing go
// command fails the test rather than skipping it (D190 §2): these tests are the gate.
func loadModulePackages(t *testing.T) []listedPackage {
	t.Helper()
	command := exec.CommandContext(t.Context(), "go", "list", "-json",
		modulePath+"/cmd/...", modulePath+"/internal/...")
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list failed: %v\n%s", err, stderr.String())
	}
	packages, err := decodePackages(output)
	if err != nil {
		t.Fatalf("decode go list output: %v", err)
	}
	if len(packages) == 0 {
		t.Fatal("go list returned no packages; the rules would pass vacuously")
	}
	return packages
}

// loadCoreImportGraph lists every package the core depends on, the standard library and
// third-party modules included, as a map from import path to its direct imports, so a rule can
// follow the route a dependency takes rather than only see that it is there.
func loadCoreImportGraph(t *testing.T) map[string][]string {
	t.Helper()
	command := exec.CommandContext(t.Context(), "go", "list", "-deps", "-json", corePath+"/...")
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list -deps failed: %v\n%s", err, stderr.String())
	}
	packages, err := decodePackages(output)
	if err != nil {
		t.Fatalf("decode go list output: %v", err)
	}
	graph := map[string][]string{}
	for _, listed := range packages {
		graph[listed.ImportPath] = listed.Imports
	}
	if len(graph) == 0 {
		t.Fatal("go list -deps returned no packages; the rule would pass vacuously")
	}
	return graph
}

// decodePackages reads the stream of JSON objects `go list -json` prints.
func decodePackages(output []byte) ([]listedPackage, error) {
	decoder := json.NewDecoder(bytes.NewReader(output))
	var packages []listedPackage
	for {
		var listed listedPackage
		err := decoder.Decode(&listed)
		if errors.Is(err, io.EOF) {
			return packages, nil
		}
		if err != nil {
			return nil, err
		}
		packages = append(packages, listed)
	}
}

// readSourceFiles returns every Go file, test files included, of the given packages.
func readSourceFiles(t *testing.T, packages []listedPackage) []sourceFile {
	t.Helper()
	var files []sourceFile
	for _, listed := range packages {
		names := append(append(append([]string(nil), listed.GoFiles...), listed.TestGoFiles...), listed.XTestGoFiles...)
		for _, name := range names {
			path := filepath.Join(listed.Dir, name)
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			files = append(files, sourceFile{path: path, contents: string(contents)})
		}
	}
	return files
}

// readDocFile returns the contents of a package's doc.go and whether it has one.
func readDocFile(t *testing.T, listed listedPackage) (string, bool) {
	t.Helper()
	path := filepath.Join(listed.Dir, "doc.go")
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents), true
}

// readQueryFiles returns every .sql file in directory. An empty directory is not an error:
// until M1 there are no queries, and the rule's self-test is what shows it can fail.
func readQueryFiles(t *testing.T, directory string) []sourceFile {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(directory, "*.sql"))
	if err != nil {
		t.Fatalf("list %s: %v", directory, err)
	}
	var files []sourceFile
	for _, path := range paths {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		files = append(files, sourceFile{path: path, contents: string(contents)})
	}
	return files
}
