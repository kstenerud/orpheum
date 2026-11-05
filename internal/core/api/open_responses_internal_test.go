// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
)

// validateAgainst validates value against the document's schema of that name, as a client would
// read a response or the server a request, and says what it refused.
func validateAgainst(t *testing.T, document *huma.OpenAPI, name string, value any) []string {
	t.Helper()
	registry := document.Components.Schemas
	schema := registry.Map()[name]
	if schema == nil {
		t.Fatalf("the document has no schema %s", name)
	}
	schema.PrecomputeMessages()
	result := &huma.ValidateResult{}
	huma.Validate(registry, schema, huma.NewPathBuffer([]byte{}, 0), huma.ModeReadFromServer, value, result)
	refused := make([]string, 0, len(result.Errors))
	for _, each := range result.Errors {
		refused = append(refused, each.Error())
	}
	return refused
}

// extensibleValues are the values schema's enumeration is marked extensible with, nil when it is not.
func extensibleValues(schema *huma.Schema) []any {
	values, isList := schema.Extensions[extensibleEnum].([]any)
	if !isList {
		return nil
	}
	return values
}

// A response is open: its schema takes a field the document does not name, its kinds and the error
// codes are marked extensible, and the document says a client ignores what it does not know. A
// request stays closed (D432 §2).
func TestDocument_ResponsesAreOpenAndRequestsClosed(t *testing.T) {
	t.Parallel()
	document := NewServer(slog.New(slog.DiscardHandler)).OpenAPI()
	library := map[string]any{
		"id": 1, "name": "Films", "kind": "movie", "metadataSource": "tmdb", "changeDetection": "watch",
		"isWritable": true, "createdAt": 1, "addedByALaterServer": "ignored",
	}
	if refused := validateAgainst(t, document, "LibraryBody", library); len(refused) != 0 {
		t.Errorf("a library with a field a later server added is refused: %v", refused)
	}
	signIn := map[string]any{
		"name": "admin", "password": "secret", "device": map[string]any{"name": "phone"}, "unknown": 1,
	}
	if refused := validateAgainst(t, document, "SignInInputBody", signIn); !slices.ContainsFunc(refused,
		func(each string) bool { return strings.Contains(each, "unknown") }) {
		t.Errorf("a sign-in with a field the document does not name is not refused for it: %v", refused)
	}
	schemas := document.Components.Schemas.Map()
	for _, name := range []string{"LibraryBody", "TitleEntryBody", "SeriesEntryBody", "GroupHitBody"} {
		kind := schemas[name].Properties[kindProperty]
		if extensible := extensibleValues(kind); !slices.Equal(extensible, kind.Enum) {
			t.Errorf("%s's kind is marked %s %v; want its values %v", name, extensibleEnum, extensible, kind.Enum)
		}
	}
	for index, form := range schemas[problemBodySchemaName].OneOf {
		code := form.Properties["code"]
		if extensible := extensibleValues(code); !slices.Equal(extensible, code.Enum) {
			t.Errorf("the error's form %d's code is marked %s %v; want %v", index, extensibleEnum, extensible,
				code.Enum)
		}
		if isClosed(form) {
			t.Errorf("the error's form %d refuses a field it does not name", index)
		}
	}
	if !strings.Contains(document.Info.Description, "a client ignores a field it does not know") {
		t.Errorf("the document's description does not say responses are open: %q", document.Info.Description)
	}
}
