// SPDX-License-Identifier: AGPL-3.0-or-later

package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"testing"
)

// cmdPath is the binary; forbidigo's [ambient] rule exempts it, so this rule watches it instead.
const cmdPath = modulePath + "/cmd/orpheum"

// environmentReaders are the functions that read the process environment, by import path.
var environmentReaders = map[string][]string{
	"os":      {"Getenv", "LookupEnv", "Environ", "ExpandEnv"},
	"syscall": {"Getenv", "Environ"},
}

// findEnvironmentReads returns every reference, called or passed as a value, to a function that
// reads the process environment, however its package was imported.
func findEnvironmentReads(files []sourceFile) []violation {
	var violations []violation
	for _, file := range files {
		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, file.path, file.contents, parser.SkipObjectResolution)
		if err != nil {
			violations = append(violations, violation{where: file.path, detail: "does not parse: " + err.Error()})
			continue
		}
		localNames := watchedImports(parsed)
		ast.Inspect(parsed, func(node ast.Node) bool {
			if call, isCall := node.(*ast.CallExpr); isCall && isTimeZoneRead(call, localNames) {
				return false
			}
			if read, found := environmentRead(node, localNames); found {
				violations = append(violations, violation{
					where:  fileSet.Position(node.Pos()).String(),
					detail: "reads the environment with " + read,
				})
			}
			return true
		})
	}
	return violations
}

// isTimeZoneRead is whether call reads the machine's time zone and nothing else, the one variable a
// release binary may read, as the default of the server's time zone (D246 §7):
// os.LookupEnv(timeZoneVariable), or os.LookupEnv("TZ").
func isTimeZoneRead(call *ast.CallExpr, localNames map[string]string) bool {
	selector, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector || selector.Sel.Name != "LookupEnv" || len(call.Args) != 1 {
		return false
	}
	pkg, isIdent := selector.X.(*ast.Ident)
	if !isIdent || localNames[pkg.Name] != "os" {
		return false
	}
	switch arg := call.Args[0].(type) {
	case *ast.Ident:
		return arg.Name == "timeZoneVariable"
	case *ast.BasicLit:
		return arg.Value == `"TZ"`
	}
	return false
}

// watchedImports maps the local name of each imported package that can read the environment to
// its import path.
func watchedImports(parsed *ast.File) map[string]string {
	localNames := map[string]string{}
	for _, imported := range parsed.Imports {
		importPath, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			continue // the parser accepted it, so it is a quoted string
		}
		if _, watched := environmentReaders[importPath]; !watched {
			continue
		}
		localName := importPath
		if imported.Name != nil {
			localName = imported.Name.Name
		}
		localNames[localName] = importPath
	}
	return localNames
}

// environmentRead reports whether node names an environment-reading function, and which.
func environmentRead(node ast.Node, localNames map[string]string) (string, bool) {
	selector, isSelector := node.(*ast.SelectorExpr)
	if !isSelector {
		return "", false
	}
	packageName, isIdent := selector.X.(*ast.Ident)
	if !isIdent {
		return "", false
	}
	importPath, watched := localNames[packageName.Name]
	if !watched || !slices.Contains(environmentReaders[importPath], selector.Sel.Name) {
		return "", false
	}
	return importPath + "." + selector.Sel.Name, true
}

// TestArch_ReleaseBinaryReadsNoEnvironment checks the files a release build compiles into
// cmd/orpheum (no dev tag, no tests): none reads the environment, so the shipped binary is
// configured by its config file, flags and database alone (D204).
func TestArch_ReleaseBinaryReadsNoEnvironment(t *testing.T) {
	t.Parallel()
	var release []listedPackage
	for _, listed := range loadModulePackages(t) {
		if listed.ImportPath == cmdPath {
			release = append(release, listedPackage{Dir: listed.Dir, GoFiles: listed.GoFiles})
		}
	}
	if len(release) == 0 || len(release[0].GoFiles) == 0 {
		t.Fatalf("go list found no release files for %s; the rule would pass vacuously", cmdPath)
	}
	reportViolations(t, findEnvironmentReads(readSourceFiles(t, release)))
}

func TestFindEnvironmentReads(t *testing.T) {
	t.Parallel()
	renamed := "package main\n\nimport o \"os\"\n\nvar lookup = o.LookupEnv\n"
	called := "package main\n\nimport \"syscall\"\n\nfunc f() { _, _ = syscall.Getenv(\"X\") }\n"
	for name, contents := range map[string]string{"renamed and passed": renamed, "called": called} {
		if got := findEnvironmentReads([]sourceFile{{path: "main.go", contents: contents}}); len(got) != 1 {
			t.Errorf("%s: got %d violations, want 1", name, len(got))
		}
	}
	other := "package main\n\nimport \"os\"\n\nfunc f() { _, _ = os.LookupEnv(\"HOME\") }\n"
	if got := findEnvironmentReads([]sourceFile{{path: "main.go", contents: other}}); len(got) != 1 {
		t.Errorf("a variable other than TZ: got %d violations, want 1", len(got))
	}
	timeZone := "package main\n\nimport \"os\"\n\nfunc f() { _, _ = os.LookupEnv(timeZoneVariable) }\n"
	if got := findEnvironmentReads([]sourceFile{{path: "main.go", contents: timeZone}}); len(got) != 0 {
		t.Errorf("the time zone's default (D246 §7): got %v, want none", got)
	}
	clean := "package main\n\nimport \"os\"\n\nfunc f() { _ = os.Args; os.Exit(0) }\n"
	if got := findEnvironmentReads([]sourceFile{{path: "main.go", contents: clean}}); len(got) != 0 {
		t.Errorf("argv and exit: got %v, want none", got)
	}
	broken := []sourceFile{{path: "broken.go", contents: "package main\n\nfunc {"}}
	if got := findEnvironmentReads(broken); len(got) != 1 {
		t.Errorf("an unparseable file: got %d violations, want 1", len(got))
	}
}
