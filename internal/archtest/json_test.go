// SPDX-License-Identifier: AGPL-3.0-or-later

package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"testing"
)

// handBuiltJSON matches a string that opens a JSON object with a key: `{"name":`. A document the
// code emits is built from a struct by encoding/json, so its names have one copy, the struct's
// tags; a string spelling them out is a second copy a rename cannot see (agent failures A13).
var handBuiltJSON = regexp.MustCompile(`\{\s*"[A-Za-z_][A-Za-z0-9_]*"\s*:`)

// findHandBuiltJSON returns every string literal in files that holds a JSON object's key.
func findHandBuiltJSON(files []sourceFile) []violation {
	var violations []violation
	for _, file := range files {
		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, file.path, file.contents, parser.SkipObjectResolution)
		if err != nil {
			violations = append(violations, violation{where: file.path, detail: "does not parse: " + err.Error()})
			continue
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			literal, isLiteral := node.(*ast.BasicLit)
			if !isLiteral || literal.Kind != token.STRING {
				return true
			}
			text, err := strconv.Unquote(literal.Value)
			if err == nil && handBuiltJSON.MatchString(text) {
				violations = append(violations, violation{
					where:  fileSet.Position(literal.Pos()).String(),
					detail: "spells out a JSON object's keys; encode a struct with encoding/json instead",
				})
			}
			return true
		})
	}
	return violations
}

// TestArch_NoHandBuiltJSON checks the files the module compiles, tests aside: none spells out a
// JSON document by hand, so a field renamed in its struct is renamed wherever the document is
// written. Tests may: a test pins the wire format by writing it out.
func TestArch_NoHandBuiltJSON(t *testing.T) {
	t.Parallel()
	var compiled []listedPackage
	for _, listed := range loadModulePackages(t) {
		compiled = append(compiled, listedPackage{Dir: listed.Dir, GoFiles: listed.GoFiles})
	}
	files := readSourceFiles(t, compiled)
	if len(files) == 0 {
		t.Fatal("go list found no source files; the rule would pass vacuously")
	}
	reportViolations(t, findHandBuiltJSON(files))
}
