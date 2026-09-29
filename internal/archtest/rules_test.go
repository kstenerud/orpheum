// SPDX-License-Identifier: GPL-3.0-or-later

package archtest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The package roots the rules name.
const (
	corePath      = modulePath + "/internal/core"
	nodePath      = modulePath + "/internal/node"
	nodeprotoPath = modulePath + "/internal/nodeproto"
	storePath     = modulePath + "/internal/store"
	execxPath     = modulePath + "/internal/node/execx"
	platformPath  = modulePath + "/internal/platform"
	archtestPath  = modulePath + "/internal/archtest"
	testutilPath  = modulePath + "/internal/testutil"
)

// sqlPackages are database/sql and every SQLite driver; only the store may import them.
var sqlPackages = []string{
	"database/sql",
	"modernc.org/sqlite",
	"github.com/ncruces/go-sqlite3",
	"github.com/mattn/go-sqlite3",
	"zombiezen.com/go/sqlite",
	"crawshaw.io/sqlite",
	"github.com/glebarez/sqlite",
	"github.com/glebarez/go-sqlite",
}

// violation is one broken rule: which package or file, and what it did.
type violation struct {
	where  string
	detail string
}

// isUnder reports whether importPath is root or a package beneath it.
func isUnder(importPath, root string) bool {
	return importPath == root || strings.HasPrefix(importPath, root+"/")
}

func isUnderAny(importPath string, roots []string) bool {
	return slices.ContainsFunc(roots, func(root string) bool { return isUnder(importPath, root) })
}

// findTransitiveDependencies returns every package under fromRoot whose full dependency
// closure (go list's Deps) contains a package under targetRoot.
func findTransitiveDependencies(packages []listedPackage, fromRoot, targetRoot string) []violation {
	var violations []violation
	for _, listed := range packages {
		if !isUnder(listed.ImportPath, fromRoot) {
			continue
		}
		for _, dependency := range listed.Deps {
			if isUnder(dependency, targetRoot) {
				violations = append(violations, violation{
					where:  listed.ImportPath,
					detail: fmt.Sprintf("depends (directly or transitively) on %s", dependency),
				})
			}
		}
	}
	return violations
}

// execExemptions are the only packages through which the core may reach os/exec, each with the
// reason (D199). The check drops just that edge: any other route to os/exec still fails.
var execExemptions = map[string]string{
	"modernc.org/libc": "the pure-Go SQLite driver's C runtime implements system() and popen() with " +
		"os/exec; SQLite never calls them",
}

// findCoreRoutesToExec returns, for every core package, one import route to os/exec that does
// not pass through an exempt importer's edge to it.
func findCoreRoutesToExec(graph map[string][]string, exempt map[string]string) []violation {
	const target = "os/exec"
	var violations []violation
	for importPath := range graph {
		if !isUnder(importPath, corePath) {
			continue
		}
		if route := routeTo(graph, importPath, target, exempt); route != nil {
			violations = append(violations, violation{
				where:  importPath,
				detail: "reaches " + target + " through " + strings.Join(route, " -> "),
			})
		}
	}
	slices.SortFunc(violations, func(first, second violation) int { return strings.Compare(first.where, second.where) })
	return violations
}

// routeTo finds an import route from start to target, breadth first, skipping exempt edges.
func routeTo(graph map[string][]string, start, target string, exempt map[string]string) []string {
	previous := map[string]string{start: ""}
	queue := []string{start}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, imported := range graph[current] {
			if _, seen := previous[imported]; seen {
				continue
			}
			if _, isExempt := exempt[current]; isExempt && imported == target {
				continue
			}
			previous[imported] = current
			if imported == target {
				var route []string
				for step := target; step != ""; step = previous[step] {
					route = append([]string{step}, route...)
				}
				return route
			}
			queue = append(queue, imported)
		}
	}
	return nil
}

// importRule says which packages may import a set of forbidden packages directly.
type importRule struct {
	forbidden            []string
	allowedImporters     []string
	allowedTestImporters []string
}

