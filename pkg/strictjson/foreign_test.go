// SPDX-License-Identifier: AGPL-3.0-or-later

package strictjson_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/pkg/strictjson"
)

const (
	lastWins  = strictjson.LastWins
	firstWins = strictjson.FirstWins
)

// decodedBy is what reading document by foreign gives, decoded free-form.
func decodedBy(t *testing.T, foreign strictjson.Foreign, document string) any {
	t.Helper()
	var got any
	if err := foreign.Unmarshal([]byte(document), &got); err != nil {
		t.Fatalf("%s: %v", document, err)
	}
	return got
}

// wantJSON is document decoded free-form by encoding/json, which holds no repeated key.
func wantJSON(t *testing.T, document string) any {
	t.Helper()
	var want any
	if err := json.Unmarshal([]byte(document), &want); err != nil {
		t.Fatalf("%s: %v", document, err)
	}
	return want
}

func TestForeign_ARepeatedKeyKeepsTheValueItsSenderKeeps(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, document, last, first string
	}{
		{"a repeat in the middle", `{"a":1,"b":2,"a":3}`, `{"b":2,"a":3}`, `{"a":1,"b":2}`},
		{"a repeat at the end, spaced", `{ "a" : 1 , "a" : 2 }`, `{"a":2}`, `{"a":1}`},
		{"a repeat at the start", `{"a":1,"a":2,"b":3}`, `{"a":2,"b":3}`, `{"a":1,"b":3}`},
		{"three of one key", `{"a":1,"a":2,"a":3}`, `{"a":3}`, `{"a":1}`},
		{"two keys repeated", `{"a":1,"b":1,"a":2,"b":2}`, `{"a":2,"b":2}`, `{"a":1,"b":1}`},
		{
			"repeats inside the repeated values",
			`{"a":{"x":1,"x":2},"a":{"y":1,"y":2}}`, `{"a":{"y":2}}`, `{"a":{"x":1}}`,
		},
		{"a repeat in an object in a list", `[{"a":1,"a":2},{"a":3}]`, `[{"a":2},{"a":3}]`, `[{"a":1},{"a":3}]`},
		{
			"a repeat in a large object",
			largeObject(40) + `,"key7":"again"}`, largeObject(40) + `}`, largeObject(40) + `}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			last := test.last
			if strings.HasPrefix(test.name, "a repeat in a large") {
				last = strings.Replace(test.last, `"key7":1`, `"key7":"again"`, 1)
			}
			if got, want := decodedBy(t, lastWins, test.document), wantJSON(t, last); !reflect.DeepEqual(got, want) {
				t.Errorf("last wins: %s gave %v, want %v", test.document, got, want)
			}
			got, want := decodedBy(t, firstWins, test.document), wantJSON(t, test.first)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("first wins: %s gave %v, want %v", test.document, got, want)
			}
		})
	}
}

// FFmpeg's ASF reader gives WM/EncodingSettings and WM/Tool both as encoder, and ffprobe shows the
// first (av_dict_get).
func TestForeign_FirstWinsReadsFfprobesRepeatedTagAsFfprobeShowsIt(t *testing.T) {
	t.Parallel()
	var probe struct {
		Format struct {
			Tags map[string]string `json:"tags"`
		} `json:"format"`
	}
	document := `{"format":{"tags":{"encoder":"Lavf58","title":"A","encoder":"Windows Media"}}}`
	if err := firstWins.Unmarshal([]byte(document), &probe); err != nil {
		t.Fatal(err)
	}
	if got := probe.Format.Tags["encoder"]; got != "Lavf58" {
		t.Errorf("encoder = %q, want the first, Lavf58", got)
	}
	if got := probe.Format.Tags["title"]; got != "A" {
		t.Errorf("title = %q, want A: a kept member beside a dropped one", got)
	}
}

func TestForeign_TakesWhatJSONAllowsAsEncodingJSONReadsIt(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ name, document string }{
		{"an escaped NUL", `["a\u0000b"]`},
		{"invalid UTF-8 in a value", "[\"a\xffb\"]"},
		{"invalid UTF-8 in a key", "{\"a\xff\":1}"},
		{"a lone low surrogate", `["a\udc00b"]`},
		{"a lone high surrogate", `["a\ud800b"]`},
		{"a high surrogate at the end", `["a\ud800"]`},
		{"a high surrogate before a letter's escape", `["\ud800A"]`},
		{"a high surrogate before another pair", `["\ud800😀"]`},
		{"keys equal only after NFC", `{"` + cafePrecomposed + `":1,"` + cafeDecomposed + `":2}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, want := decodedBy(t, lastWins, test.document), wantJSON(t, test.document)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%q gave %#v, want encoding/json's %#v", test.document, got, want)
			}
		})
	}
}

func TestForeign_StillRefusesWhatNoSenderIsAllowed(t *testing.T) {
	t.Parallel()
	type named struct {
		Name string `json:"name"`
	}
	for _, test := range []struct{ name, document, names string }{
		{"a key naming a field in another case", `{"Name":"x"}`, "only in another case"},
		{"data after the value", `{"name":"x"} {}`, "data after the top-level value"},
		{"nesting past the limit", strings.Repeat("[", strictjson.MaxDepth+1), "nesting deeper"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var into named
			err := lastWins.Unmarshal([]byte(test.document), &into)
			if !errors.Is(err, strictjson.ErrRefused) || !strings.Contains(err.Error(), test.names) {
				t.Errorf("%s: err = %v, want ErrRefused naming %q", test.document, err, test.names)
			}
		})
	}
	var into any
	if err := lastWins.Unmarshal([]byte("[\"a\x00b\"]"), &into); err == nil {
		t.Error("a raw NUL, a control character JSON does not allow in a string, was taken")
	}
}

func TestForeign_ARepeatsRuleItDoesNotKnowIsAnError(t *testing.T) {
	t.Parallel()
	var into any
	if err := strictjson.Foreign(0).Unmarshal([]byte(`{}`), &into); err == nil {
		t.Error("a Foreign that is neither rule read a document")
	}
}

func TestForeign_DecodeReadsAsUnmarshalDoes(t *testing.T) {
	t.Parallel()
	var got map[string]int
	if err := firstWins.Decode(strings.NewReader(`{"a":1,"a":2}`), &got); err != nil {
		t.Fatal(err)
	}
	if got["a"] != 1 {
		t.Errorf("a = %d, want the first, 1", got["a"])
	}
}
