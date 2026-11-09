// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
)

// GET /items' entries are a union on kind in the document, a title's entry or a series', so a
// generated client narrows on kind (D425 §7); each variant has only its own kind's fields.
func TestItemEntries_AreAUnionOnKindInTheDocument(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	served := harness.do(t, http.MethodGet, "/openapi.json", "", nil)
	var document struct {
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal([]byte(served.body), &document); err != nil || served.status != http.StatusOK {
		t.Fatalf("GET /openapi.json = %d: %v", served.status, err)
	}
	var listed struct {
		Properties struct {
			Items struct {
				Items struct {
					OneOf []struct {
						Ref string `json:"$ref"`
					} `json:"oneOf"`
					Discriminator struct {
						PropertyName string            `json:"propertyName"`
						Mapping      map[string]string `json:"mapping"`
					} `json:"discriminator"`
				} `json:"items"`
			} `json:"items"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(document.Components.Schemas["ListItemsOutputBody"], &listed); err != nil {
		t.Fatal(err)
	}
	entry := listed.Properties.Items.Items
	const title, series = "#/components/schemas/TitleEntryBody", "#/components/schemas/SeriesEntryBody"
	if len(entry.OneOf) != 2 || entry.OneOf[0].Ref != title || entry.OneOf[1].Ref != series {
		t.Errorf("listItems' entry is oneOf %+v; want the title's entry and the series'", entry.OneOf)
	}
	want := map[string]string{"film": title, "episode": title, "extra": title, "other": title, "series": series}
	if entry.Discriminator.PropertyName != "kind" || len(entry.Discriminator.Mapping) != len(want) {
		t.Errorf("listItems' entry's discriminator: %+v; want kind, mapped %v", entry.Discriminator, want)
	}
	for kind, ref := range want {
		if entry.Discriminator.Mapping[kind] != ref {
			t.Errorf("kind %s maps to %q, want %q", kind, entry.Discriminator.Mapping[kind], ref)
		}
	}
	for name, absent := range map[string]string{"TitleEntryBody": "episodeCount", "SeriesEntryBody": "watchedPercent"} {
		var variant struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		err := json.Unmarshal(document.Components.Schemas[name], &variant)
		if err != nil || variant.Properties["kind"] == nil {
			t.Fatalf("%s: %v, %s", name, err, document.Components.Schemas[name])
		}
		if _, has := variant.Properties[absent]; has {
			t.Errorf("%s declares %s, another kind's field", name, absent)
		}
	}
}

// An entry is written as the variant its kind is: a series' entry without a title's fields, a
// film's without a series'.
func TestItemEntries_AreWrittenAsTheirKindsVariant(t *testing.T) {
	t.Parallel()
	for _, each := range []struct{ kind, kept, dropped string }{
		{"series", "episodeCount", "watchedPercent"},
		{"movie", "watchedPercent", "episodeCount"},
	} {
		entry := api.ItemEntryBody{Kind: each.kind}
		entry.ID, entry.Name = 1, "A"
		entry.WatchedPercent, entry.EpisodeCount = 40, 8
		encoded, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		var written map[string]any
		if err := json.Unmarshal(encoded, &written); err != nil {
			t.Fatal(err)
		}
		if _, has := written[each.dropped]; has || written[each.kept] == nil || written["kind"] != each.kind {
			t.Errorf("a %s's entry is written %s; want %s and not %s", each.kind, encoded, each.kept, each.dropped)
		}
	}
}
