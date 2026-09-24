// SPDX-License-Identifier: GPL-3.0-or-later

package archtest

import (
	"strings"
	"testing"
)

// Each rule is seen to fail on a graph that violates it, and to pass on one that does not
// (D190 §3). Without these, a rule that could never fire would look exactly like a clean tree.

func TestFindTransitiveDependencies_CatchesAnIndirectPath(t *testing.T) {
	t.Parallel()
	violating := []listedPackage{{
		ImportPath: corePath + "/library",
		Deps:       []string{"fmt", storePath, nodePath + "/scan"},
	}}
	if got := findTransitiveDependencies(violating, corePath, nodePath); len(got) != 1 {
		t.Errorf("core depending on %s/scan: got %d violations, want 1", nodePath, len(got))
	}
	execing := []listedPackage{{ImportPath: corePath + "/jobs", Deps: []string{"os/exec"}}}
	if got := findTransitiveDependencies(execing, corePath, "os/exec"); len(got) != 1 {
		t.Errorf("core depending on os/exec: got %d violations, want 1", len(got))
	}
	clean := []listedPackage{
		{ImportPath: corePath + "/library", Deps: []string{"fmt", nodeprotoPath}},
		{ImportPath: nodePath + "/scan", Deps: []string{"os/exec"}},
	}
	if got := findTransitiveDependencies(clean, corePath, nodePath); len(got) != 0 {
		t.Errorf("clean graph (nodeproto is not node): got %v, want none", got)
	}
}

func TestFindCoreRoutesToExec_ExemptsOnlyTheNamedEdge(t *testing.T) {
	t.Parallel()
	exempt := map[string]string{"modernc.org/libc": "a reason"}
	throughTheDriver := map[string][]string{
		corePath + "/auth":    {storePath},
		storePath:             {"modernc.org/sqlite"},
		"modernc.org/sqlite":  {"modernc.org/libc"},
		"modernc.org/libc":    {"os/exec", "os"},
		nodePath + "/execx":   {"os/exec"},
		corePath + "/library": {"fmt"},
	}
	if got := findCoreRoutesToExec(throughTheDriver, exempt); len(got) != 0 {
		t.Errorf("core reaching os/exec only through the exempt edge: got %v, want none", got)
	}
	if got := findCoreRoutesToExec(throughTheDriver, nil); len(got) != 1 ||
		!strings.Contains(got[0].detail, "modernc.org/libc -> os/exec") {
		t.Errorf("without the exemption: got %v, want the route through libc", got)
	}
	anotherRoute := map[string][]string{
		corePath + "/auth":       {storePath, "example.com/shells"},
		storePath:                {"modernc.org/libc"},
		"modernc.org/libc":       {"os/exec"},
		"example.com/shells":     {"example.com/shells/run"},
		"example.com/shells/run": {"os/exec"},
	}
	got := findCoreRoutesToExec(anotherRoute, exempt)
	if len(got) != 1 || !strings.Contains(got[0].detail, "example.com/shells/run -> os/exec") {
		t.Errorf("a second route beside the exempt one: got %v, want it named", got)
	}
	direct := map[string][]string{corePath + "/jobs": {"os/exec"}, "modernc.org/libc": {"os/exec"}}
	if got := findCoreRoutesToExec(direct, exempt); len(got) != 1 {
		t.Errorf("core importing os/exec itself: got %v, want it caught", got)
	}
}

