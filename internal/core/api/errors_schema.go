// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"reflect"

	"github.com/danielgtaylor/huma/v2"
)

// problemBodySchemaName is ProblemBody's name among the document's schemas.
const problemBodySchemaName = "ProblemBody"

// Schema is ProblemBody's schema: one form per code with detail, typed by its detail's struct, and
// one for every other code, which carries none. It is registered once under its own name, so the
// SDK names it, and referred to from where it is used.
func (ProblemBody) Schema(registry huma.Registry) *huma.Schema {
	if _, isRegistered := registry.Map()[problemBodySchemaName]; !isRegistered {
		detailed := map[Code]bool{}
		details := problemDetails()
		forms := make([]*huma.Schema, 0, len(details)+1)
		for _, detail := range details {
			detailed[detail.problemCode()] = true
			forms = append(forms, problemForm([]Code{detail.problemCode()},
				registry.Schema(reflect.TypeOf(detail), true, "")))
		}
		var plain []Code
		for _, code := range codes() {
			if !detailed[code] {
				plain = append(plain, code)
			}
		}
		forms = append(forms, problemForm(plain, nil))
		registry.Map()[problemBodySchemaName] = &huma.Schema{
			Description: "An error: a person is shown words chosen by its code, never its message (cut-a §5.2)",
			OneOf:       forms,
		}
	}
	return &huma.Schema{Ref: schemaRefPrefix + problemBodySchemaName}
}

// schemaRefPrefix is where the document's schemas are, as huma's default configuration puts them.
const schemaRefPrefix = "#/components/schemas/"

// problemForm is one form of ProblemBody: its codes, and the detail they carry, if any.
func problemForm(formCodes []Code, detail *huma.Schema) *huma.Schema {
	names := make([]any, 0, len(formCodes))
	for _, code := range formCodes {
		names = append(names, string(code))
	}
	properties := map[string]*huma.Schema{
		"code": {
			Type: huma.TypeString, Enum: names,
			Extensions: map[string]any{extensibleEnum: names},
		},
		"message": {Type: huma.TypeString, Description: "For developers, in English"},
		"failedHop": {
			Type: huma.TypeString, Enum: []any{string(HopNode), string(HopProvider)},
			Description: "The leg of the request that failed, for reachability errors",
		},
	}
	if detail != nil {
		properties["detail"] = detail
	}
	return &huma.Schema{
		Type: huma.TypeObject, Properties: properties, Required: []string{"code", "message"},
		AdditionalProperties: false,
	}
}
