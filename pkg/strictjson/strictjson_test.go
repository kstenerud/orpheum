// SPDX-License-Identifier: AGPL-3.0-or-later

package strictjson_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/pkg/strictjson"
)

// refused is a document each rule of D456 refuses, with what the error must name.
type refused struct {
	name, document, names string
}

func TestUnmarshal_RefusesWhatEachRuleRefuses(t *testing.T) {
	t.Parallel()
	for _, test := range []refused{
		{"a key said twice", `{"name":1,"name":2}`, "duplicate key"},
		{"a key said twice in a nested object", `{"a":[{"x":1,"x":2}]}`, `$."a"[0]`},
		{"keys equal after NFC", `{"` + cafePrecomposed + `":1,"` + cafeDecomposed + `":2}`, "duplicate key"},
		{
			"keys equal after NFC, one escaped",
			`{"` + cafePrecomposed + `":1,"cafe` + u("0301") + `":2}`, "duplicate key",
		},
		{"a key said twice in a large object", largeObject(100) + `,"key7":1}`, "duplicate key"},
		{
			"keys equal after NFC in a large object",
			largeObject(100) + `,"` + cafePrecomposed + `":1,"` + cafeDecomposed + `":1}`, "duplicate key",
		},
		{"invalid UTF-8 in a value", "[\"a\xffb\"]", "invalid UTF-8"},
		{"invalid UTF-8 in a key", "{\"a\xff\":1}", "invalid UTF-8"},
		{"an overlong encoding", "[\"\xc0\xae\"]", "invalid UTF-8"},
		{"a surrogate encoded in UTF-8", "[\"\xed\xa0\x80\"]", "invalid UTF-8"},
		{"a truncated sequence", "[\"\xe2\x82\"]", "invalid UTF-8"},
		{"a lone high surrogate", `["` + u("d83d") + `"]`, "lone high surrogate"},
		{"a lone high surrogate before a letter", `["` + u("d83d") + `x"]`, "lone high surrogate"},
		{"a lone low surrogate", `["` + u("de00") + `"]`, "lone low surrogate"},
		{"a high surrogate before a letter escaped", `["` + u("d83d") + u("0041") + `"]`, "not followed by a low one"},
		{"two high surrogates", `["` + u("d83d") + u("d83d") + `"]`, "not followed by a low one"},
		{"an escaped NUL in a value", `{"a":"x` + u("0000") + `"}`, "NUL"},
		{"an escaped NUL in a key", `{"a` + u("0000") + `":1}`, "NUL"},
		{"a raw NUL", "[\"x\x00\"]", "NUL"},
		{"a second value", `{} {}`, "after the top-level value"},
		{"a number after the value", `1 2`, "after the top-level value"},
		{"garbage after the value", `{"a":1}x`, "after the top-level value"},
	} {
		var into any
		err := strictjson.Unmarshal([]byte(test.document), &into)
		if !errors.Is(err, strictjson.ErrRefused) || !strings.Contains(err.Error(), test.names) {
			t.Errorf("%s: %v; want ErrRefused naming %q", test.name, err, test.names)
		}
		if into != nil {
			t.Errorf("%s: decoded %v from a refused document", test.name, into)
		}
	}
}

func TestUnmarshal_AcceptsWhatTheRulesAllow(t *testing.T) {
	t.Parallel()
	for _, document := range []string{
		`{"a":{"x":1},"b":{"x":2}}`, // the same key in two objects
		`[{"x":1},{"x":2}]`,         // and in two elements
		`{"a":{"b":1},"b":2}`,       // a nested key again outside
		`{"x":1,"y":2,"xy":3}`,      // keys that share a prefix
		`["` + u("d83d") + u("de00") + "\", \"\xf0\x9f\x98\x80\"]", // a surrogate pair, escaped and raw
		`["` + u("00e9") + `", "caf` + u("00e9") + `"]`,            // escapes that are not NUL
		`{"` + cafePrecomposed + `":1,"cafe":2}`,                   // keys that differ
		`{"name":1,"NAME":2}`,                                      // two keys of a map, in two cases
		" \t\r\n{} \t\r\n",                                         // whitespace around the value
		largeObject(100) + "}",                                     // many keys, each once
		`{"a":"\"\\\/\b\f\n\r\t"}`,                                 // every simple escape
		`[0,-0,1.5,-1e10,2E+3,3e-2,123456789012345678901234567890]`,
	} {
		var into any
		if err := strictjson.Unmarshal([]byte(document), &into); err != nil {
			t.Errorf("%s: %v; want it decoded", document, err)
		}
	}
}