func TestFindDirectImporters_JudgesProductionAndTestImportsSeparately(t *testing.T) {
	t.Parallel()
	rule := importRule{
		forbidden:            []string{"os/exec"},
		allowedImporters:     []string{execxPath},
		allowedTestImporters: []string{execxPath, archtestPath},
	}
	violating := []listedPackage{
		{ImportPath: corePath + "/jobs", Imports: []string{"os/exec"}},
		{ImportPath: nodePath + "/probe", TestImports: []string{"os/exec"}},
		{ImportPath: archtestPath, Imports: []string{"os/exec"}},
	}
	if got := findDirectImporters(violating, rule); len(got) != 3 {
		t.Errorf("got %d violations, want 3 (core, a node test, archtest production code): %v", len(got), got)
	}
	clean := []listedPackage{
		{ImportPath: execxPath, Imports: []string{"os/exec"}},
		{ImportPath: archtestPath, XTestImports: []string{"os/exec"}},
		{ImportPath: nodePath + "/probe", Imports: []string{execxPath}},
	}
	if got := findDirectImporters(clean, rule); len(got) != 0 {
		t.Errorf("clean graph: got %v, want none", got)
	}
}

func TestFindDirectImporters_TestutilIsForTestsOnly(t *testing.T) {
	t.Parallel()
	rule := importRule{
		forbidden:            []string{testutilPath},
		allowedImporters:     []string{testutilPath},
		allowedTestImporters: []string{modulePath},
	}
	shipping := []listedPackage{{ImportPath: nodePath + "/probe", Imports: []string{testutilPath}}}
	if got := findDirectImporters(shipping, rule); len(got) != 1 {
		t.Errorf("a shipping package importing testutil: got %d violations, want 1", len(got))
	}
	tests := []listedPackage{
		{ImportPath: nodePath + "/probe", TestImports: []string{testutilPath}},
		{ImportPath: corePath + "/api", XTestImports: []string{testutilPath}},
	}
	if got := findDirectImporters(tests, rule); len(got) != 0 {
		t.Errorf("tests importing testutil: got %v, want none", got)
	}
}

func TestFindDirectImporters_CatchesADriverOutsideTheStore(t *testing.T) {
	t.Parallel()
	rule := importRule{
		forbidden:            sqlPackages,
		allowedImporters:     []string{storePath},
		allowedTestImporters: []string{storePath},
	}
	violating := []listedPackage{{ImportPath: corePath + "/auth", Imports: []string{"modernc.org/sqlite/lib"}}}
	if got := findDirectImporters(violating, rule); len(got) != 1 {
		t.Errorf("a driver subpackage imported by core: got %d violations, want 1", len(got))
	}
	clean := []listedPackage{{ImportPath: storePath, Imports: []string{"database/sql", "modernc.org/sqlite"}}}
	if got := findDirectImporters(clean, rule); len(got) != 0 {
		t.Errorf("the store importing the driver: got %v, want none", got)
	}
}

func TestFindNodeprotoInternalImports(t *testing.T) {
	t.Parallel()
	violating := []listedPackage{{ImportPath: nodeprotoPath, Imports: []string{"encoding/json", storePath}}}
	if got := findNodeprotoInternalImports(violating); len(got) != 1 {
		t.Errorf("nodeproto importing the store: got %d violations, want 1", len(got))
	}
	clean := []listedPackage{{ImportPath: nodeprotoPath, Imports: []string{"encoding/json", platformPath + "/clock"}}}
	if got := findNodeprotoInternalImports(clean); len(got) != 0 {
		t.Errorf("nodeproto importing platform: got %v, want none", got)
	}
}

func TestFindFreeFunctions(t *testing.T) {
	t.Parallel()
	violating := []sourceFile{{path: "report.go", contents: "package nodeproto\n\nfunc Decide() {}\n"}}
	if got := findFreeFunctions(violating); len(got) != 1 {
		t.Errorf("a free function: got %d violations, want 1", len(got))
	}
	broken := []sourceFile{{path: "broken.go", contents: "package nodeproto\n\nfunc {"}}
	if got := findFreeFunctions(broken); len(got) != 1 {
		t.Errorf("an unparseable file: got %d violations, want 1", len(got))
	}
	methodOnly := "package nodeproto\n\ntype Kind string\n\nfunc (kind Kind) String() string { return string(kind) }\n"
	clean := []sourceFile{{path: "report.go", contents: methodOnly}}
	if got := findFreeFunctions(clean); len(got) != 0 {
		t.Errorf("a method only: got %v, want none", got)
	}
}

