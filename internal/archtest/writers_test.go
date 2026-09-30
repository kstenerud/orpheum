// SPDX-License-Identifier: GPL-3.0-or-later

package archtest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The generated query packages whose methods write the tables.
const (
	sqlcgenPath  = storePath + "/sqlcgen"
	cachegenPath = storePath + "/cachegen"
)

// schemaPlanPath is schema.md, relative to the module's root: its "Who writes each table" section
// names each table's one writing package.
const schemaPlanPath = "docs/contributors/design/plans/schema.md"

// noWriterYet is how the owners table says that no package writes a table yet.
const noWriterYet = "none yet"

// queryHeader starts one sqlc query: "-- name: InsertFile :one".
var queryHeader = regexp.MustCompile(`(?m)^-- name: (\w+) :\w+\s*$`)

// writeStatement finds the table a statement writes, after an insert, replace, update or delete,
// with the table's name bare or quoted, as "grant" must be.
var writeStatement = regexp.MustCompile(`(?is)\b(?:insert(?:\s+or\s+\w+)?\s+into|replace\s+into|` +
	`update(?:\s+or\s+\w+)?|delete\s+from)\s+["'` + "`" + `]?(\w+)`)

// ownerRow is one row of schema.md's owners table: the tables, the database, the writing package.
var ownerRow = regexp.MustCompile("(?m)^\\| (`\\w+`(?:, `\\w+`)*) \\| (?:library|cache)\\.db \\| (`[^`]+`|" +
	noWriterYet + ") \\|")

// findWriteQueries maps each query that writes a table, as "<generated package>.<query name>", to
// the table it writes. Each file is one .sql file of queries generated into generatedPackage.
func findWriteQueries(files []sourceFile, generatedPackage string) map[string]string {
	tableByQuery := map[string]string{}
	for _, file := range files {
		headers := queryHeader.FindAllStringSubmatchIndex(file.contents, -1)
		for index, header := range headers {
			end := len(file.contents)
			if index+1 < len(headers) {
				end = headers[index+1][0]
			}
			statement := sqlLineComment.ReplaceAllString(file.contents[header[1]:end], "")
			if written := writeStatement.FindStringSubmatch(statement); written != nil {
				name := file.contents[header[2]:header[3]]
				tableByQuery[generatedPackage+"."+name] = written[1]
			}
		}
	}
	return tableByQuery
}

// readTableOwners reads schema.md's owners table: each table's one writing package, or "" when
// no package writes it yet.
func readTableOwners(schemaPlan string) map[string]string {
	ownerByTable := map[string]string{}
	for _, row := range ownerRow.FindAllStringSubmatch(schemaPlan, -1) {
		owner := strings.Trim(row[2], "`")
		if owner == noWriterYet {
			owner = ""
		}
		for table := range strings.SplitSeq(row[1], ", ") {
			ownerByTable[strings.Trim(table, "`")] = owner
		}
	}
	return ownerByTable
}

// findTablesWithoutOneWriter returns every table written by a package other than the one the
// owners table names, and every written table the owners table does not list. The first map is
// each package's calls to the generated queries, the second the table each write among them
// writes.
func findTablesWithoutOneWriter(
	queriesByPackage map[string][]string, tableByQuery, ownerByTable map[string]string,
) []violation {
	writersByTable := map[string]map[string]bool{}
	for importPath, queries := range queriesByPackage {
		for _, query := range queries {
			table, writes := tableByQuery[query]
			if !writes {
				continue
			}
			if writersByTable[table] == nil {
				writersByTable[table] = map[string]bool{}
			}
			writersByTable[table][strings.TrimPrefix(importPath, modulePath+"/")] = true
		}
	}
	var violations []violation
	for _, table := range slices.Sorted(maps.Keys(writersByTable)) {
		writers := slices.Sorted(maps.Keys(writersByTable[table]))
		owner, listed := ownerByTable[table]
		switch {
		case !listed:
			violations = append(violations, violation{where: table, detail: fmt.Sprintf(
				"is written by %s, but %s's owners table does not list it",
				strings.Join(writers, " and "), schemaPlanPath)})
		case len(writers) > 1:
			violations = append(violations, violation{where: table, detail: fmt.Sprintf(
				"is written by %s; one package writes a table, and the others ask it (%s names %q)",
				strings.Join(writers, " and "), schemaPlanPath, owner)})
		case writers[0] != owner:
			violations = append(violations, violation{where: table, detail: fmt.Sprintf(
				"is written by %s, but %s names %q as its writer",
				writers[0], schemaPlanPath, ownerOrNoneYet(owner))})
		}
	}
	return violations
}

// ownerOrNoneYet says "none yet" for an empty owner.
func ownerOrNoneYet(owner string) string {
	if owner == "" {
		return noWriterYet
	}
	return owner
}

// exportedPackage is the part of `go list -export -json` output the writer rule reads: where the
// compiler's export data for a package is. The keys are the go command's.
type exportedPackage struct {
	ImportPath string `json:"ImportPath"`
	Export     string `json:"Export"`
}

