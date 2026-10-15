// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// untypedPositions returns where a generated file uses interface{} or any for a value: sqlc's
// fallback for a SQLite column it could not type (standards/sql.md §3). The DBTX interface is
// exempt, because its variadic arguments are database/sql's own signature.
func untypedPositions(t *testing.T, name, source string) []string {
	t.Helper()
	files := token.NewFileSet()
	parsed, err := parser.ParseFile(files, name, source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	var found []string
	ast.Inspect(parsed, func(node ast.Node) bool {
		if spec, isType := node.(*ast.TypeSpec); isType && spec.Name.Name == "DBTX" {
			return false
		}
		switch typed := node.(type) {
		case *ast.InterfaceType:
			if typed.Methods == nil || len(typed.Methods.List) == 0 {
				found = append(found, files.Position(typed.Pos()).String())
			}
		case *ast.Ident:
			if typed.Name == "any" {
				found = append(found, files.Position(typed.Pos()).String())
			}
		}
		return true
	})
	return found
}

func TestGeneratedQueries_HaveNoUntypedValues(t *testing.T) {
	t.Parallel()
	sources, err := filepath.Glob(filepath.Join("sqlcgen", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) == 0 {
		t.Fatal("found no generated code; the check would pass vacuously")
	}
	for _, source := range sources {
		contents, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		for _, position := range untypedPositions(t, source, string(contents)) {
			t.Errorf("%s: sqlc could not type a value; give the column an override in sqlc.yaml", position)
		}
	}
}

func TestUntypedPositions_FindsAnUntypedFieldButNotDBTX(t *testing.T) {
	t.Parallel()
	source := strings.Join([]string{
		"package sqlcgen",
		"type DBTX interface { ExecContext(ctx context.Context, query string, args ...interface{}) error }",
		"type Row struct { Typed int64; Untyped interface{}; AlsoUntyped any }",
	}, "\n")
	if found := untypedPositions(t, "synthetic.go", source); len(found) != 2 {
		t.Errorf("found %v, want the two untyped fields and nothing in DBTX", found)
	}
}

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

// callerSources are the module's Go sources outside the generated code, tests too: a query only a
// test reads through is a fixture's, kept on purpose.
func callerSources(t *testing.T) string {
	t.Helper()
	var all strings.Builder
	err := filepath.WalkDir(filepath.Join("..", ".."), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && (entry.Name() == "node_modules" || strings.HasPrefix(entry.Name(), ".") && path != filepath.Join("..", "..")) {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.Contains(path, string(filepath.Separator)+"sqlcgen"+string(filepath.Separator)) {
			return nil
		}
		contents, readErr := os.ReadFile(path)
		all.Write(contents)
		return readErr
	})
	if err != nil {
		t.Fatal(err)
	}
	return all.String()
}

// uncalledBefore are the queries nothing called when TestGeneratedQueries_EachHasACaller came
// (2026-10-15), left for the owner to remove or bring back into use: CountEarlierMisses is named by
// plans/scenarios.md as where an artwork miss is counted, which a removal would leave stale.
var uncalledBefore = map[string]bool{"CountEarlierMisses": true, "GetFetchFailure": true}

// Every query the generated code holds has a caller: one left behind when its last caller went is
// dead code nothing notices, as D299's removed forgetting of realm keys would have been had its
// query stayed.
func TestGeneratedQueries_EachHasACaller(t *testing.T) {
	t.Parallel()
	sources, err := filepath.Glob(filepath.Join("sqlcgen", "*.sql.go"))
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
			if !uncalledBefore[method] && !regexp.MustCompile(`\.`+method+`\b`).MatchString(callers) {
				t.Errorf("%s: %s has no caller; remove its query", source, method)
			}
		}
	}
}
