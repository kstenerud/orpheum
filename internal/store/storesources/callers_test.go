// SPDX-License-Identifier: AGPL-3.0-or-later

package storesources

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// moduleRoot is the module's directory, from this package's.
var moduleRoot = filepath.Join("..", "..", "..")

// queryMethods are the methods a generated file declares on *Queries: one per query.
func queryMethods(t *testing.T, name string, source []byte) []string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), name, source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	var methods []string
	for _, declared := range parsed.Decls {
		function, isFunction := declared.(*ast.FuncDecl)
		if !isFunction || function.Recv == nil || len(function.Recv.List) != 1 {
			continue
		}
		if star, isPointer := function.Recv.List[0].Type.(*ast.StarExpr); isPointer {
			if receiver, isNamed := star.X.(*ast.Ident); isNamed && receiver.Name == "Queries" {
				methods = append(methods, function.Name.Name)
			}
		}
	}
	return methods
}

// isSkippedDirectory is a directory no Go source of the module's is in: node_modules, and a hidden
// one other than the root itself, which is named "..".
func isSkippedDirectory(path string, entry os.DirEntry) bool {
	return entry.IsDir() && path != moduleRoot &&
		(entry.Name() == "node_modules" || strings.HasPrefix(entry.Name(), "."))
}

// callerSources are the module's Go sources outside the generated code, tests too: a query only a
// test reads through is a fixture's, kept on purpose.
func callerSources(t *testing.T) string {
	t.Helper()
	var all strings.Builder
	err := filepath.WalkDir(moduleRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if isSkippedDirectory(path, entry) {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.Contains(path, string(filepath.Separator)+"sqlcgen"+string(filepath.Separator)) {
			return nil
		}
		contents, readErr := os.ReadFile(path) //nolint:gosec // the module's own files
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		_, _ = all.Write(contents) // a strings.Builder's Write never fails
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return all.String()
}

// Every query the generated code holds has a caller: one left behind when its last caller went is
// dead code nothing notices, as D299's removed forgetting of realm keys would have been had its
// query stayed.
func TestGeneratedQueries_EachHasACaller(t *testing.T) {
	t.Parallel()
	sources, err := filepath.Glob(filepath.Join("..", "sqlcgen", "*.sql.go"))
	if err != nil || len(sources) == 0 {
		t.Fatalf("found no generated queries (%v); the check would pass vacuously", err)
	}
	callers := callerSources(t)
	for _, source := range sources {
		contents, readErr := os.ReadFile(source)
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, method := range queryMethods(t, source, contents) {
			if !regexp.MustCompile(`\.` + method + `\b`).MatchString(callers) {
				t.Errorf("%s: %s has no caller; remove its query", source, method)
			}
		}
	}
}
