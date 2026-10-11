// SPDX-License-Identifier: AGPL-3.0-or-later

package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// findBareReceives returns every receive from a channel in a test file that can wait forever: one
// that is not a case of a select, the one place a receive has company that can end the wait (a
// timer, a context), and is not a wait for a context to end (<-ctx.Done()), which ends with the
// test. A test waits on a channel through testutil.Receive, which fails naming what it waited for;
// a goroutine the test started selects on its context (audit-2026-09-30 §5.3).
func findBareReceives(files []sourceFile) []violation {
	var violations []violation
	for _, file := range files {
		if !strings.HasSuffix(file.path, "_test.go") {
			continue
		}
		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, file.path, file.contents, parser.SkipObjectResolution)
		if err != nil {
			violations = append(violations, violation{where: file.path, detail: "does not parse: " + err.Error()})
			continue
		}
		inSelect := map[ast.Expr]bool{}
		ast.Inspect(parsed, func(node ast.Node) bool {
			if clause, isClause := node.(*ast.CommClause); isClause && clause.Comm != nil {
				if receive := receiveOf(clause.Comm); receive != nil {
					inSelect[receive] = true
				}
			}
			return true
		})
		ast.Inspect(parsed, func(node ast.Node) bool {
			unary, isUnary := node.(*ast.UnaryExpr)
			if isUnary && unary.Op == token.ARROW && !inSelect[unary] && !isContextEnding(unary.X) {
				violations = append(violations, violation{
					where: fileSet.Position(unary.Pos()).String(),
					detail: "receives from a channel with no bound; use testutil.Receive, " +
						"or a select on the context in a goroutine the test started",
				})
			}
			return true
		})
	}
	return violations
}

// isContextEnding is whether a receive's channel is ctx.Done(): this codebase names a context ctx.
func isContextEnding(channel ast.Expr) bool {
	call, isCall := channel.(*ast.CallExpr)
	if !isCall || len(call.Args) != 0 {
		return false
	}
	selector, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector || selector.Sel.Name != "Done" {
		return false
	}
	owner, isIdent := selector.X.(*ast.Ident)
	return isIdent && owner.Name == "ctx"
}

// receiveOf is the receive a select case waits on: `case <-c:`, `case v := <-c:`, `case v, ok = <-c:`.
func receiveOf(comm ast.Stmt) ast.Expr {
	var expression ast.Expr
	switch typed := comm.(type) {
	case *ast.ExprStmt:
		expression = typed.X
	case *ast.AssignStmt:
		if len(typed.Rhs) == 1 {
			expression = typed.Rhs[0]
		}
	}
	if unary, isUnary := expression.(*ast.UnaryExpr); isUnary && unary.Op == token.ARROW {
		return unary
	}
	return nil
}

// TestArch_EveryTestWaitOnAChannelIsBounded checks every test file of the module: a receive from
// a channel is a select's case or goes through testutil.Receive, so a wait that never ends fails
// in a minute saying what it waited for, rather than hanging the run.
func TestArch_EveryTestWaitOnAChannelIsBounded(t *testing.T) {
	t.Parallel()
	files := readSourceFiles(t, loadModulePackages(t))
	tests := 0
	for _, file := range files {
		if strings.HasSuffix(file.path, "_test.go") {
			tests++
		}
	}
	if tests == 0 {
		t.Fatal("found no test files; the rule would pass vacuously")
	}
	reportViolations(t, findBareReceives(files))
}

func TestFindBareReceives_CatchesEachUnboundedWaitAndPassesASelectsCases(t *testing.T) {
	t.Parallel()
	bounded := `package a
func a() {
	select {
	case <-c:
	case v := <-d:
		_ = v
	case v, ok := <-e:
		_, _ = v, ok
	case <-ctx.Done():
	}
	_ = testutil.Receive(t, c, "c")
	go func() { c <- 1 }()
	<-ctx.Done()
}`
	if got := findBareReceives([]sourceFile{{path: "a_test.go", contents: bounded}}); len(got) != 0 {
		t.Errorf("bounded waits: got %v, want none", got)
	}
	for _, body := range []string{
		`<-c`,
		`v := <-c; _ = v`,
		`if err := <-done; err != nil {}`,
		`defer func() { <-stopped }()`,
		`select { case v := <-c: _ = <-v }`,
		`go func() { <-release }()`,
		`<-child.Done()`,
	} {
		contents := "package a\nfunc a() { " + body + " }"
		if got := findBareReceives([]sourceFile{{path: "a_test.go", contents: contents}}); len(got) != 1 {
			t.Errorf("%s: got %v, want one violation", body, got)
		}
	}
	production := "package a\nfunc a() { <-c }"
	if got := findBareReceives([]sourceFile{{path: "a.go", contents: production}}); len(got) != 0 {
		t.Errorf("a receive outside a test: got %v, want none; this rule is the tests'", got)
	}
}