// findDirectImporters returns every direct import of a forbidden package by a package the rule
// does not allow. Test imports are judged against allowedTestImporters.
func findDirectImporters(packages []listedPackage, rule importRule) []violation {
	var violations []violation
	for _, listed := range packages {
		if !isUnderAny(listed.ImportPath, rule.allowedImporters) {
			for _, imported := range listed.Imports {
				if isUnderAny(imported, rule.forbidden) {
					violations = append(violations, violation{where: listed.ImportPath, detail: "imports " + imported})
				}
			}
		}
		if isUnderAny(listed.ImportPath, rule.allowedTestImporters) {
			continue
		}
		for _, imported := range slices.Concat(listed.TestImports, listed.XTestImports) {
			if isUnderAny(imported, rule.forbidden) {
				violations = append(violations, violation{
					where:  listed.ImportPath,
					detail: "a test imports " + imported,
				})
			}
		}
	}
	return violations
}

// findNodeprotoInternalImports returns imports by internal/nodeproto of any module package
// outside internal/platform.
func findNodeprotoInternalImports(packages []listedPackage) []violation {
	var violations []violation
	for _, listed := range packages {
		if !isUnder(listed.ImportPath, nodeprotoPath) {
			continue
		}
		for _, imported := range listed.Imports {
			if isUnder(imported, modulePath) && !isUnder(imported, platformPath) {
				violations = append(violations, violation{where: listed.ImportPath, detail: "imports " + imported})
			}
		}
	}
	return violations
}

// findFreeFunctions returns every top-level function that is not a method. Wire types may
// have methods (a String, a validation); a free function is logic and belongs elsewhere.
func findFreeFunctions(files []sourceFile) []violation {
	var violations []violation
	for _, file := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), file.path, file.contents, parser.SkipObjectResolution)
		if err != nil {
			violations = append(violations, violation{where: file.path, detail: "does not parse: " + err.Error()})
			continue
		}
		for _, declaration := range parsed.Decls {
			function, isFunction := declaration.(*ast.FuncDecl)
			if isFunction && function.Recv == nil {
				violations = append(violations, violation{
					where:  file.path,
					detail: "declares func " + function.Name.Name,
				})
			}
		}
	}
	return violations
}

// generatedPackages hold only a generator's output, where no hand-written doc.go may live (the
// pre-edit hook refuses it); the package that owns the generator documents them instead.
var generatedPackages = []string{storePath + "/sqlcgen", storePath + "/cachegen"}

// requiredDocHeadings are the doc.go headings every package carries, in this order.
var requiredDocHeadings = []string{"Invariants", "Read before changing", "Where to change"}

// checkDocFile returns what is wrong with a package's doc.go: missing, no package comment, or
// a required heading absent or out of order.
func checkDocFile(importPath, source string, hasDocFile bool) []violation {
	if !hasDocFile {
		return []violation{{where: importPath, detail: "has no doc.go"}}
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), "doc.go", source, parser.ParseComments|parser.PackageClauseOnly)
	if err != nil {
		return []violation{{where: importPath, detail: "doc.go does not parse: " + err.Error()}}
	}
	if parsed.Doc == nil {
		return []violation{{where: importPath, detail: "doc.go has no package comment"}}
	}
	var headings []string
	for line := range strings.SplitSeq(parsed.Doc.Text(), "\n") {
		if heading, isHeading := strings.CutPrefix(line, "# "); isHeading {
			headings = append(headings, heading)
		}
	}
	var violations []violation
	next := 0
	for _, required := range requiredDocHeadings {
		position := slices.Index(headings[next:], required)
		if position < 0 {
			violations = append(violations, violation{
				where:  importPath,
				detail: fmt.Sprintf("doc.go lacks the heading %q, or has it out of order", required),
			})
			continue
		}
		next += position + 1
	}
	return violations
}

// genericPackageNames say nothing about what a package is for.
var genericPackageNames = []string{
	"util", "utils", "common", "helper", "helpers", "misc", "shared", "lib", "base", "stuff", "types",
}

// findGenericPackageNames returns every package whose name or directory is a generic name.
func findGenericPackageNames(packages []listedPackage) []violation {
	var violations []violation
	for _, listed := range packages {
		for _, name := range []string{listed.Name, path.Base(listed.ImportPath)} {
			if slices.Contains(genericPackageNames, strings.ToLower(name)) {
				violations = append(violations, violation{
					where:  listed.ImportPath,
					detail: fmt.Sprintf("is named %q, which does not say what it is for", name),
				})
				break
			}
		}
	}
	return violations
}

