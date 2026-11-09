// SPDX-License-Identifier: AGPL-3.0-or-later

package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"strconv"
	"testing"
)

// strictJSONPackages are the two copies of the strict decoder (D456), which alone call
// encoding/json's decoding to do it.
var strictJSONPackages = map[string]bool{
	modulePath + "/internal/platform/strictjson": true,
	publicPath + "/internal/strictjson":          true,
}

// laxJSONFiles are the files D456's Status names as still decoding with encoding/json, each with
// why, until the owner decides them.
var laxJSONFiles = map[string]string{
	modulePath + "/internal/node/probe/probe.go":    "ffprobe gives a real file's tags one key twice",
	modulePath + "/internal/node/probe/disc.go":     "ffprobe gives a real file's tags one key twice",
	modulePath + "/internal/node/artwork/derive.go": "ffprobe gives a real file's tags one key twice",
	modulePath + "/internal/core/providers/opensubtitles/client.go": "a provider's NUL is cleaned " +
		"where it enters (D211 §2)",
	modulePath + "/internal/core/identify/identifytest/provider.go": "the fake provider, as the real ones",
}

// laxDecoders are encoding/json's functions that decode a document without D456's rules.
var laxDecoders = map[string]bool{"Unmarshal": true, "NewDecoder": true}

// goFile is a compiled file named by its package's import path and its own name.
type goFile struct {
	name     string
	contents string
}

// encodingJSONNames returns the names a file imports encoding/json as.
func encodingJSONNames(parsed *ast.File) map[string]bool {
	names := map[string]bool{}
	for _, imported := range parsed.Imports {
		if importPath, err := strconv.Unquote(imported.Path.Value); err != nil || importPath != "encoding/json" {
			continue
		}
		name := "json"
		if imported.Name != nil {
			name = imported.Name.Name
		}
		names[name] = true
	}
	return names
}

// findLaxJSONDecoding returns every call of encoding/json's Unmarshal or NewDecoder in files,
// under whatever name the file imports encoding/json as.
func findLaxJSONDecoding(files []goFile) []violation {
	var violations []violation
	for _, file := range files {
		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, file.name, file.contents, parser.SkipObjectResolution)
		if err != nil {
			violations = append(violations, violation{where: file.name, detail: "does not parse: " + err.Error()})
			continue
		}
		names := encodingJSONNames(parsed)
		ast.Inspect(parsed, func(node ast.Node) bool {
			selector, isSelector := node.(*ast.SelectorExpr)
			if !isSelector || !laxDecoders[selector.Sel.Name] {
				return true
			}
			if receiver, isName := selector.X.(*ast.Ident); isName && names[receiver.Name] {
				violations = append(violations, violation{
					where:  fileSet.Position(selector.Pos()).String(),
					detail: "decodes with encoding/json; use internal/platform/strictjson (D456)",
				})
			}
			return true
		})
	}
	return violations
}

// TestArch_JSONIsDecodedStrictly checks the files the module compiles, tests aside: none decodes
// JSON with encoding/json but the strict decoder's own and the files D456 names, so every document
// the Go code reads keeps D456's rules. Tests may: a test builds what it checks.
func TestArch_JSONIsDecodedStrictly(t *testing.T) {
	t.Parallel()
	var files []goFile
	for _, listed := range loadModulePackages(t) {
		if strictJSONPackages[listed.ImportPath] {
			continue
		}
		for _, source := range readSourceFiles(t, []listedPackage{{Dir: listed.Dir, GoFiles: listed.GoFiles}}) {
			name := listed.ImportPath + "/" + path.Base(source.path)
			if _, isLax := laxJSONFiles[name]; !isLax {
				files = append(files, goFile{name: name, contents: source.contents})
			}
		}
	}
	if len(files) == 0 {
		t.Fatal("go list found no source files; the rule would pass vacuously")
	}
	reportViolations(t, findLaxJSONDecoding(files))
}

// The rule's checker, seen to fail on input that breaks it (D190 §3).
func TestFindLaxJSONDecoding_FindsEachDecoderUnderItsImportedName(t *testing.T) {
	t.Parallel()
	lax := []goFile{
		{name: "a.go", contents: "package a\n\nimport \"encoding/json\"\n\nvar _ = json.Unmarshal\n"},
		{name: "b.go", contents: "package b\n\nimport std \"encoding/json\"\n\nvar _ = std.NewDecoder\n"},
	}
	if got := findLaxJSONDecoding(lax); len(got) != 2 {
		t.Errorf("found %d lax decodings in two, want 2: %v", len(got), got)
	}
	clean := []goFile{
		{name: "c.go", contents: "package c\n\nimport \"encoding/json\"\n\nvar _ = json.Marshal\n"},
		{name: "d.go", contents: "package d\n\nimport json \"example.com/strictjson\"\n\nvar _ = json.Unmarshal\n"},
	}
	if got := findLaxJSONDecoding(clean); len(got) != 0 {
		t.Errorf("found lax decodings in clean files: %v", got)
	}
}
