// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
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