// unsuppressibleLinters are never silenced by //nolint: complexity is fixed by extracting
// named steps, and an architecture exception is an owner's change to .golangci.yml.
var unsuppressibleLinters = []string{
	"cyclop", "gocognit", "funlen", "nestif", "gocyclo", "maintidx", "depguard", "all",
}

var nolintDirective = regexp.MustCompile(`//\s*nolint:([\w,\s-]+)`)

// findForbiddenSuppressions returns every //nolint directive that names an unsuppressible linter.
func findForbiddenSuppressions(files []sourceFile) []violation {
	var violations []violation
	for _, file := range files {
		for lineIndex, line := range strings.Split(file.contents, "\n") {
			for _, match := range nolintDirective.FindAllStringSubmatch(line, -1) {
				for linter := range strings.SplitSeq(match[1], ",") {
					linter = strings.TrimSpace(strings.Fields(linter + " ")[0])
					if slices.Contains(unsuppressibleLinters, linter) {
						violations = append(violations, violation{
							where:  fmt.Sprintf("%s:%d", file.path, lineIndex+1),
							detail: "suppresses " + linter + ", which is never suppressed",
						})
					}
				}
			}
		}
	}
	return violations
}

// reportViolations fails the test once per violation.
func reportViolations(t *testing.T, violations []violation) {
	t.Helper()
	for _, found := range violations {
		t.Errorf("%s %s", found.where, found.detail)
	}
}

// requirePackagesUnder fails when no listed package is under root, so a rule about root cannot
// pass because there was nothing to check.
func requirePackagesUnder(t *testing.T, packages []listedPackage, root string) {
	t.Helper()
	if !slices.ContainsFunc(packages, func(listed listedPackage) bool { return isUnder(listed.ImportPath, root) }) {
		t.Fatalf("go list found no package under %s; the rule would pass vacuously", root)
	}
}

func TestArch_CoreNeverImportsNode(t *testing.T) {
	t.Parallel()
	packages := loadModulePackages(t)
	requirePackagesUnder(t, packages, corePath)
	reportViolations(t, findTransitiveDependencies(packages, corePath, nodePath))
}

func TestArch_CoreNeverExecs(t *testing.T) {
	t.Parallel()
	packages := loadModulePackages(t)
	requirePackagesUnder(t, packages, corePath)
	reportViolations(t, findCoreRoutesToExec(loadCoreImportGraph(t), execExemptions))
}

func TestArch_NodeNeverImportsCore(t *testing.T) {
	t.Parallel()
	packages := loadModulePackages(t)
	requirePackagesUnder(t, packages, nodePath)
	reportViolations(t, findTransitiveDependencies(packages, nodePath, corePath))
}

func TestArch_OnlyStoreImportsDatabaseSQL(t *testing.T) {
	t.Parallel()
	packages := loadModulePackages(t)
	requirePackagesUnder(t, packages, storePath)
	reportViolations(t, findDirectImporters(packages, importRule{
		forbidden:            sqlPackages,
		allowedImporters:     []string{storePath},
		allowedTestImporters: []string{storePath},
	}))
}

// TestArch_OnlyExecxImportsOsExec checks that, of the code that ships, only execx starts a
// process. The test-only package internal/testutil may too, to run the tools the tests need
// (MKVToolNix, FFmpeg, this binary); that it never ships is TestArch_TestSupportIsImportedOnlyByTests.
func TestArch_OnlyExecxImportsOsExec(t *testing.T) {
	t.Parallel()
	packages := loadModulePackages(t)
	requirePackagesUnder(t, packages, execxPath)
	reportViolations(t, findDirectImporters(packages, importRule{
		forbidden:            []string{"os/exec"},
		allowedImporters:     []string{execxPath, testutilPath},
		allowedTestImporters: []string{execxPath, archtestPath},
	}))
}

