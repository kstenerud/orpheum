// SPDX-License-Identifier: AGPL-3.0-or-later

package archtest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
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
// command fails the test rather than skipping it (D190 §2): these tests are the gate. The first
// call in the process runs `go list`; every later one shares its result (test architecture
// review, 2026-10-26, finding 8: about 22 calls a run, each its own process).
func loadModulePackages(t *testing.T) []listedPackage {
	t.Helper()
	modulePackagesCache.once.Do(func() {
		command := exec.CommandContext(t.Context(), "go", "list", "-json",
			modulePath+"/cmd/...", modulePath+"/internal/...", publicPath+"/...")
		var stderr bytes.Buffer
		command.Stderr = &stderr
		output, err := command.Output()
		if err != nil {
			modulePackagesCache.err = fmt.Errorf("go list failed: %w\n%s", err, stderr.String())
			return
		}
		packages, err := decodePackages(output)
		if err != nil {
			modulePackagesCache.err = fmt.Errorf("decode go list output: %w", err)
			return
		}
		if len(packages) == 0 {
			modulePackagesCache.err = errors.New("go list returned no packages; the rules would pass vacuously")
			return
		}
		modulePackagesCache.packages = packages
	})
	if modulePackagesCache.err != nil {
		t.Fatal(modulePackagesCache.err)
	}
	return modulePackagesCache.packages
}

// modulePackagesCache is loadModulePackages' result, loaded once per process: `go list` walks the
// whole module graph, which costs seconds repeated by each of archtest's many callers.
var modulePackagesCache struct {
	once     sync.Once
	packages []listedPackage
	err      error
}

// loadCoreImportGraph lists every package the core depends on, the standard library and
// third-party modules included, as a map from import path to its direct imports, so a rule can
// follow the route a dependency takes rather than only see that it is there. The first call in
// the process runs `go list -deps`; every later one shares its result (finding 8).
func loadCoreImportGraph(t *testing.T) map[string][]string {
	t.Helper()
	coreImportGraphCache.once.Do(func() {
		command := exec.CommandContext(t.Context(), "go", "list", "-deps", "-json", corePath+"/...")
		var stderr bytes.Buffer
		command.Stderr = &stderr
		output, err := command.Output()
		if err != nil {
			coreImportGraphCache.err = fmt.Errorf("go list -deps failed: %w\n%s", err, stderr.String())
			return
		}
		packages, err := decodePackages(output)
		if err != nil {
			coreImportGraphCache.err = fmt.Errorf("decode go list output: %w", err)
			return
		}
		graph := map[string][]string{}
		for _, listed := range packages {
			graph[listed.ImportPath] = listed.Imports
		}
		if len(graph) == 0 {
			coreImportGraphCache.err = errors.New("go list -deps returned no packages; the rule would pass vacuously")
			return
		}
		coreImportGraphCache.graph = graph
	})
	if coreImportGraphCache.err != nil {
		t.Fatal(coreImportGraphCache.err)
	}
	return coreImportGraphCache.graph
}

// coreImportGraphCache is loadCoreImportGraph's result, loaded once per process.
var coreImportGraphCache struct {
	once  sync.Once
	graph map[string][]string
	err   error
}

// loadModulePackages shares one go list per process (finding 8): two calls, as two of archtest's
// many tests would each make, return the same slice, not two each parsed from a go list of its
// own.
func TestLoadModulePackages_SharesOneGoListPerProcess(t *testing.T) {
	t.Parallel()
	first := loadModulePackages(t)
	second := loadModulePackages(t)
	if len(first) == 0 || len(second) == 0 {
		t.Fatal("loadModulePackages returned no packages")
	}
	if reflect.ValueOf(first).Pointer() != reflect.ValueOf(second).Pointer() {
		t.Error("two calls to loadModulePackages did not share one result: each ran its own go list")
	}
}

// loadCoreImportGraph shares one go list per process too (finding 8).
func TestLoadCoreImportGraph_SharesOneGoListPerProcess(t *testing.T) {
	t.Parallel()
	first := loadCoreImportGraph(t)
	second := loadCoreImportGraph(t)
	if len(first) == 0 || len(second) == 0 {
		t.Fatal("loadCoreImportGraph returned no packages")
	}
	if reflect.ValueOf(first).Pointer() != reflect.ValueOf(second).Pointer() {
		t.Error("two calls to loadCoreImportGraph did not share one result: each ran its own go list")
	}
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
