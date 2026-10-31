// SPDX-License-Identifier: AGPL-3.0-or-later

package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// homeReaders are the API's functions that may ask isFromHome without a principal of their own:
// the question itself, and the log's label of how a request came (pathOf), which grants nothing.
var homeReaders = map[string]bool{"isFromHome": true, "pathOf": true}

// findHomeAsCredential returns every function of the API that asks whether a request came from home
// (isFromHome) with no signed-in person in hand: it neither takes an auth.Principal nor calls one of
// the server's require… methods, which authenticate the request. Being at home, or on the port only
// the server's own machine reaches, is a condition on a session, never one: Plex2Shell's second step
// was a server trusting a request for where it came from (research/security-week-2026-10-30.md §5).
func findHomeAsCredential(files []sourceFile) []violation {
	var violations []violation
	for _, file := range files {
		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, file.path, file.contents, parser.SkipObjectResolution)
		if err != nil {
			violations = append(violations, violation{where: file.path, detail: "does not parse: " + err.Error()})
			continue
		}
		for _, declaration := range parsed.Decls {
			function, isFunction := declaration.(*ast.FuncDecl)
			if !isFunction || function.Body == nil || homeReaders[function.Name.Name] {
				continue
			}
			if callsNamed(function.Body, "isFromHome") && !hasPrincipal(function) {
				violations = append(violations, violation{
					where: fileSet.Position(function.Pos()).String(),
					detail: function.Name.Name + " asks whether a request came from home with no signed-in " +
						"person: take an auth.Principal, or call a require… method first",
				})
			}
		}
	}
	return violations
}

// callsNamed reports whether body calls a function or method named name.
func callsNamed(body *ast.BlockStmt, name string) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		if call, isCall := node.(*ast.CallExpr); isCall && calledName(call) == name {
			found = true
		}
		return !found
	})
	return found
}

// calledName is the name a call calls: a function's or a method's, or "" for anything else.
func calledName(call *ast.CallExpr) string {
	switch called := call.Fun.(type) {
	case *ast.Ident:
		return called.Name
	case *ast.SelectorExpr:
		return called.Sel.Name
	default:
		return ""
	}
}

// hasPrincipal reports whether function has a signed-in person in hand: a parameter of type
// auth.Principal, or a call of a require… method, each of which authenticates the request.
func hasPrincipal(function *ast.FuncDecl) bool {
	for _, field := range function.Type.Params.List {
		if selector, isSelector := field.Type.(*ast.SelectorExpr); isSelector && selector.Sel.Name == "Principal" {
			return true
		}
	}
	found := false
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if call, isCall := node.(*ast.CallExpr); isCall && strings.HasPrefix(calledName(call), "require") {
			found = true
		}
		return !found
	})
	return found
}

func TestArch_HomeIsNeverACredential(t *testing.T) {
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
	files := readSourceFiles(t, api)
	asked := 0
	for _, file := range files {
		asked += strings.Count(file.contents, "isFromHome(")
	}
	if asked < 3 {
		t.Fatalf("isFromHome is asked %d times in %s; the rule would pass vacuously", asked, apiPath)
	}
	reportViolations(t, findHomeAsCredential(files))
}

// The rule's own check: a handler that grants for home alone is found; one that asks with a session,
// or with a principal it was given, and the log's label, are not.
func TestHomeIsNeverACredential_FindsAGrantForHomeAlone(t *testing.T) {
	t.Parallel()
	source := `package api

func (server *Server) open(ctx context.Context) error {
	if isFromHome(ctx) {
		return nil
	}
	return errRefused
}

func (server *Server) guarded(ctx context.Context, input *Input) error {
	_, principal, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil || !isFromHome(ctx) {
		return errRefused
	}
	return use(principal)
}

func administersHere(ctx context.Context, principal auth.Principal) bool {
	return principal.IsAdmin() && isFromHome(ctx)
}

func pathOf(ctx context.Context) string {
	if isFromHome(ctx) {
		return "home"
	}
	return "away"
}
`
	found := findHomeAsCredential([]sourceFile{{path: "synthetic.go", contents: source}})
	if len(found) != 1 || !strings.Contains(found[0].detail, "open asks") {
		t.Errorf("found %v; want the one handler that grants for home alone", found)
	}
}