// testSupportPackages are the packages that exist for tests: internal/testutil and every package
// named for a test's use (storetest, clocktest, settingstest, authtest…), which may import one
// another.
func testSupportPackages(packages []listedPackage) []string {
	support := []string{testutilPath}
	for _, listed := range packages {
		name := listed.ImportPath[strings.LastIndex(listed.ImportPath, "/")+1:]
		if strings.HasSuffix(name, "test") && listed.ImportPath != modulePath+"/internal/archtest" {
			support = append(support, listed.ImportPath)
		}
	}
	return support
}

// TestArch_TestSupportIsImportedOnlyByTests checks that no package that ships imports a
// test-support package, so what one may do — skip a test, read test variables, run MKVToolNix,
// fake a service — never reaches a binary.
func TestArch_TestSupportIsImportedOnlyByTests(t *testing.T) {
	t.Parallel()
	packages := loadModulePackages(t)
	requirePackagesUnder(t, packages, testutilPath)
	support := testSupportPackages(packages)
	if len(support) < 2 {
		t.Fatalf("found no test-support package beside testutil (%v); the rule would check too little", support)
	}
	reportViolations(t, findDirectImporters(packages, importRule{
		forbidden:            support,
		allowedImporters:     support,
		allowedTestImporters: []string{modulePath},
	}))
}

func TestArch_NodeprotoHasNoLogic(t *testing.T) {
	t.Parallel()
	packages := loadModulePackages(t)
	requirePackagesUnder(t, packages, nodeprotoPath)
	reportViolations(t, findNodeprotoInternalImports(packages))
	var productionFiles []sourceFile
	for _, file := range readSourceFiles(t, packages) {
		if strings.Contains(file.path, "/internal/nodeproto/") && !strings.HasSuffix(file.path, "_test.go") {
			productionFiles = append(productionFiles, file)
		}
	}
	reportViolations(t, findFreeFunctions(productionFiles))
}

func TestArch_EveryPackageHasDocGo(t *testing.T) {
	t.Parallel()
	packages := loadModulePackages(t)
	for _, listed := range packages {
		if slices.Contains(generatedPackages, listed.ImportPath) {
			continue
		}
		source, found := readDocFile(t, listed)
		reportViolations(t, checkDocFile(listed.ImportPath, source, found))
	}
}

func TestArch_NoGenericPackageNames(t *testing.T) {
	t.Parallel()
	reportViolations(t, findGenericPackageNames(loadModulePackages(t)))
}

func TestArch_NoForbiddenSuppressions(t *testing.T) {
	t.Parallel()
	files := readSourceFiles(t, loadModulePackages(t))
	if len(files) == 0 {
		t.Fatal("found no Go files; the rule would pass vacuously")
	}
	reportViolations(t, findForbiddenSuppressions(files))
}

// starSelection matches SELECT * and RETURNING *, plain or qualified (t.*). COUNT(*) does not
// match, because the star is not the whole column list.
var starSelection = regexp.MustCompile(`(?is)\b(select|returning)\s+(distinct\s+)?([a-z_][a-z0-9_]*\.)?\*`)

var sqlLineComment = regexp.MustCompile(`--[^\n]*`)

// findStarSelections returns every query file that selects or returns * instead of naming
// its columns. The source is read because sqlc expands * before `sqlc vet` sees a query.
func findStarSelections(files []sourceFile) []violation {
	var violations []violation
	for _, file := range files {
		if starSelection.MatchString(sqlLineComment.ReplaceAllString(file.contents, "")) {
			violations = append(violations, violation{
				where:  file.path,
				detail: "selects or returns *; name every column",
			})
		}
	}
	return violations
}

func TestArch_QueriesNameEveryColumn(t *testing.T) {
	t.Parallel()
	var storeDirectory string
	for _, listed := range loadModulePackages(t) {
		if listed.ImportPath == storePath {
			storeDirectory = listed.Dir
		}
	}
	if storeDirectory == "" {
		t.Fatalf("go list did not find %s", storePath)
	}
	files := readQueryFiles(t, filepath.Join(storeDirectory, "queries"))
	files = append(files, readQueryFiles(t, filepath.Join(storeDirectory, "cache", "queries"))...)
	reportViolations(t, findStarSelections(files))
}