func TestCheckDocFile(t *testing.T) {
	t.Parallel()
	complete := strings.Join([]string{
		"// Package x does one thing.", "//", "// # Invariants", "//",
		"// # Read before changing", "//", "// # Where to change", "package x", "",
	}, "\n")
	reversed := strings.Join([]string{
		"// Package x does one thing.", "//", "// # Where to change", "//",
		"// # Read before changing", "//", "// # Invariants", "package x", "",
	}, "\n")
	withoutWhereToChange := strings.Replace(complete, "# Where to change", "Where to change", 1)
	cases := []struct {
		name           string
		source         string
		found          bool
		wantViolations int
	}{
		{name: "complete", source: complete, found: true, wantViolations: 0},
		{name: "missing file", found: false, wantViolations: 1},
		{name: "no package comment", source: "package x\n", found: true, wantViolations: 1},
		{name: "missing a heading", source: withoutWhereToChange, found: true, wantViolations: 1},
		{name: "headings out of order", source: reversed, found: true, wantViolations: 2},
		{name: "does not parse", source: "package", found: true, wantViolations: 1},
	}
	for _, tc := range cases {
		if got := checkDocFile("example/x", tc.source, tc.found); len(got) != tc.wantViolations {
			t.Errorf("%s: got %d violations %v, want %d", tc.name, len(got), got, tc.wantViolations)
		}
	}
}

func TestFindGenericPackageNames(t *testing.T) {
	t.Parallel()
	violating := []listedPackage{
		{ImportPath: corePath + "/util", Name: "util"},
		{ImportPath: nodePath + "/scan/helpers", Name: "helpers"},
	}
	if got := findGenericPackageNames(violating); len(got) != 2 {
		t.Errorf("got %d violations, want 2", len(got))
	}
	clean := []listedPackage{{ImportPath: corePath + "/library", Name: "library"}}
	if got := findGenericPackageNames(clean); len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}

func TestFindForbiddenSuppressions(t *testing.T) {
	t.Parallel()
	// Built by concatenation so this file does not itself contain the directives it tests.
	directive := "//" + "nolint:"
	violating := []sourceFile{{path: "a.go", contents: strings.Join([]string{
		"func long() { " + directive + "funlen // it is long",
		"x := 1 " + directive + "gosec,cyclop // two linters",
		"import _ \"x\" " + directive + "depguard // an exception",
	}, "\n")}}
	if got := findForbiddenSuppressions(violating); len(got) != 3 {
		t.Errorf("got %d violations %v, want 3", len(got), got)
	}
	explained := "var v = 1 " + directive + "gochecknoglobals // the linker sets it\n"
	clean := []sourceFile{{path: "a.go", contents: explained}}
	if got := findForbiddenSuppressions(clean); len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}

func TestFindStarSelections(t *testing.T) {
	t.Parallel()
	violating := []sourceFile{
		{path: "a.sql", contents: "-- name: ListTitles :many\nSELECT * FROM live_title;\n"},
		{path: "b.sql", contents: "-- name: ListFiles :many\nselect f.* from file f;\n"},
		{path: "c.sql", contents: "-- name: AddJob :one\nINSERT INTO job (kind) VALUES (?) RETURNING *;\n"},
	}
	if got := findStarSelections(violating); len(got) != 3 {
		t.Errorf("got %d violations %v, want 3", len(got), got)
	}
	clean := []sourceFile{{path: "d.sql", contents: strings.Join([]string{
		"-- name: CountTitles :one",
		"-- a comment may say SELECT * without harm",
		"SELECT count(*) FROM live_title;",
		"-- name: ListTitles :many",
		"SELECT id, name FROM live_title;",
	}, "\n")}}
	if got := findStarSelections(clean); len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}