// loadGeneratedQueryCalls type-checks every shipped package of the module except the generated
// and test-support ones, and returns, per package, the generated query methods it names
// ("<generated package>.<method>"). Type-checking, rather than reading names, tells a call to
// sqlcgen's RenameDevice from a call to a service's method of the same name.
func loadGeneratedQueryCalls(t *testing.T, packages []listedPackage) map[string][]string {
	t.Helper()
	queriesByPackage := map[string][]string{}
	for importPath, functions := range loadFunctionsUsed(t, packages) {
		queriesByPackage[importPath] = []string{}
		for _, function := range functions {
			if path := function.Pkg().Path(); path == sqlcgenPath || path == cachegenPath {
				queriesByPackage[importPath] = append(queriesByPackage[importPath], path+"."+function.Name())
			}
		}
	}
	return queriesByPackage
}

// loadFunctionsUsed type-checks every shipped package of the module except the generated and
// test-support ones, and returns, per package, every function and method its shipped files name.
func loadFunctionsUsed(t *testing.T, packages []listedPackage) map[string][]*types.Func {
	t.Helper()
	exportFiles := loadExportFiles(t)
	fileSet := token.NewFileSet()
	imports := importer.ForCompiler(fileSet, "gc", func(importPath string) (io.ReadCloser, error) {
		exportFile, found := exportFiles[importPath]
		if !found || exportFile == "" {
			return nil, fmt.Errorf("no export data for %s", importPath)
		}
		return os.Open(exportFile)
	})
	skipped := append(append([]string{archtestPath}, generatedPackages...), testSupportPackages(packages)...)
	functionsByPackage := map[string][]*types.Func{}
	for _, listed := range packages {
		if slices.Contains(skipped, listed.ImportPath) || len(listed.GoFiles) == 0 {
			continue
		}
		functionsByPackage[listed.ImportPath] = functionsUsed(t, fileSet, imports, listed)
	}
	return functionsByPackage
}

// functionsUsed type-checks one package's shipped files and returns the functions and methods
// they name, each from a package.
func functionsUsed(
	t *testing.T, fileSet *token.FileSet, imports types.Importer, listed listedPackage,
) []*types.Func {
	t.Helper()
	var files []*ast.File
	for _, name := range listed.GoFiles {
		parsed, err := parser.ParseFile(fileSet, filepath.Join(listed.Dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files = append(files, parsed)
	}
	info := &types.Info{Uses: map[*ast.Ident]types.Object{}}
	config := types.Config{Importer: imports}
	if _, err := config.Check(listed.ImportPath, fileSet, files, info); err != nil {
		t.Fatalf("type-check %s: %v", listed.ImportPath, err)
	}
	var functions []*types.Func
	for _, used := range info.Uses {
		if function, isFunction := used.(*types.Func); isFunction && function.Pkg() != nil {
			functions = append(functions, function)
		}
	}
	return functions
}

// loadExportFiles runs `go list -export` over the module and its dependencies: each package's
// compiled export data, which the type checker imports.
func loadExportFiles(t *testing.T) map[string]string {
	t.Helper()
	command := exec.CommandContext(t.Context(), "go", "list", "-export", "-deps", "-json=ImportPath,Export",
		modulePath+"/cmd/...", modulePath+"/internal/...")
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list -export failed: %v\n%s", err, stderr.String())
	}
	exportFiles := map[string]string{}
	decoder := json.NewDecoder(bytes.NewReader(output))
	for {
		var listed exportedPackage
		err := decoder.Decode(&listed)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decode go list -export output: %v", err)
		}
		exportFiles[listed.ImportPath] = listed.Export
	}
	return exportFiles
}

// TestArch_EachTableHasOneWritingPackage checks that each library.db and cache.db table is
// written by one package, the one schema.md's owners table names: a package whose code calls a
// generated query that inserts into, updates or deletes from it. Any other package asks that one
// (architecture map F5).
func TestArch_EachTableHasOneWritingPackage(t *testing.T) {
	t.Parallel()
	packages := loadModulePackages(t)
	var storeDirectory string
	for _, listed := range packages {
		if listed.ImportPath == storePath {
			storeDirectory = listed.Dir
		}
	}
	if storeDirectory == "" {
		t.Fatalf("go list did not find %s", storePath)
	}
	tableByQuery := findWriteQueries(readQueryFiles(t, filepath.Join(storeDirectory, "queries")), sqlcgenPath)
	maps.Copy(tableByQuery,
		findWriteQueries(readQueryFiles(t, filepath.Join(storeDirectory, "cache", "queries")), cachegenPath))
	if len(tableByQuery) == 0 {
		t.Fatal("found no query that writes a table; the rule would pass vacuously")
	}
	schemaPlan, err := os.ReadFile(filepath.Join(storeDirectory, "..", "..", schemaPlanPath))
	if err != nil {
		t.Fatalf("read the owners table: %v", err)
	}
	ownerByTable := readTableOwners(string(schemaPlan))
	if len(ownerByTable) == 0 {
		t.Fatalf("%s has no owners table (\"Who writes each table\"); the rule would check nothing", schemaPlanPath)
	}
	queriesByPackage := loadGeneratedQueryCalls(t, packages)
	if len(queriesByPackage) == 0 {
		t.Fatal("found no package calling a generated query; the rule would pass vacuously")
	}
	reportViolations(t, findTablesWithoutOneWriter(queriesByPackage, tableByQuery, ownerByTable))
}
