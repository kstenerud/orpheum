// SPDX-License-Identifier: AGPL-3.0-or-later

package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// apiPath is the HTTP API, whose handlers are registered with huma.
const apiPath = corePath + "/api"

// findMisnamedHandlers returns every huma.Register call whose handler is not the method named
// for its operation: a handler is found from the API document by its OperationID, and a reader
// at the registration sees one name for both. A registration this rule cannot read, an
// OperationID that is not a string literal or a handler that is not a method value, is reported
// too, so the rule cannot pass by not looking.
func findMisnamedHandlers(files []sourceFile) []violation {
	var violations []violation
	for _, file := range files {
		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, file.path, file.contents, parser.SkipObjectResolution)
		if err != nil {
			violations = append(violations, violation{where: file.path, detail: "does not parse: " + err.Error()})
			continue
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if !isCall || !isHumaRegister(call) {
				return true
			}
			where := fileSet.Position(call.Pos()).String()
			if problem := handlerNameProblem(call); problem != "" {
				violations = append(violations, violation{where: where, detail: problem})
			}
			return true
		})
	}
	return violations
}

// isHumaRegister reports whether call is huma.Register(api, operation, handler).
func isHumaRegister(call *ast.CallExpr) bool {
	selector, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector || selector.Sel.Name != "Register" {
		return false
	}
	packageName, isIdent := selector.X.(*ast.Ident)
	return isIdent && packageName.Name == "huma" && len(call.Args) == 3
}

// handlerNameProblem says what is wrong with one registration's names, or "" when its handler
// is the method its OperationID names.
func handlerNameProblem(call *ast.CallExpr) string {
	operation, isLiteral := call.Args[1].(*ast.CompositeLit)
	if !isLiteral {
		return "registers an operation this rule cannot read: not a huma.Operation literal"
	}
	operationID := ""
	for _, element := range operation.Elts {
		field, isField := element.(*ast.KeyValueExpr)
		if key, isIdent := field.Key.(*ast.Ident); !isField || !isIdent || key.Name != "OperationID" {
			continue
		}
		text, isString := field.Value.(*ast.BasicLit)
		if !isString || text.Kind != token.STRING {
			return "names its OperationID by an expression this rule cannot read"
		}
		unquoted, err := strconv.Unquote(text.Value)
		if err != nil {
			return "names its OperationID by a string this rule cannot read"
		}
		operationID = unquoted
	}
	if operationID == "" {
		return "registers an operation with no OperationID"
	}
	handler, isSelector := call.Args[2].(*ast.SelectorExpr)
	if !isSelector {
		return "registers operation " + operationID + " with a handler that is not a method value"
	}
	if handler.Sel.Name != operationID {
		return "registers operation " + operationID + " with the handler " + handler.Sel.Name
	}
	return ""
}

// TestArch_APIHandlersAreNamedForTheirOperations checks internal/core/api: each handler is the
// method its operation's OperationID names, so the API document and the code say one name.
func TestArch_APIHandlersAreNamedForTheirOperations(t *testing.T) {
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
	reportViolations(t, findMisnamedHandlers(readSourceFiles(t, api)))
}
