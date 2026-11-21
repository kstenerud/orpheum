// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// commandTypesDeclared reads every CommandType constant this package declares, from its source, so
// a test that walks them cannot fall behind a type added in another file.
func commandTypesDeclared(t *testing.T) []CommandType {
	t.Helper()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var found []CommandType
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			value, isValue := node.(*ast.ValueSpec)
			if named, isNamed := typeOf(value, isValue); !isNamed || named.Name != "CommandType" {
				return true
			}
			for _, literal := range value.Values {
				if text, isText := literal.(*ast.BasicLit); isText {
					found = append(found, CommandType(strings.Trim(text.Value, "\"")))
				}
			}
			return true
		})
	}
	if len(found) < 2 {
		t.Fatalf("found %d command types; the walk would pass vacuously", len(found))
	}
	return found
}

// typeOf is the type a constant or variable is declared with, when it names one.
func typeOf(value *ast.ValueSpec, isValue bool) (*ast.Ident, bool) {
	if !isValue {
		return nil, false
	}
	named, isNamed := value.Type.(*ast.Ident)
	return named, isNamed
}

// Every command type answers which slot it runs in and which media file it reads, set down here
// for each: a type added without a row fails, as one the methods do not answer fails the exhaustive
// lint, so a new command cannot quietly run unthrottled or hold no reader of its disk (the code
// audit of 2026-11-21, finding 1).
func TestWork_EveryCommandTypeAnswersItsSlotAndMediaFile(t *testing.T) {
	t.Parallel()
	video := BytesFile{RootPath: "/media", Path: []byte("a.mkv")}
	type answer struct {
		work Work
		slot Slot
		file string
	}
	answers := map[CommandType]answer{
		CommandExtractSubtitle: {Work{ExtractSubtitle: &ExtractSubtitle{Video: &video}}, SlotExtract, "a.mkv"},
		CommandReadThrough:     {Work{ReadThrough: &ReadThrough{Video: video}}, SlotReadThrough, "a.mkv"},
		CommandMakeChunks:      {Work{MakeChunks: &MakeChunks{Video: video}}, SlotRemux, "a.mkv"},
		CommandHashWhole:       {Work{HashWhole: &HashWhole{File: video}}, SlotReadThrough, "a.mkv"},
		CommandFindRepeats: {
			Work{FindRepeats: &FindRepeats{Videos: []RepeatVideo{{Video: video}}}}, SlotReadThrough, "a.mkv",
		},
		CommandList: {Work{List: &List{RootPath: "/media", Path: "a.mkv"}}, "", "a.mkv"},
		CommandExamine: {
			Work{Examine: &Examine{RootPath: "/media", Files: []ExamineFile{{Path: Path{Text: "a.mkv"}}}}}, "", "a.mkv",
		},
		CommandEncodeArtwork:    {Work{EncodeArtwork: &ArtworkBatch{}}, SlotArtwork, ""},
		CommandMakeSystemVideos: {Work{MakeSystemVideos: &MakeSystemVideos{}}, SlotArtwork, ""},
		CommandCancel:           {},
		CommandReprioritize:     {},
	}
	for _, commandType := range commandTypesDeclared(t) {
		each, isAnswered := answers[commandType]
		if !isAnswered {
			t.Errorf("command type %q has no row here: say which slot it runs in and which file it reads, "+
				"here and in Work.Slot and Work.MediaFile", commandType)
			continue
		}
		each.work.Type = commandType
		if got := each.work.Slot(); got != each.slot {
			t.Errorf("%s runs in slot %q, want %q", commandType, got, each.slot)
		}
		file, isMedia := each.work.MediaFile()
		if string(file.Path) != each.file || isMedia != (each.file != "") {
			t.Errorf("%s reads %q (%v), want %q", commandType, file.Path, isMedia, each.file)
		}
	}
}

// Every result that can say its work stalled answers it through Result.Stall, walked over every
// payload a result carries: one added with a Stall the method does not read fails, and one without
// must be named as having none (the code audit of 2026-11-21, finding 1).
func TestResult_StallAnswersEveryResultThatCarriesOne(t *testing.T) {
	t.Parallel()
	// The results whose work has no stall to report: an artwork batch encodes images it was sent, and
	// the system videos are made from nothing on a disk.
	resultsWithoutStall := []string{"Artwork", "SystemVideos"}
	resultType := reflect.TypeFor[Result]()
	stallType := reflect.TypeFor[*Stall]()
	payloads := 0
	for index := range resultType.NumField() {
		field := resultType.Field(index)
		if field.Type.Kind() != reflect.Pointer || field.Type.Elem().Kind() != reflect.Struct ||
			field.Name == "Work" || field.Name == "Held" {
			continue
		}
		payloads++
		payload := reflect.New(field.Type.Elem())
		stallField, hasStall := field.Type.Elem().FieldByName("Stall")
		var result Result
		reflect.ValueOf(&result).Elem().Field(index).Set(payload)
		if !hasStall || stallField.Type != stallType {
			if !slices.Contains(resultsWithoutStall, field.Name) {
				t.Errorf("result %s carries no Stall: name it in resultsWithoutStall, or give it one and read "+
					"it in Result.Stall", field.Name)
			}
			if got := result.Stall(); got != nil {
				t.Errorf("result %s carries no Stall, but Stall answers %v", field.Name, got)
			}
			continue
		}
		want := &Stall{}
		payload.Elem().FieldByIndex(stallField.Index).Set(reflect.ValueOf(want))
		if got := result.Stall(); got != want {
			t.Errorf("result %s's Stall is not answered by Result.Stall", field.Name)
		}
	}
	if payloads < 2 {
		t.Fatalf("found %d result payloads; the walk would pass vacuously", payloads)
	}
}
