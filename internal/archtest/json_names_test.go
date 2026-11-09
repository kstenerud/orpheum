// SPDX-License-Identifier: AGPL-3.0-or-later

package archtest

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"testing"
)

// camelCase is how every name Orpheum puts in JSON is spelled, a key or a value naming one of a
// closed set (D459): a lower-case letter, then letters and digits.
var camelCase = regexp.MustCompile(`^[a-z][a-zA-Z0-9]*$`)

// openAPIPath is the API's description, generated from the handlers, from this package's folder.
const openAPIPath = "../../api/openapi.json"

// foreignAPIValues are the API's enum values another format names, which keep its spelling (D459).
var foreignAPIValues = map[string]string{
	"h264_nvenc":    "FFmpeg's encoder name",
	"h264_qsv":      "FFmpeg's encoder name",
	"h264_vaapi":    "FFmpeg's encoder name",
	"h264-sdr":      "the codec ladder's name, which stays as other tools' identifiers do (the owner, 2026-10-27)",
	"hevc-sdr":      "the codec ladder's name, which stays as other tools' identifiers do (the owner, 2026-10-27)",
	"hevc-hdr10":    "the codec ladder's name, which stays as other tools' identifiers do (the owner, 2026-10-27)",
	"av1-sdr":       "the codec ladder's name, which stays as other tools' identifiers do (the owner, 2026-10-27)",
	"av1-hdr10":     "the codec ladder's name, which stays as other tools' identifiers do (the owner, 2026-10-27)",
	"aac-stereo":    "the codec ladder's name, which stays as other tools' identifiers do (the owner, 2026-10-27)",
	"aac-multi":     "the codec ladder's name, which stays as other tools' identifiers do (the owner, 2026-10-27)",
	"h264-aac":      "the codec ladder's name, which stays as other tools' identifiers do (the owner, 2026-10-27)",
	"cache.db":      "a file's name",
	"persistent.db": "a file's name",
}

// wirePackages are the packages whose named string types are values a JSON message carries: the
// node protocol, the discovery protocol, the API's own and the watch-together protocol's.
var wirePackages = map[string]bool{
	modulePath + "/internal/nodeproto":     true,
	modulePath + "/internal/core/api":      true,
	modulePath + "/internal/core/together": true,
	publicPath + "/discoveryproto":         true,
}

// foreignWireTypes are named string types in wirePackages whose values another format names.
var foreignWireTypes = map[string]string{
	"ArtSizeName": "the file name in an image's address (D212)",
}

// isOwnNonCamelName says whether text is a name Orpheum spells that is not camelCase: neither
// empty, camelCase nor one of foreign's.
func isOwnNonCamelName(text string, foreign map[string]string) bool {
	_, isForeign := foreign[text]
	return text != "" && !isForeign && !camelCase.MatchString(text)
}

// nonCamelNamesOf adds to found each property name and enum value object names itself that is not
// camelCase.
func nonCamelNamesOf(object map[string]any, found map[string]bool) {
	if properties, isObject := object["properties"].(map[string]any); isObject {
		for name := range properties {
			if isOwnNonCamelName(name, nil) {
				found["property "+strconv.Quote(name)] = true
			}
		}
	}
	values, isList := object["enum"].([]any)
	for index := 0; isList && index < len(values); index++ {
		listed := values[index]
		if text, isText := listed.(string); isText && isOwnNonCamelName(text, foreignAPIValues) {
			found["enum value "+strconv.Quote(text)] = true
		}
	}
}

// nonCamelNamesIn adds to found each property name and enum value in node that is not camelCase.
func nonCamelNamesIn(node any, found map[string]bool) {
	switch value := node.(type) {
	case map[string]any:
		nonCamelNamesOf(value, found)
		for _, child := range value {
			nonCamelNamesIn(child, found)
		}
	case []any:
		for _, child := range value {
			nonCamelNamesIn(child, found)
		}
	}
}

// findNonCamelAPINames returns every property name and string enum value in an OpenAPI document
// that is not camelCase and that foreignAPIValues does not name.
func findNonCamelAPINames(document any) []violation {
	found := map[string]bool{}
	nonCamelNamesIn(document, found)
	violations := make([]violation, 0, len(found))
	for name := range found {
		violations = append(violations, violation{where: openAPIPath, detail: name + " is not camelCase (D459)"})
	}
	sort.Slice(violations, func(i, j int) bool { return violations[i].detail < violations[j].detail })
	return violations
}

