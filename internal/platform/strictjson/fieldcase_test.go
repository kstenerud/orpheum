// SPDX-License-Identifier: AGPL-3.0-or-later

package strictjson_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/strictjson"
)

type inner struct {
	Name string `json:"name"`
}

type Promoted struct {
	Title string `json:"title"`
}

type PromotedByPointer struct {
	Year int
}

type selfDecoding struct{ raw []byte }

func (decoding *selfDecoding) UnmarshalJSON(data []byte) error {
	decoding.raw = append([]byte(nil), data...)
	return nil
}

type outer struct {
	Promoted
	*PromotedByPointer

	Name     string                   `json:"name"`
	Inner    inner                    `json:"inner"`
	Pointer  *inner                   `json:"pointer"`
	List     []inner                  `json:"list"`
	Array    [2]inner                 `json:"array"`
	ByKey    map[string]inner         `json:"byKey"`
	Free     map[string]int           `json:"free"`
	Any      any                      `json:"any"`
	Raw      json.RawMessage          `json:"raw"`
	Self     selfDecoding             `json:"self"`
	Skipped  string                   `json:"-"`
	Untagged int                      // named by its Go name
	Options  string                   `json:",omitempty"` //nolint:tagliatelle // named by its Go name, in its case
	Nested   map[string][]*inner      `json:"nested"`
	unread   int                      //nolint:unused // an unexported field decodes from no key
	Pointers map[string]*selfDecoding `json:"pointers"`
}

func TestUnmarshal_RefusesAKeyNamingAFieldOnlyInAnotherCase(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ name, document, key, field string }{
		{"at the top", `{"NAME":"a"}`, `"NAME"`, `"name"`},
		{"in a nested struct", `{"inner":{"Name":"a"}}`, `"Name"`, `"name"`},
		{"behind a pointer", `{"pointer":{"nAme":"a"}}`, `"nAme"`, `"name"`},
		{"in a slice's element", `{"list":[{"name":"a"},{"NAME":"b"}]}`, `"NAME"`, `"name"`},
		{"in an array's element", `{"array":[{"Name":"a"}]}`, `"Name"`, `"name"`},
		{"in a map's value", `{"byKey":{"x":{"NAME":"a"}}}`, `"NAME"`, `"name"`},
		{"deep in maps and slices", `{"nested":{"x":[{"NAME":"a"}]}}`, `"NAME"`, `"name"`},
		{"through an embedded struct", `{"Title":"a"}`, `"Title"`, `"title"`},
		{"through an embedded pointer", `{"year":1999}`, `"year"`, `"Year"`},
		{"of a field named by its Go name", `{"untagged":1}`, `"untagged"`, `"Untagged"`},
		{"of a field whose tag gives no name", `{"options":"x"}`, `"options"`, `"Options"`},
		{"in the outer field's own case", `{"Inner":{}}`, `"Inner"`, `"inner"`},
	} {
		var into outer
		err := strictjson.Unmarshal([]byte(test.document), &into)
		if !errors.Is(err, strictjson.ErrRefused) || !strings.Contains(err.Error(), test.key) ||
			!strings.Contains(err.Error(), test.field) {
			t.Errorf("%s: %v; want ErrRefused naming the key %s and the field %s", test.name, err, test.key,
				test.field)
		}
	}
}

func TestUnmarshal_AcceptsKeysInTheirFieldsCaseAndFreeFormKeysInAnyCase(t *testing.T) {
	t.Parallel()
	for _, document := range []string{
		`{"name":"a","inner":{"name":"b"},"pointer":{"name":"c"},"list":[{"name":"d"}],"array":[{"name":"e"}],
			"byKey":{"x":{"name":"f"}},"title":"g","Year":1,"Untagged":2,"Options":"h","nested":{"x":[{"name":"i"}]}}`,
		`{"free":{"name":1,"NAME":2,"Name":3}}`,                  // a map's keys are the sender's
		`{"any":{"NAME":1,"inner":{"Name":2}}}`,                  // so is anything an interface holds
		`{"raw":{"NAME":1}}`,                                     // and a raw message
		`{"self":{"NAME":1}}`,                                    // and a type that decodes itself
		`{"pointers":{"x":{"NAME":1}}}`,                          // behind a pointer too
		`{"unknown":1,"alsoUnknown":{"NAME":1},"inner":{"x":1}}`, // a key naming no field, in any case
		`{"Skipped":"x","skipped":"y","-":"z","unread":1}`,       // and keys naming only an ignored field
	} {
		var into outer
		if err := strictjson.Unmarshal([]byte(document), &into); err != nil {
			t.Errorf("%s: %v; want it decoded", document, err)
		}
	}
	var into outer
	if err := strictjson.Unmarshal([]byte(`{"title":"t","Year":1999}`), &into); err != nil ||
		into.Title != "t" || into.PromotedByPointer == nil || into.Year != 1999 {
		t.Errorf("promoted fields: %+v, %v", into, err)
	}
}

// Two fields at one depth claiming one name leave it to neither, as encoding/json does; the
// shallower of two depths wins.
func TestUnmarshal_FollowsEncodingJSONsChoiceBetweenFieldsClaimingOneName(t *testing.T) {
	t.Parallel()
	type left struct{ Name string }
	type right struct{ Name string }
	type tied struct {
		left
		right
	}
	var both tied
	if err := strictjson.Unmarshal([]byte(`{"NAME":"x"}`), &both); err != nil {
		t.Errorf("a name no field takes: %v; want it ignored as unknown", err)
	}
	type shallow struct {
		left
		Name string `json:"title"`
	}
	var into shallow
	err := strictjson.Unmarshal([]byte(`{"name":"x"}`), &into)
	if !errors.Is(err, strictjson.ErrRefused) || !strings.Contains(err.Error(), `"Name"`) {
		t.Errorf("the promoted Name in another case: %v; want ErrRefused", err)
	}
}