// cafePrecomposed and cafeDecomposed are one word, NFC-equal: é as one code point, and as e and a
// combining acute accent.
const (
	cafePrecomposed = "caf\xc3\xa9"
	cafeDecomposed  = "cafe\xcc\x81"
)

// u is JSON's escape of the code unit hex, written so: a backslash and a u.
func u(hex string) string { return `\` + "u" + hex }

// largeObject is an object of count distinct keys, key0 onward, its closing brace left off.
func largeObject(count int) string {
	var object strings.Builder
	object.WriteString("{")
	for index := range count {
		if index > 0 {
			object.WriteString(",")
		}
		object.WriteString(`"key` + itoa(index) + `":1`)
	}
	return object.String()
}

func itoa(number int) string { return strconv.Itoa(number) }

func TestUnmarshal_AnUnknownFieldIsAccepted(t *testing.T) {
	t.Parallel()
	var into struct {
		Name string `json:"name"`
	}
	err := strictjson.Unmarshal([]byte(`{"name":"a","addedLater":{"x":[1,2]}}`), &into)
	if err != nil || into.Name != "a" {
		t.Errorf("got %+v, %v; want the name and the unknown field ignored", into, err)
	}
}

type everyKind struct {
	Text    string            `json:"text"`
	Number  float64           `json:"number"`
	Integer int64             `json:"integer"`
	Flag    bool              `json:"flag"`
	List    []int             `json:"list"`
	Map     map[string]string `json:"map"`
	Pointer *everyKind        `json:"pointer"`
	Raw     json.RawMessage   `json:"raw"`
	Any     any               `json:"any"`
}

func TestUnmarshal_DecodesADocumentAsEncodingJSONDoes(t *testing.T) {
	t.Parallel()
	document := []byte(`{"text":"h` + u("00e9") + `llo ` + u("d83d") + u("de00") + "\xc3\xa9" + ` \"q\"",
		"number":-1.25e2,"integer":9007199254740993,"flag":true,"list":[1,2,3],"map":{"a":"b","b":"c"},
		"pointer":{"text":"in","pointer":null},"raw":{"kept": [1, 2]},"any":[null,{"x":1.5},"s"],"extra":1}`)
	var strict, standard everyKind
	strictErr := strictjson.Unmarshal(document, &strict)
	standardErr := json.Unmarshal(document, &standard)
	if strictErr != nil || standardErr != nil || !reflect.DeepEqual(strict, standard) {
		t.Errorf("strict %+v, %v; encoding/json %+v, %v", strict, strictErr, standard, standardErr)
	}
	var strictAny, standardAny any
	strictErr = strictjson.Unmarshal(document, &strictAny)
	standardErr = json.Unmarshal(document, &standardAny)
	if strictErr != nil || standardErr != nil || !reflect.DeepEqual(strictAny, standardAny) {
		t.Errorf("strict %v, %v; encoding/json %v, %v", strictAny, strictErr, standardAny, standardErr)
	}
}

func TestUnmarshal_NotJSONIsSaidInEncodingJSONsWords(t *testing.T) {
	t.Parallel()
	for _, document := range []string{
		``, `{`, `{"a"}`, `[1,]`, `nul`, `[01]`, `1.`, `"\x"`, `"` + u("12") + `"`, "[\"\t\"]",
	} {
		var into any
		err := strictjson.Unmarshal([]byte(document), &into)
		var syntax *json.SyntaxError
		if !errors.As(err, &syntax) || errors.Is(err, strictjson.ErrRefused) {
			t.Errorf("%q: %v; want encoding/json's syntax error", document, err)
		}
	}
	var into struct{ Number int }
	var typeErr *json.UnmarshalTypeError
	if err := strictjson.Unmarshal([]byte(`{"Number":"one"}`), &into); !errors.As(err, &typeErr) {
		t.Errorf("a string for an int: %v; want encoding/json's type error", err)
	}
}

func TestUnmarshal_HoldsEachLimitAtItAndRefusesPastIt(t *testing.T) {
	t.Parallel()
	nested := func(arrays int, inner string) string {
		return strings.Repeat("[", arrays) + inner + strings.Repeat("]", arrays)
	}
	list := func(count int) string { return "[" + strings.TrimSuffix(strings.Repeat("0,", count), ",") + "]" }
	for _, test := range []struct {
		name, atLimit, pastLimit, names string
	}{
		{"depth", nested(strictjson.MaxDepth-1, "1"), nested(strictjson.MaxDepth, "1"), "nesting deeper than 500"},
		{"depth of empty arrays", nested(strictjson.MaxDepth, ""), nested(strictjson.MaxDepth+1, ""), "nesting"},
		{"elements", list(strictjson.MaxElements), list(strictjson.MaxElements + 1), "more than 1000000 elements"},
		{
			"string bytes", `"` + strings.Repeat("a", strictjson.MaxStringBytes) + `"`,
			`"` + strings.Repeat("a", strictjson.MaxStringBytes+1) + `"`, "more than 10000000 bytes",
		},
		{
			"string bytes after escapes", `"` + strings.Repeat(`\n`, strictjson.MaxStringBytes) + `"`,
			`"` + strings.Repeat(`\n`, strictjson.MaxStringBytes) + `a"`, "more than 10000000 bytes",
		},
	} {
		var into any
		if err := strictjson.Unmarshal([]byte(test.atLimit), &into); err != nil {
			t.Errorf("%s at the limit: %v", test.name, err)
		}
		err := strictjson.Unmarshal([]byte(test.pastLimit), &into)
		if !errors.Is(err, strictjson.ErrRefused) || !strings.Contains(err.Error(), test.names) {
			t.Errorf("%s past the limit: %v; want ErrRefused naming %q", test.name, err, test.names)
		}
	}
}

// The limits are BONJSON's recommended defaults, and a document's no larger than BONJSON's (D456 §5).
func TestLimits_AreBONJSONsRecommendedDefaults(t *testing.T) {
	t.Parallel()
	if strictjson.MaxDepth != 500 || strictjson.MaxElements != 1_000_000 ||
		strictjson.MaxStringBytes != 10_000_000 || strictjson.MaxDocumentBytes > 2_000_000_000 {
		t.Errorf("depth %d, elements %d, string %d, document %d", strictjson.MaxDepth, strictjson.MaxElements,
			strictjson.MaxStringBytes, strictjson.MaxDocumentBytes)
	}
}

func TestDecode_RefusesASecondValueThatEncodingJSONsDecoderLeftUnread(t *testing.T) {
	t.Parallel()
	var into map[string]int
	err := strictjson.Decode(strings.NewReader(`{"a":1} {"a":2}`), &into)
	if !errors.Is(err, strictjson.ErrRefused) || into != nil {
		t.Errorf("got %v, %v; want ErrRefused and nothing decoded", into, err)
	}
	err = strictjson.Decode(strings.NewReader("{\"a\":1}\n"), &into)
	if err != nil || into["a"] != 1 {
		t.Errorf("one value and a newline: %v, %v", into, err)
	}
	err = strictjson.Decode(strings.NewReader(`{"a":1,"a":2}`), &into)
	if !errors.Is(err, strictjson.ErrRefused) {
		t.Errorf("a duplicate read from a reader: %v; want ErrRefused", err)
	}
}

func BenchmarkUnmarshal(b *testing.B) {
	document := benchmarkDocument(b)
	b.SetBytes(int64(len(document)))
	for b.Loop() {
		var into []listedFilm
		if err := strictjson.Unmarshal(document, &into); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodingJSONUnmarshal(b *testing.B) {
	document := benchmarkDocument(b)
	b.SetBytes(int64(len(document)))
	for b.Loop() {
		var into []listedFilm
		if err := json.Unmarshal(document, &into); err != nil {
			b.Fatal(err)
		}
	}
}

// listedFilm is a library listing's shape.
type listedFilm struct {
	ID       int64             `json:"id"`
	Title    string            `json:"title"`
	Overview string            `json:"overview"`
	Year     int               `json:"year"`
	Rating   float64           `json:"rating"`
	Genres   []string          `json:"genres"`
	Paths    []string          `json:"paths"`
	Extra    map[string]string `json:"extra"`
	Watched  bool              `json:"watched"`
}

// benchmarkDocument is a listing of thousands of films, about 3 MB.
func benchmarkDocument(b *testing.B) []byte {
	b.Helper()
	films := make([]listedFilm, 5000)
	for index := range films {
		films[index] = listedFilm{
			ID: int64(index), Title: "Ein Film \xc3\xbcber Caf\xc3\xa9 Nummer " + itoa(index),
			Overview: strings.Repeat("A long overview of the film, with words. ", 8), Year: 1990 + index%30,
			Rating: 7.25, Genres: []string{"Drama", "Comedy"}, Paths: []string{"/media/films/a.mkv"},
			Extra: map[string]string{"tmdb": itoa(index), "imdb": "tt" + itoa(index)}, Watched: index%2 == 0,
		}
	}
	encoded, err := json.Marshal(films)
	if err != nil {
		b.Fatal(err)
	}
	return encoded
}
