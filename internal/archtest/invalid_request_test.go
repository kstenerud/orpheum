// SPDX-License-Identifier: AGPL-3.0-or-later

package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"testing"
)

// errorsFile is the one file of internal/core/api that builds an invalid_request directly: its
// constructor, invalidRequestAt, and huma's own validation errors.
const errorsFile = "errors.go"

// invalidRequestNames are the identifiers that build an invalid_request without saying where.
var invalidRequestNames = map[string]bool{"CodeInvalidRequest": true, "InvalidRequestDetailBody": true}

// findUnplacedInvalidRequests returns every invalid_request in the API's handlers that could
// leave without naming where the problem is: one built from its code or detail outside
// errors.go, and an invalidRequestAt whose location is empty or cannot be read. A client names
// the field in its own words from the location (D188 §1); without one it can only say
// "something", as the library form did on devnas (2026-10-01).
func findUnplacedInvalidRequests(files []sourceFile) []violation {
	var violations []violation
	for _, file := range files {
		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, file.path, file.contents, parser.SkipObjectResolution)
		if err != nil {
			violations = append(violations, violation{where: file.path, detail: "does not parse: " + err.Error()})
			continue
		}
		isErrorsFile := filepath.Base(file.path) == errorsFile
		ast.Inspect(parsed, func(node ast.Node) bool {
			where := func() string { return fileSet.Position(node.Pos()).String() }
			switch typed := node.(type) {
			case *ast.Ident:
				if !isErrorsFile && invalidRequestNames[typed.Name] {
					violations = append(violations, violation{
						where: where(),
						detail: "builds an invalid_request from " + typed.Name +
							"; use invalidRequestAt, which names where",
					})
				}
			case *ast.CallExpr:
				if problem := locationProblem(typed); problem != "" {
					violations = append(violations, violation{where: where(), detail: problem})
				}
			}
			return true
		})
	}
	return violations
}

// locationProblem says what is wrong with an invalidRequestAt call's location, or "" when the
// call is not one or its location is a non-empty string literal, alone or followed by more
// ("body.valuesByKey." + key).
func locationProblem(call *ast.CallExpr) string {
	name, isIdent := call.Fun.(*ast.Ident)
	if !isIdent || name.Name != "invalidRequestAt" {
		return ""
	}
	if len(call.Args) != 3 {
		return "calls invalidRequestAt with a shape this rule cannot read"
	}
	location := call.Args[1]
	if sum, isSum := location.(*ast.BinaryExpr); isSum && sum.Op == token.ADD {
		location = sum.X
	}
	literal, isLiteral := location.(*ast.BasicLit)
	if !isLiteral || literal.Kind != token.STRING {
		return "names its location by an expression this rule cannot read; start it with a string literal"
	}
	if text, err := strconv.Unquote(literal.Value); err != nil || text == "" {
		return "names no location: a field such as body.name, or body for the request as a whole"
	}
	return ""
}

// TestArch_EveryInvalidRequestSaysWhere checks internal/core/api: every invalid_request a
// handler sends is built by invalidRequestAt with a location, so a client can name the field.
func TestArch_EveryInvalidRequestSaysWhere(t *testing.T) {
	t.Parallel()
	var api []listedPackage
	for _, listed := range loadModulePackages(t) {
		if listed.ImportPath == apiPath {
			api = append(api, listedPackage{Dir: listed.Dir, GoFiles: listed.GoFiles})
		}
	}
	if len(api) == 0 || len(api[0].GoFiles) == 0 {
		t.Fatalf("go list found no source for %s; the rule would pass vacuously", apiPath)
	}
	reportViolations(t, findUnplacedInvalidRequests(readSourceFiles(t, api)))
}

func TestFindUnplacedInvalidRequests_CatchesEachWayToSayNowhere(t *testing.T) {
	t.Parallel()
	placed := `package api
func a() { _ = invalidRequestAt(400, "body.name", "m"); _ = invalidRequestAt(422, "body.valuesByKey."+key, "m") }`
	if got := findUnplacedInvalidRequests([]sourceFile{{path: "a.go", contents: placed}}); len(got) != 0 {
		t.Errorf("located refusals: got %v, want none", got)
	}
	for _, contents := range []string{
		`package api
func a() { _ = newError(400, CodeInvalidRequest, "m") }`,
		`package api
func a() { _ = newDetailedError(400, "m", InvalidRequestDetailBody{}) }`,
		`package api
func a() { _ = invalidRequestAt(400, "", "m") }`,
		`package api
func a() { _ = invalidRequestAt(400, where, "m") }`,
	} {
		if got := findUnplacedInvalidRequests([]sourceFile{{path: "a.go", contents: contents}}); len(got) != 1 {
			t.Errorf("%s: got %v, want one violation", contents, got)
		}
	}
	inErrorsFile := `package api
func a() { _ = newError(400, CodeInvalidRequest, "m") }`
	if got := findUnplacedInvalidRequests([]sourceFile{{path: "x/errors.go", contents: inErrorsFile}}); len(got) != 0 {
		t.Errorf("errors.go builds the constructor itself: got %v, want none", got)
	}
}
