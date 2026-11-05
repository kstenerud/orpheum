// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// extensibleEnum marks an enumeration a later server may add values to: a client accepts a value it
// does not know, skipping the item or treating it as the closest it knows (D432 §2). Its value is
// the enumeration's values today, as Zalando's guidelines write it; the schema keeps its enum too,
// so a generated client still narrows on the values it knows.
const extensibleEnum = "x-extensible-enum"

// kindProperty is the name a response gives the kind of what it describes.
const kindProperty = "kind"

// openDescription is what the document says of its own openness (D432 §2).
const openDescription = " Responses are open: a client ignores a field it does not know, and a kind or an " +
	"error code it does not know (an enumeration marked x-extensible-enum gains values without notice), " +
	"skipping the item or treating it as the closest it knows. Requests are closed: a field the " +
	"document does not name is refused."

// openResponses makes the document's responses open (D432 §2): a schema only a response uses no
// longer refuses properties it does not name, and its kind enumerations and the error codes are
// marked extensible. A schema a request uses stays closed, so the server still refuses a request
// field it does not know; huma validates requests alone, so this changes no answer.
func openResponses(document *huma.OpenAPI) {
	schemas := document.Components.Schemas.Map()
	requested := map[string]bool{}
	operations := operationsIn(document)
	for _, each := range operations {
		for _, parameter := range each.operation.Parameters {
			markReferenced(parameter.Schema, schemas, requested)
		}
		if each.operation.RequestBody != nil {
			for _, content := range each.operation.RequestBody.Content {
				markReferenced(content.Schema, schemas, requested)
			}
		}
	}
	for name, schema := range schemas {
		if !requested[name] {
			openSchema(schema)
		}
	}
	for _, each := range operations {
		for _, response := range each.operation.Responses {
			for _, content := range response.Content {
				openSchema(content.Schema)
			}
		}
	}
}

// markReferenced marks every named schema the given one refers to, itself or through another.
func markReferenced(schema *huma.Schema, schemas map[string]*huma.Schema, marked map[string]bool) {
	eachSubschema(schema, func(each *huma.Schema) {
		name, isNamed := strings.CutPrefix(each.Ref, schemaRefPrefix)
		if !isNamed || marked[name] {
			return
		}
		marked[name] = true
		markReferenced(schemas[name], schemas, marked)
	})
}

// openSchema opens a schema and its inline parts, never one it refers to by name: each drops
// additionalProperties false, and a kind's enumeration is marked extensible (an error's code is
// marked where its forms are made, problemForm).
func openSchema(schema *huma.Schema) {
	eachSubschema(schema, func(each *huma.Schema) {
		if isClosed(each) {
			each.AdditionalProperties = nil
		}
		for name, property := range each.Properties {
			if name == kindProperty && len(property.Enum) > 0 {
				if property.Extensions == nil {
					property.Extensions = map[string]any{}
				}
				property.Extensions[extensibleEnum] = property.Enum
			}
		}
	})
}

// isClosed says whether schema refuses a property it does not name.
func isClosed(schema *huma.Schema) bool {
	allows, isBool := schema.AdditionalProperties.(bool)
	return isBool && !allows
}

// eachSubschema calls visit with schema and each schema written inside it, not following a
// reference by name.
func eachSubschema(schema *huma.Schema, visit func(*huma.Schema)) {
	if schema == nil {
		return
	}
	visit(schema)
	for _, property := range schema.Properties {
		eachSubschema(property, visit)
	}
	eachSubschema(schema.Items, visit)
	eachSubschema(schema.Not, visit)
	if additional, isSchema := schema.AdditionalProperties.(*huma.Schema); isSchema {
		eachSubschema(additional, visit)
	}
	for _, group := range [][]*huma.Schema{schema.OneOf, schema.AnyOf, schema.AllOf} {
		for _, each := range group {
			eachSubschema(each, visit)
		}
	}
}