// nonCamelConstants returns each value in one const declaration of a named string type, not in
// foreignWireTypes, that is not camelCase.
func nonCamelConstants(fileSet *token.FileSet, general *ast.GenDecl) []violation {
	var violations []violation
	typeName := ""
	for _, spec := range general.Specs {
		valueSpec, isValue := spec.(*ast.ValueSpec)
		if !isValue {
			continue
		}
		// A spec with values and no type is an untyped constant; one with neither repeats the spec
		// before it, its type too.
		if valueSpec.Type != nil || len(valueSpec.Values) > 0 {
			typeName = ""
		}
		if named, isName := valueSpec.Type.(*ast.Ident); isName {
			typeName = named.Name
		}
		if _, foreign := foreignWireTypes[typeName]; typeName == "" || typeName == "string" || foreign {
			continue
		}
		violations = append(violations, nonCamelLiterals(fileSet, typeName, valueSpec.Values)...)
	}
	return violations
}

// nonCamelLiterals returns each string literal among values, constants of typeName, that is not
// camelCase.
func nonCamelLiterals(fileSet *token.FileSet, typeName string, values []ast.Expr) []violation {
	var violations []violation
	for _, value := range values {
		literal, isLiteral := value.(*ast.BasicLit)
		if !isLiteral || literal.Kind != token.STRING {
			continue
		}
		if text, err := strconv.Unquote(literal.Value); err == nil && isOwnNonCamelName(text, nil) {
			violations = append(violations, violation{
				where:  fileSet.Position(literal.Pos()).String(),
				detail: typeName + " " + strconv.Quote(text) + " is not camelCase (D459)",
			})
		}
	}
	return violations
}

// findNonCamelWireValues returns every constant of a named string type in files whose value is
// neither empty nor camelCase, its type not in foreignWireTypes.
func findNonCamelWireValues(files []sourceFile) []violation {
	var violations []violation
	for _, file := range files {
		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, file.path, file.contents, parser.SkipObjectResolution)
		if err != nil {
			violations = append(violations, violation{where: file.path, detail: "does not parse: " + err.Error()})
			continue
		}
		for _, declaration := range parsed.Decls {
			if general, isGeneral := declaration.(*ast.GenDecl); isGeneral && general.Tok == token.CONST {
				violations = append(violations, nonCamelConstants(fileSet, general)...)
			}
		}
	}
	return violations
}

// TestArch_JSONNamesAreCamelCase checks that every name Orpheum puts in JSON is camelCase (D459):
// each property and enum value the API's description lists, and each value of a named string type
// in the packages that define a protocol's messages. Keys in Go struct tags are tagliatelle's to
// check (.golangci.yml).
func TestArch_JSONNamesAreCamelCase(t *testing.T) {
	t.Parallel()
	contents, err := os.ReadFile(filepath.FromSlash(openAPIPath))
	if err != nil {
		t.Fatalf("read the API's description: %v", err)
	}
	var document any
	if err := json.Unmarshal(contents, &document); err != nil {
		t.Fatalf("decode %s: %v", openAPIPath, err)
	}
	reportViolations(t, findNonCamelAPINames(document))
	var wire []listedPackage
	for _, listed := range loadModulePackages(t) {
		if wirePackages[listed.ImportPath] {
			wire = append(wire, listedPackage{Dir: listed.Dir, GoFiles: listed.GoFiles})
		}
	}
	if len(wire) != len(wirePackages) {
		t.Fatalf("go list found %d of the %d protocol packages; one moved, and the rule would miss it",
			len(wire), len(wirePackages))
	}
	reportViolations(t, findNonCamelWireValues(readSourceFiles(t, wire)))
}

// The rule's checkers, seen to fail on input that breaks them (D190 §3).
func TestFindNonCamelNames_FindsEachSnakeKebabAndCapitalName(t *testing.T) {
	t.Parallel()
	document := map[string]any{"components": map[string]any{"schemas": map[string]any{"A": map[string]any{
		"properties": map[string]any{"fileId": map[string]any{}, "file_id": map[string]any{
			"enum": []any{"readThrough", "read_through", "read-through", "Idle", "h264_nvenc", ""},
		}},
	}}}}
	if got := findNonCamelAPINames(document); len(got) != 4 {
		t.Errorf("found %d non-camelCase names in a document with four, want 4: %v", len(got), got)
	}
	source := sourceFile{path: "a.go", contents: "package a\n\ntype Code string\n\ntype ArtSizeName string\n\n" +
		"const (\n\tA Code = \"tooBusy\"\n\tB Code = \"too_busy\"\n\tC Code = \"TooBusy\"\n" +
		"\tG = \"too-busy\"\n\tD Code = \"\"\n)\n\n" +
		"const E ArtSizeName = \"portrait-s\"\n\nconst F = \"free_text\"\n"}
	if got := findNonCamelWireValues([]sourceFile{source}); len(got) != 2 {
		t.Errorf("found %d non-camelCase values in a file with two, want 2: %v", len(got), got)
	}
}
